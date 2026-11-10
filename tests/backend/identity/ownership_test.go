package identity

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"vanillapay/internal/auth"
	"vanillapay/internal/merchant"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_MerchantCannotMutateAnotherMerchantsWhitelist(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantOne, _ := db.SeedMerchant(t)
	merchantTwo, _ := db.SeedMerchant(t)
	service := db.NewApplication(t).Services.Merchant
	actorOne := platform.Actor{Role: "merchant", SubjectID: merchantOne, MerchantID: merchantOne}
	actorTwo := platform.Actor{Role: "merchant", SubjectID: merchantTwo, MerchantID: merchantTwo}

	if err := service.SetIPWhitelist(t.Context(), actorOne, merchantTwo, merchant.IPWhitelist{Enabled: true, Entries: []string{"192.0.2.0/24"}}); err == nil {
		t.Fatal("M1 mutated M2's IP whitelist")
	} else {
		var apiErr *platform.Error
		if !errors.As(err, &apiErr) || apiErr.Code != "FORBIDDEN" {
			t.Fatalf("M1 cross-owner whitelist mutation error=%v, want FORBIDDEN", err)
		}
	}
	var rowCount int
	var rulesEnabled bool
	var ruleCount int
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*),bool_and(enabled),COALESCE(max(cardinality(entries)),0) FROM merchant_ip_rules WHERE merchant_id=$1`, merchantTwo).Scan(&rowCount, &rulesEnabled, &ruleCount); err != nil || rowCount != 1 || rulesEnabled || ruleCount != 0 {
		t.Fatalf("rejected M1 mutation left M2 whitelist rows=%d err=%v", rowCount, err)
	}

	if err := service.SetIPWhitelist(t.Context(), actorTwo, merchantTwo, merchant.IPWhitelist{Enabled: true, Entries: []string{"198.51.100.7"}}); err != nil {
		t.Fatalf("M2 could not update its own whitelist: %v", err)
	}
	var enabled bool
	var entries []string
	if err := db.Pool.QueryRow(t.Context(), `SELECT enabled,entries::text[] FROM merchant_ip_rules WHERE merchant_id=$1`, merchantTwo).Scan(&enabled, &entries); err != nil || !enabled || len(entries) != 1 || entries[0] != "198.51.100.7/32" {
		t.Fatalf("M2 own whitelist enabled=%v entries=%v err=%v", enabled, entries, err)
	}
}

func TestHTTP_MerchantOwnershipBoundary(t *testing.T) {
	db := testsupport.NewDatabase(t)
	m1, _ := db.SeedMerchant(t)
	m2, m2Key := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)

	bootstrapCode, err := fixture.App.Services.Auth.RotateBootstrapCode(t.Context())
	if err != nil {
		t.Fatal("issue admin bootstrap code for ownership fixture")
	}
	if _, err := fixture.App.Services.Auth.Bootstrap(t.Context(), auth.BootstrapRequest{
		InitializationCode: bootstrapCode, Username: "ownership_admin", Password: fixturePassword, PasswordConfirmation: fixturePassword,
	}); err != nil {
		t.Fatalf("bootstrap ownership fixture admin: %v", err)
	}
	var version int64
	if err := db.Pool.QueryRow(t.Context(), `SELECT version FROM site_settings WHERE singleton=true`).Scan(&version); err != nil {
		t.Fatal("read initial security settings version")
	}
	security, err := json.Marshal(map[string]any{
		"turnstile_site_key": "ownership-site-key", "turnstile_secret": "ownership-secret", "turnstile_enabled": true, "turnstile_token": "ok:settings_security",
		"restrict_email_domains": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "help@example.test", "en": "help@example.test"},
	})
	if err != nil {
		t.Fatal("encode local Siteverify settings")
	}
	if _, err := fixture.App.Services.Settings.UpdateGroup(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, "security", settings.UpdateGroup{ExpectedVersion: version, Value: security}); err != nil {
		t.Fatalf("configure local Siteverify: %v", err)
	}
	fixture.Siteverify.SetMode("accept")

	password := fixturePassword
	passwordHash, err := auth.HashPassword(password, nil)
	if err != nil {
		t.Fatal("hash ownership fixture password")
	}
	for _, id := range []int64{m1, m2} {
		if _, err := db.Pool.Exec(t.Context(), `UPDATE merchants SET password_hash=$1 WHERE id=$2`, passwordHash, id); err != nil {
			t.Fatalf("set merchant %d fixture password: %v", id, err)
		}
	}
	var username1, email1, username2, email2 string
	if err := db.Pool.QueryRow(t.Context(), `SELECT username,email FROM merchants WHERE id=$1`, m1).Scan(&username1, &email1); err != nil {
		t.Fatal("read M1 fixture identity")
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT username,email FROM merchants WHERE id=$1`, m2).Scan(&username2, &email2); err != nil {
		t.Fatal("read M2 fixture identity")
	}
	for _, account := range []struct {
		id       int64
		username string
		client   string
	}{{m1, username1, "ownership-m1"}, {m2, username2, "ownership-m2"}} {
		login := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"identifier": account.username, "password": password, "turnstile_token": "ok:login",
		}, "merchant:"+account.client)
		if login.Code != http.StatusOK {
			t.Fatalf("login merchant %d: status=%d body=%s", account.id, login.Code, login.Body.String())
		}
	}

	application := fixture.App
	planID := db.SeedPlan(t, "ownership billing plan", 1000, 3650, []string{"alipay"}, nil)
	deviceToken := db.PrepareBillingMerchant(t, application, m2, m2Key, planID)
	order, err := application.Services.Payment.Create(t.Context(), payment.CreateCommand{
		MerchantID: m2, Protocol: "v1", Purpose: "external", OutTradeNo: "ownership-m2-out", Name: "M2 private order marker",
		OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", ReturnURL: "https://merchant.example.test/return",
		ClientIP: "198.51.100.88", SourceIP: "192.0.2.88", Actor: platform.Actor{Role: "merchant", SubjectID: m2, MerchantID: m2},
	})
	if err != nil {
		t.Fatalf("create M2 owned order: %v", err)
	}
	var assetID string
	if err := db.Pool.QueryRow(t.Context(), `SELECT id FROM media_assets WHERE owner_merchant_id=$1 AND kind='qr' AND active=true ORDER BY id LIMIT 1`, m2).Scan(&assetID); err != nil {
		t.Fatal("read M2 QR asset identity")
	}
	receipt := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", map[string]any{
		"event_id": "e0000000-0000-4000-8000-000000000301", "channel": "alipay", "amount_cents": 100,
		"occurred_at": db.Clock.Now().Format("2006-01-02T15:04:05.999999999Z07:00"), "raw_text": "M2 private receipt marker",
	}, "device:"+deviceToken)
	if receipt.Code != http.StatusCreated {
		t.Fatalf("create M2 receipt: status=%d body=%s", receipt.Code, receipt.Body.String())
	}
	var receiptData struct {
		ID string `json:"receipt_id"`
	}
	var receiptEnvelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(receipt.Body.Bytes(), &receiptEnvelope); err != nil || json.Unmarshal(receiptEnvelope.Data, &receiptData) != nil || receiptData.ID == "" {
		t.Fatalf("decode M2 receipt response: body=%s err=%v", receipt.Body.String(), err)
	}
	var orderID int64
	if err := db.Pool.QueryRow(t.Context(), `SELECT id FROM orders WHERE trade_no=$1`, order.TradeNo).Scan(&orderID); err != nil {
		t.Fatal("read M2 order row")
	}
	paidTx, err := db.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal("begin M2 payment fixture transaction")
	}
	if err := application.Services.Payment.MarkPaidTx(t.Context(), paidTx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
		_ = paidTx.Rollback(t.Context())
		t.Fatalf("mark M2 order paid for private notification: %v", err)
	}
	if err := paidTx.Commit(t.Context()); err != nil {
		t.Fatal("commit M2 paid order fixture")
	}
	refundDigest := sha256.Sum256([]byte("M2-private-refund-request"))
	if _, err := application.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{
		MerchantID: m2, TradeNo: order.TradeNo, OutRefundNo: "ownership-m2-refund", AmountCents: 100,
		Reason: "M2 private refund marker", RequestDigest: refundDigest[:], Actor: platform.Actor{Role: "merchant", SubjectID: m2, MerchantID: m2},
	}); err != nil {
		t.Fatalf("create M2 private refund notification: %v", err)
	}

	t.Run("V12.B.r04/v01", func(t *testing.T) {
		for _, probe := range []struct{ path, marker string }{
			{"/api/v1/merchant/orders/" + order.TradeNo, "M2 private order marker"},
			{"/api/v1/merchant/receipts/" + receiptData.ID, "M2 private receipt marker"},
			{"/media/" + assetID, assetID},
		} {
			response := fixture.Request(t, http.MethodGet, probe.path, nil, "merchant:ownership-m1")
			if response.Code != http.StatusNotFound {
				t.Errorf("M1 cross-owner probe %s status=%d body=%s, want 404", probe.path, response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), probe.marker) {
				t.Errorf("M1 cross-owner response exposed marker %q: %s", probe.marker, response.Body.String())
			}
		}
		me := fixture.Request(t, http.MethodGet, "/api/v1/merchant/me", nil, "merchant:ownership-m1")
		if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), username1) || strings.Contains(me.Body.String(), username2) || strings.Contains(me.Body.String(), email2) {
			t.Fatalf("M1 profile DTO leaked or omitted owner identity: status=%d body=%s", me.Code, me.Body.String())
		}
	})
	t.Run("V12.B.r04/v02", func(t *testing.T) {
		var page struct {
			Data struct {
				Items []json.RawMessage `json:"items"`
			} `json:"data"`
		}
		response := fixture.Request(t, http.MethodGet, "/api/v1/merchant/notifications?limit=20", nil, "merchant:ownership-m1")
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil {
			t.Fatalf("M1 notification page status=%d body=%s", response.Code, response.Body.String())
		}
		for _, item := range page.Data.Items {
			if strings.Contains(string(item), username2) || strings.Contains(string(item), "M2 private") || strings.Contains(string(item), assetID) {
				t.Fatalf("M1 notification page contains an M2 data fragment: %s", item)
			}
		}
		m2Response := fixture.Request(t, http.MethodGet, "/api/v1/merchant/notifications?limit=20", nil, "merchant:ownership-m2")
		var m2Page struct {
			Data struct {
				Items []json.RawMessage `json:"items"`
			} `json:"data"`
		}
		if m2Response.Code != http.StatusOK || json.Unmarshal(m2Response.Body.Bytes(), &m2Page) != nil || len(m2Page.Data.Items) == 0 {
			t.Fatalf("M2 fixture did not contain its own notification: status=%d body=%s", m2Response.Code, m2Response.Body.String())
		}
		if !strings.Contains(string(m2Page.Data.Items[0]), order.TradeNo) {
			t.Fatalf("M2 private notification did not contain its owner-only order marker: %s", m2Page.Data.Items[0])
		}
	})
}
