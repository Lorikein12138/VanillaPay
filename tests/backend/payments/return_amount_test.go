package payments

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_ProtocolAmountsAndSignedReturnContracts(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	planID := db.SeedPlan(t, "return amount contract plan", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	if _, err := db.Pool.Exec(t.Context(), `UPDATE merchant_collection_settings SET float_limit_cents=1,float_direction='down' WHERE merchant_id=$1`, merchantID); err != nil {
		t.Fatal("configure one-cent downward floating fixture")
	}
	fixture.Network.SetDNS("merchant.example.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	type callback struct {
		mu     sync.Mutex
		values []url.Values
	}
	seen := &callback{}
	callbackURL := fixture.Network.ServeTLS(t, "merchant.example.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid callback form", http.StatusBadRequest)
			return
		}
		seen.mu.Lock()
		copyValues := make(url.Values, len(r.PostForm))
		for key, entries := range r.PostForm {
			copyValues[key] = append([]string(nil), entries...)
		}
		seen.values = append(seen.values, copyValues)
		seen.mu.Unlock()
		_, _ = w.Write([]byte("success"))
	}))
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	createWithReturnURL := func(protocol, outTrade, returnURL string) (payment.OrderDTO, error) {
		t.Helper()
		return fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: merchantID, Protocol: protocol, Purpose: "external", OutTradeNo: outTrade, Name: "One dollar order",
			OriginalCents: 100, NotifyURL: callbackURL + "/notify", ReturnURL: returnURL, ClientIP: "198.51.100.80", SourceIP: "192.0.2.80",
			Channel: "alipay", Actor: actor,
		})
	}
	create := func(protocol, outTrade, returnURL string) payment.OrderDTO {
		t.Helper()
		order, err := createWithReturnURL(protocol, outTrade, returnURL)
		if err != nil {
			t.Fatalf("create %s protocol order: %v", protocol, err)
		}
		return order
	}
	markPaid := func(order payment.OrderDTO) {
		t.Helper()
		orderID, err := fixture.App.Services.Payment.OrderID(t.Context(), order.TradeNo)
		if err != nil {
			t.Fatalf("lookup order %s: %v", order.TradeNo, err)
		}
		tx, err := db.Pool.Begin(t.Context())
		if err != nil {
			t.Fatalf("begin payment transition for %s: %v", order.TradeNo, err)
		}
		if err := fixture.App.Services.Payment.MarkPaidTx(t.Context(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
			_ = tx.Rollback(t.Context())
			t.Fatalf("mark order %s paid: %v", order.TradeNo, err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatalf("commit paid order %s: %v", order.TradeNo, err)
		}
	}

	create("v1", "return-v1-reservation", "https://merchant.example.test/reservation")
	v1 := create("v1", "return-v1-amount", "https://merchant.example.test/return?campaign=original")
	v1Pending := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+v1.TradeNo, nil, "guest")
	var pendingEnvelope struct {
		Data struct {
			Status          string  `json:"status"`
			ActualCents     *int64  `json:"actual_cents"`
			ReturnURLSigned *string `json:"return_url_signed"`
		} `json:"data"`
	}
	if v1Pending.Code != http.StatusOK || json.Unmarshal(v1Pending.Body.Bytes(), &pendingEnvelope) != nil || pendingEnvelope.Data.Status != "pending" || pendingEnvelope.Data.ActualCents == nil || *pendingEnvelope.Data.ActualCents != 99 || pendingEnvelope.Data.ReturnURLSigned != nil {
		t.Fatalf("floating pending checkout status=%d data=%#v body=%s; want 100 original/99 actual and no success return", v1Pending.Code, pendingEnvelope.Data, v1Pending.Body.String())
	}
	var pid string
	if err := db.Pool.QueryRow(t.Context(), `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&pid); err != nil {
		t.Fatal("read V1 protocol PID")
	}
	v1Query := url.Values{"pid": {pid}, "key": {merchantKey}, "act": {"order"}, "trade_no": {v1.TradeNo}, "sign_type": {"MD5"}}
	v1Query.Set("sign", mustV1ContractSignature(v1Query, merchantKey))
	v1QueryResponse := fixture.Request(t, http.MethodGet, "/api.php?"+v1Query.Encode(), nil, "guest")
	var v1QueryData map[string]json.RawMessage
	if v1QueryResponse.Code != http.StatusOK || json.Unmarshal(v1QueryResponse.Body.Bytes(), &v1QueryData) != nil || string(v1QueryData["money"]) != `"1.00"` || string(v1QueryData["actual_money"]) != `"0.99"` {
		t.Fatalf("V1 query amount response=%d data=%s; want money 1.00 and actual_money 0.99", v1QueryResponse.Code, v1QueryResponse.Body.String())
	}
	markPaid(v1)
	v1ID, err := fixture.App.Services.Payment.OrderID(t.Context(), v1.TradeNo)
	if err != nil {
		t.Fatal("lookup V1 paid order")
	}
	if err := fixture.App.Services.Payment.ProcessCallback(t.Context(), v1ID); err != nil {
		t.Fatalf("deliver real V1 callback through local TLS: %v", err)
	}
	seen.mu.Lock()
	v1Notice := seen.values[0]
	seen.mu.Unlock()
	if v1Notice.Get("money") != "1.00" || v1Notice.Get("trade_no") != v1.TradeNo || !verifyV1Independent(v1Notice, merchantKey) {
		t.Fatalf("V1 callback amount/signature = money %q trade %q", v1Notice.Get("money"), v1Notice.Get("trade_no"))
	}
	checkoutV1 := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+v1.TradeNo, nil, "guest")
	var paidEnvelope struct {
		Data struct {
			Status          string  `json:"status"`
			ReturnURLSigned *string `json:"return_url_signed"`
		} `json:"data"`
	}
	if checkoutV1.Code != http.StatusOK || json.Unmarshal(checkoutV1.Body.Bytes(), &paidEnvelope) != nil || paidEnvelope.Data.Status != "paid" || paidEnvelope.Data.ReturnURLSigned == nil {
		t.Fatalf("paid V1 checkout did not expose signed return: status=%d body=%s", checkoutV1.Code, checkoutV1.Body.String())
	}
	v1Return, err := url.Parse(*paidEnvelope.Data.ReturnURLSigned)
	if err != nil || v1Return.Query().Get("campaign") != "original" || v1Return.Query().Get("money") != "1.00" || v1Return.Query().Get("trade_no") != v1.TradeNo || v1Return.Query().Get("trade_status") != "TRADE_SUCCESS" || !verifyV1Independent(v1Return.Query(), merchantKey) {
		t.Fatalf("V1 signed return did not preserve original query and sign original cents: %s err=%v", *paidEnvelope.Data.ReturnURLSigned, err)
	}
	secondOrder := create("v1", "return-v1-snapshot", "https://merchant.example.test/return?campaign=snapshot")
	keyCipher, err := platform.Encrypt(db.Key[:], []byte("rotated-v1-key-after-order"), rand.Reader)
	if err != nil {
		t.Fatal("encrypt rotated merchant key")
	}
	if _, err := db.Pool.Exec(t.Context(), `UPDATE merchants SET key_cipher=$1,key_version=key_version+1 WHERE id=$2`, keyCipher, merchantID); err != nil {
		t.Fatal("rotate merchant V1 key after order snapshot")
	}
	markPaid(secondOrder)
	secondOrderID, err := fixture.App.Services.Payment.OrderID(t.Context(), secondOrder.TradeNo)
	if err != nil {
		t.Fatal("lookup V1 key-snapshot order")
	}
	if err := fixture.App.Services.Payment.ProcessCallback(t.Context(), secondOrderID); err != nil {
		t.Fatalf("deliver V1 snapshot callback: %v", err)
	}
	seen.mu.Lock()
	v1SnapshotNotice := seen.values[1]
	seen.mu.Unlock()
	if verifyV1Independent(v1SnapshotNotice, "rotated-v1-key-after-order") || !verifyV1Independent(v1SnapshotNotice, merchantKey) {
		t.Fatal("V1 callback did not keep the order's pre-rotation signing-key snapshot")
	}

	var reservedData struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	mutableTarget := create("v1", "return-no-target-override", "https://merchant.example.test/return?campaign=original")
	markPaid(mutableTarget)
	attackerQuery := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+mutableTarget.TradeNo+"?return_url=https%3A%2F%2Fevil.example.test%2Fsteal", nil, "guest")
	if attackerQuery.Code != http.StatusOK || json.Unmarshal(attackerQuery.Body.Bytes(), &reservedData) != nil {
		t.Fatalf("checkout request with arbitrary target parameter failed: %d %s", attackerQuery.Code, attackerQuery.Body.String())
	}
	var signedURL string
	if json.Unmarshal(reservedData.Data["return_url_signed"], &signedURL) != nil || !strings.HasPrefix(signedURL, "https://merchant.example.test/return?") || strings.Contains(signedURL, "evil.example.test") {
		t.Fatalf("checkout accepted an arbitrary return target: %s", signedURL)
	}
	mutableSignedURL := signedURL
	t.Run("V35.B.r04/v02", func(t *testing.T) {
		_, err := createWithReturnURL("v1", "return-reserved-collision", "https://merchant.example.test/return?campaign=ok&money=forged")
		if err == nil {
			t.Fatal("order with a reserved protocol field in return_url was accepted")
		}
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no=$2`, merchantID, "return-reserved-collision").Scan(&count); err != nil {
			t.Fatalf("check rejected return URL left no order: %v", err)
		}
		if count != 0 {
			t.Fatalf("rejected return URL persisted %d orders", count)
		}
	})

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("generate independent V2 merchant signing key")
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: mustMarshalPKIX(t, &privateKey.PublicKey)})
	if _, err := db.Pool.Exec(t.Context(), `UPDATE merchants SET rsa_public_key_pem=$1 WHERE id=$2`, string(publicPEM), merchantID); err != nil {
		t.Fatal("install V2 merchant public key")
	}
	v2 := create("v2", "return-v2-amount", "https://merchant.example.test/v2-return?campaign=original")
	v2Query := url.Values{"pid": {pid}, "trade_no": {v2.TradeNo}, "timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}}
	v2Query.Set("sign", signV2Independent(t, v2Query, privateKey))
	v2QueryResponse := fixture.RequestWithHeaders(t, http.MethodPost, "/api/pay/query", strings.NewReader(v2Query.Encode()), "guest", http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}})
	var v2QueryData map[string]any
	if v2QueryResponse.Code != http.StatusOK || json.Unmarshal(v2QueryResponse.Body.Bytes(), &v2QueryData) != nil || v2QueryData["money"] != "1.00" || v2QueryData["actual_money"] != "0.99" {
		t.Fatalf("V2 query amount response=%d body=%s; want money 1.00 and actual_money 0.99", v2QueryResponse.Code, v2QueryResponse.Body.String())
	}
	verifyPlatformV2Response(t, fixture, v2QueryResponse.Body.Bytes())
	markPaid(v2)
	v2ID, err := fixture.App.Services.Payment.OrderID(t.Context(), v2.TradeNo)
	if err != nil {
		t.Fatal("lookup V2 paid order")
	}
	if err := fixture.App.Services.Payment.ProcessCallback(t.Context(), v2ID); err != nil {
		t.Fatalf("deliver real V2 callback through local TLS: %v", err)
	}
	seen.mu.Lock()
	v2Notice := seen.values[len(seen.values)-1]
	seen.mu.Unlock()
	if v2Notice.Get("money") != "1.00" || v2Notice.Get("trade_no") != v2.TradeNo {
		t.Fatalf("V2 callback amount=%q trade=%q", v2Notice.Get("money"), v2Notice.Get("trade_no"))
	}
	verifyPlatformV2Values(t, fixture, v2Notice)
	v2Checkout := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+v2.TradeNo, nil, "guest")
	if v2Checkout.Code != http.StatusOK || json.Unmarshal(v2Checkout.Body.Bytes(), &reservedData) != nil {
		t.Fatalf("read paid V2 checkout: status=%d body=%s", v2Checkout.Code, v2Checkout.Body.String())
	}
	if json.Unmarshal(reservedData.Data["return_url_signed"], &signedURL) != nil || !strings.HasPrefix(signedURL, "https://merchant.example.test/v2-return?") {
		t.Fatalf("V2 signed return did not preserve allowed target: %s", signedURL)
	}
	v2ReturnValues := mustParseURLQuery(t, signedURL)
	verifyPlatformV2Values(t, fixture, v2ReturnValues)

	t.Run("V35.B.r01/v01", func(t *testing.T) {
		var actual int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT actual_cents FROM orders WHERE trade_no=$1`, v1.TradeNo).Scan(&actual); err != nil || actual != 99 {
			t.Fatalf("100-cent order float actual=%d err=%v", actual, err)
		}
	})
	t.Run("V35.B.r01/v02", func(t *testing.T) {
		if v1Notice.Get("money") != "1.00" || v2Notice.Get("money") != "1.00" || string(v1QueryData["money"]) != `"1.00"` || v2QueryData["money"] != "1.00" || v1Return.Query().Get("money") != "1.00" || v2ReturnValues.Get("money") != "1.00" {
			t.Fatalf("protocol original-money surfaces differ: v1 notice=%q v1 query=%s v1 return=%q v2 notice=%q v2 query=%v v2 return=%q", v1Notice.Get("money"), v1QueryData["money"], v1Return.Query().Get("money"), v2Notice.Get("money"), v2QueryData["money"], v2ReturnValues.Get("money"))
		}
	})
	t.Run("V35.B.r02/v01", func(t *testing.T) {
		if !verifyV1Independent(v1SnapshotNotice, merchantKey) || verifyV1Independent(v1SnapshotNotice, "rotated-v1-key-after-order") {
			t.Fatal("V1 signed callback was not tied to the immutable old KEY snapshot")
		}
	})
	t.Run("V35.B.r02/v02", func(t *testing.T) {
		verifyPlatformV2Values(t, fixture, v2Notice)
		verifyPlatformV2Values(t, fixture, v2ReturnValues)
	})
	t.Run("V35.B.r03/v01", func(t *testing.T) {
		if pendingEnvelope.Data.ReturnURLSigned != nil {
			t.Fatalf("unpaid checkout exposed a successful return URL: %s", *pendingEnvelope.Data.ReturnURLSigned)
		}
	})
	t.Run("V35.B.r03/v02", func(t *testing.T) {
		if paidEnvelope.Data.Status != "paid" || paidEnvelope.Data.ReturnURLSigned == nil {
			t.Fatalf("paid checkout omitted signed success return: %#v", paidEnvelope.Data)
		}
	})
	t.Run("V35.B.r04/v01", func(t *testing.T) {
		if v1Return.Query().Get("campaign") != "original" || v1Return.Query().Get("trade_no") != v1.TradeNo {
			t.Fatalf("signed return lost original query or protocol result: %s", v1Return)
		}
	})
	t.Run("V35.B.r04/v03", func(t *testing.T) {
		if !strings.HasPrefix(mutableSignedURL, "https://merchant.example.test/") || strings.Contains(mutableSignedURL, "evil.example.test") {
			t.Fatalf("untrusted request parameter changed the order's return target: %s", mutableSignedURL)
		}
	})
}

func mustMarshalPKIX(t *testing.T, key *rsa.PublicKey) []byte {
	t.Helper()
	encoded, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal("encode independent V2 merchant public key")
	}
	return encoded
}

func signV2Independent(t *testing.T, values url.Values, key *rsa.PrivateKey) string {
	t.Helper()
	names := make([]string, 0, len(values))
	for name, entries := range values {
		if name != "sign" && name != "sign_type" && len(entries) > 0 && entries[0] != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+values.Get(name))
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "&")))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal("sign independent V2 request")
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func verifyV1Independent(values url.Values, key string) bool {
	return values.Get("sign") != "" && values.Get("sign") == mustV1ContractSignature(values, key)
}

func verifyPlatformV2Response(t *testing.T, fixture *testsupport.HTTPFixture, body []byte) {
	t.Helper()
	values := mustParseJSONValues(t, body)
	verifyPlatformV2Values(t, fixture, values)
}

func verifyPlatformV2Values(t *testing.T, fixture *testsupport.HTTPFixture, values url.Values) {
	t.Helper()
	keys, err := fixture.App.Services.Payment.PlatformPublicKeys(t.Context())
	if err != nil || len(keys) == 0 {
		t.Fatalf("read platform public signing keys: keys=%#v err=%v", keys, err)
	}
	var matched bool
	for _, key := range keys {
		if values.Get("key_id") != key["id"] {
			continue
		}
		block, _ := pem.Decode([]byte(key["pem"]))
		if block == nil {
			t.Fatal("platform signing key PEM is invalid")
		}
		parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			t.Fatal("parse platform signing key")
		}
		pub, ok := parsed.(*rsa.PublicKey)
		if !ok {
			t.Fatal("platform signing key is not RSA")
		}
		signature, err := base64.StdEncoding.DecodeString(values.Get("sign"))
		if err != nil {
			t.Fatal("decode V2 RSA signature")
		}
		fields := values
		names := make([]string, 0, len(fields))
		for name, entries := range fields {
			if name != "sign" && name != "sign_type" && len(entries) > 0 && entries[0] != "" {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			parts = append(parts, name+"="+fields.Get(name))
		}
		digest := sha256.Sum256([]byte(strings.Join(parts, "&")))
		if rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], signature) == nil {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("V2 response signature did not verify against published platform keys: %#v", values)
	}
}

func mustParseJSONValues(t *testing.T, body []byte) url.Values {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("decode signed V2 response: %v", err)
	}
	values := make(url.Values)
	for key, raw := range data {
		switch value := raw.(type) {
		case string:
			values.Set(key, value)
		case float64:
			values.Set(key, strconv.FormatInt(int64(value), 10))
		case nil:
			if key == "expire_reason" || key == "refunded_at" {
				values.Set(key, "null")
			} else {
				values.Set(key, "")
			}
		}
	}
	return values
}

func mustParseURLQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse signed return URL: %v", err)
	}
	return u.Query()
}
