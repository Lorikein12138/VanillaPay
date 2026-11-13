package identity

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"vanillapay/internal/auth"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_IntegrationKeyLifecycle(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	merchantID, oldKey := db.SeedMerchant(t)
	otherID, otherKey := db.SeedMerchant(t)
	password := fixturePassword
	passwordHash, err := auth.HashPassword(password, nil)
	if err != nil {
		t.Fatal("hash integration-key fixture password")
	}
	if _, err := db.Pool.Exec(t.Context(), `UPDATE merchants SET password_hash=$1 WHERE id=$2`, passwordHash, merchantID); err != nil {
		t.Fatal("set integration-key fixture password")
	}
	var username, email, pid string
	if err := db.Pool.QueryRow(t.Context(), `SELECT username,email,pid FROM merchants WHERE id=$1`, merchantID).Scan(&username, &email, &pid); err != nil {
		t.Fatal("read integration-key fixture account")
	}
	if err := configureTOTPTestSiteverify(t, fixture, t.Context()); err != nil {
		t.Fatalf("configure local Siteverify: %v", err)
	}
	client := "merchant:integration-key-lifecycle"
	login := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"identifier": username, "password": password, "turnstile_token": "ok:login",
	}, client)
	assertIdentityResponse(t, login, http.StatusOK, "integration-key fixture login")

	planID := db.SeedPlan(t, "integration key lifecycle", 1000, 3650, []string{"alipay"}, nil)
	if db.PrepareBillingMerchant(t, fixture.App, merchantID, oldKey, planID) == "" {
		t.Fatal("prepare integration-key billing device")
	}
	fixture.Network.SetDNS("merchant.example.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	callbackRequests := make(chan url.Values, 2)
	callbackURL := fixture.Network.ServeTLS(t, "merchant.example.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		callbackRequests <- r.URL.Query()
		_, _ = w.Write([]byte("success"))
	}))
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	historicalOrder, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
		MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "old-key-callback-order", Name: "Old key callback",
		OriginalCents: 100, NotifyURL: callbackURL + "/notify", ReturnURL: "https://merchant.example.test/return",
		ClientIP: "198.51.100.50", SourceIP: "192.0.2.50", Actor: actor,
	})
	if err != nil {
		t.Fatalf("create historical order before key rotation: %v", err)
	}
	if _, err := fixture.App.Services.Payment.SelectChannel(t.Context(), historicalOrder.TradeNo, "alipay"); err != nil {
		t.Fatalf("select historical order channel: %v", err)
	}

	t.Run("V14.B.r01/v01", func(t *testing.T) {
		const path = "/api/v1/merchant/integration/reset-key"
		missing := fixture.Request(t, http.MethodPost, path, map[string]any{"security_proof": map[string]string{}}, client)
		assertIdentityError(t, missing, "VALIDATION_ERROR", "merchant KEY reset with missing email code field")
		response := fixture.Request(t, http.MethodPost, path, map[string]any{"security_proof": map[string]string{"email_code": ""}}, client)
		assertIdentityError(t, response, "INVALID_CODE", "merchant KEY reset without email proof")
		var version int
		if err := db.Pool.QueryRow(t.Context(), `SELECT key_version FROM merchants WHERE id=$1`, merchantID).Scan(&version); err != nil || version != 1 {
			t.Fatalf("failed unauthenticated key reset changed key_version=%d err=%v", version, err)
		}
	})

	var rotatedKey string
	t.Run("V14.B.r01/v02", func(t *testing.T) {
		code := sendProfileEmailCode(t, fixture, client, "reset_key", email, "", "")
		response := fixture.Request(t, http.MethodPost, "/api/v1/merchant/integration/reset-key", map[string]any{
			"security_proof": map[string]string{"email_code": code},
		}, client)
		data := assertIdentityResponse(t, response, http.StatusOK, "merchant KEY reset with email proof")
		var result struct {
			PID        string `json:"pid"`
			Key        string `json:"key"`
			KeyVersion int    `json:"key_version"`
		}
		if err := json.Unmarshal(data, &result); err != nil || result.PID != pid || result.Key == "" || result.Key == oldKey || result.KeyVersion != 2 {
			t.Fatalf("rotation did not issue a new credential for the merchant at version 2: %v", err)
		}
		rotatedKey = result.Key
		audits := fixture.Request(t, http.MethodGet, "/api/v1/merchant/audit-logs?action=merchant.reset_key", nil, client)
		var page struct {
			Items []struct {
				Action   string `json:"action"`
				Metadata struct {
					KeyVersion int `json:"key_version"`
				} `json:"safe_metadata"`
			} `json:"items"`
		}
		if err := json.Unmarshal(assertIdentityResponse(t, audits, http.StatusOK, "read key rotation audit"), &page); err != nil ||
			len(page.Items) != 1 || page.Items[0].Action != "merchant.reset_key" || page.Items[0].Metadata.KeyVersion != result.KeyVersion {
			t.Fatalf("rotation audit missing numeric key version: %v", err)
		}
	})

	protocolValues := func(outTradeNo string) url.Values {
		return url.Values{
			"pid": {pid}, "notify_url": {callbackURL + "/protocol-notify"}, "return_url": {"https://merchant.example.test/return"},
			"out_trade_no": {outTradeNo}, "name": {"rotated key order"}, "money": {"1.00"}, "clientip": {"198.51.100.51"}, "type": {"alipay"},
		}
	}
	var rotatedOrder protocolResult
	t.Run("V14.B.r02/v01", func(t *testing.T) {
		rotatedOrder = postV1Protocol(t, fixture, "/mapi.php", "192.0.2.51:43001", protocolValues("rotated-key-old-signature"), oldKey)
		if rotatedOrder.ErrorCode != "INVALID_SIGNATURE" || rotatedOrder.TradeNo != "" {
			t.Fatalf("old KEY created a post-rotation order: %#v", rotatedOrder)
		}
	})
	t.Run("V14.B.r02/v02", func(t *testing.T) {
		rotatedOrder = postV1Protocol(t, fixture, "/mapi.php", "192.0.2.51:43002", protocolValues("rotated-key-new-signature"), rotatedKey)
		if rotatedOrder.Code != 1 || rotatedOrder.TradeNo == "" {
			t.Fatalf("new KEY failed to create a V1 order: %#v", rotatedOrder)
		}
	})
	t.Run("V14.B.r02/v03", func(t *testing.T) {
		if err := fixture.App.Services.Payment.SupplementIdempotent(t.Context(), actor, historicalOrder.TradeNo,
			payment.SupplementRequest{Reason: "settle pre-rotation order", Channel: "alipay"}, "old-key-callback-settle"); err != nil {
			t.Fatalf("settle historical order after key rotation: %v", err)
		}
		var orderID int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT id FROM orders WHERE trade_no=$1`, historicalOrder.TradeNo).Scan(&orderID); err != nil {
			t.Fatalf("read historical order id: %v", err)
		}
		if err := fixture.App.Services.Payment.ProcessCallback(t.Context(), orderID); err != nil {
			t.Fatalf("deliver historical callback after key rotation: %v", err)
		}
		select {
		case values := <-callbackRequests:
			if got, want := values.Get("sign"), signWhitelistV1(values, oldKey); got != want {
				t.Fatalf("historical callback signature=%q, want original order KEY signature %q", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("historical callback was not sent to the controlled local TLS endpoint")
		}
	})

	var publicKeyPEM string
	t.Run("V14.B.r03/v01", func(t *testing.T) {
		db.Clock.Set(db.Clock.Now().Add(61 * time.Second))
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal("generate independent merchant RSA key")
		}
		publicKeyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: mustPKIXPublicKey(t, &privateKey.PublicKey)}))
		code := sendProfileEmailCode(t, fixture, client, "reset_key", email, "", "")
		response := fixture.Request(t, http.MethodPut, "/api/v1/merchant/integration/rsa-public-key", map[string]any{
			"public_key_pem": publicKeyPEM, "security_proof": map[string]string{"email_code": code},
		}, client)
		data := assertIdentityResponse(t, response, http.StatusOK, "merchant RSA public key upload")
		var result struct {
			Fingerprint string `json:"fingerprint"`
			UpdatedAt   string `json:"updated_at"`
		}
		if err := json.Unmarshal(data, &result); err != nil || len(result.Fingerprint) != 64 || result.UpdatedAt == "" {
			t.Fatalf("RSA public-key response = %#v err=%v", result, err)
		}
		var stored string
		if err := db.Pool.QueryRow(t.Context(), `SELECT rsa_public_key_pem FROM merchants WHERE id=$1`, merchantID).Scan(&stored); err != nil || stored != publicKeyPEM {
			t.Fatalf("stored RSA public key changed: equal=%t err=%v", stored == publicKeyPEM, err)
		}
	})
	t.Run("V14.B.r03/v02", func(t *testing.T) {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal("generate private-key rejection fixture")
		}
		privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			t.Fatal("encode private-key rejection fixture")
		}
		shortKey, err := rsa.GenerateKey(rand.Reader, 1024)
		if err != nil {
			t.Fatal("generate too-short RSA key fixture")
		}
		for label, value := range map[string]string{
			"private key rejected":      string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
			"short public key rejected": string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: mustPKIXPublicKey(t, &shortKey.PublicKey)})),
		} {
			response := fixture.Request(t, http.MethodPut, "/api/v1/merchant/integration/rsa-public-key", map[string]any{"public_key_pem": value}, client)
			assertIdentityError(t, response, "VALIDATION_ERROR", label)
		}
	})
	t.Run("V14.B.r03/v03", func(t *testing.T) {
		first := fixture.App.Services.Merchant
		before, err := first.Integration(t.Context(), actor, "https://pay.example.test")
		if err != nil {
			t.Fatalf("read platform key before application reconstruction: %v", err)
		}
		beforeKeys, ok := before["platform_public_keys"].([]map[string]string)
		if !ok || len(beforeKeys) != 1 || beforeKeys[0]["pem"] == "" {
			t.Fatalf("platform public key list before reconstruction = %#v", before["platform_public_keys"])
		}
		restarted := db.NewApplication(t)
		after, err := restarted.Services.Merchant.Integration(t.Context(), actor, "https://pay.example.test")
		if err != nil {
			t.Fatalf("read platform key after application reconstruction: %v", err)
		}
		afterKeys, ok := after["platform_public_keys"].([]map[string]string)
		if !ok || len(afterKeys) != 1 || afterKeys[0]["id"] != beforeKeys[0]["id"] || afterKeys[0]["pem"] != beforeKeys[0]["pem"] {
			t.Fatalf("platform public key changed after reconstruction: before=%#v after=%#v", beforeKeys, after["platform_public_keys"])
		}
	})
	t.Run("V14.B.r04/v01", func(t *testing.T) {
		otherValues := protocolValues("cross-merchant-key-signature")
		otherValues.Set("pid", pid)
		result := postV1Protocol(t, fixture, "/mapi.php", "192.0.2.52:43003", otherValues, otherKey)
		if result.ErrorCode != "INVALID_SIGNATURE" || result.TradeNo != "" {
			t.Fatalf("another merchant's signing key authenticated this merchant PID: %#v (second merchant id %d)", result, otherID)
		}
	})
}

func mustPKIXPublicKey(t *testing.T, key *rsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal("marshal independent RSA public key")
	}
	return der
}
