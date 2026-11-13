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
	"net/http/httptest"
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
		seen.values = append(seen.values, r.URL.Query())
		seen.mu.Unlock()
		_, _ = w.Write([]byte("success"))
	}))
	const preserved = "order_id=38&resume_token=resume&status=success"
	const collisions = "&out_trade_no=old&%6fut_trade_no=older&sign=old&param=old&key_id=old&clientip=old"
	notifyURL := callbackURL + "/notify?" + preserved + collisions
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	createWithReturnURL := func(protocol, outTrade, returnURL string) (payment.OrderDTO, error) {
		t.Helper()
		return fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: merchantID, Protocol: protocol, Purpose: "external", OutTradeNo: outTrade, Name: "One dollar order",
			OriginalCents: 100, NotifyURL: notifyURL, ReturnURL: returnURL, ClientIP: "198.51.100.80", SourceIP: "192.0.2.80",
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
	var pid string
	if err := db.Pool.QueryRow(t.Context(), `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&pid); err != nil {
		t.Fatal("read protocol PID")
	}
	v1Params := url.Values{"pid": {pid}, "out_trade_no": {"return-v1-amount"}, "notify_url": {notifyURL}, "return_url": {"https://api.lorikein.com/payment/result?campaign=original&" + preserved + collisions}, "name": {"One dollar order"}, "money": {"1.00"}, "type": {"alipay"}, "clientip": {"198.51.100.80"}}
	v1Params.Set("sign", mustV1ContractSignature(v1Params, merchantKey))
	v1Create := fixture.Request(t, http.MethodGet, "/submit.php?"+v1Params.Encode(), nil, "guest")
	if v1Create.Code != http.StatusFound || !strings.HasPrefix(v1Create.Header().Get("Location"), "/pay/") {
		t.Fatalf("V1 submit status=%d body=%s", v1Create.Code, v1Create.Body.String())
	}
	v1 := payment.OrderDTO{TradeNo: strings.TrimPrefix(v1Create.Header().Get("Location"), "/pay/")}
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
	assertCompatibleQuery(t, v1Notice, "return-v1-amount")
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
	assertCompatibleQuery(t, v1Return.Query(), "return-v1-amount")
	// sub2api confirms a notification by querying act=order with every
	// parameter in both the query and the form body; identical values are one
	// parameter and must not be rejected as INVALID_SIGNATURE.
	sub2apiQuery := url.Values{"act": {"order"}, "pid": {pid}, "key": {merchantKey}, "out_trade_no": {"return-v1-amount"}}
	sub2apiResponse := fixture.RequestWithHeaders(t, http.MethodPost, "/api.php?"+sub2apiQuery.Encode(), strings.NewReader(sub2apiQuery.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	var sub2apiData map[string]json.RawMessage
	if sub2apiResponse.Code != http.StatusOK || json.Unmarshal(sub2apiResponse.Body.Bytes(), &sub2apiData) != nil || string(sub2apiData["code"]) != "1" || string(sub2apiData["status"]) != "1" || string(sub2apiData["money"]) != `"1.00"` || string(sub2apiData["trade_no"]) != `"`+v1.TradeNo+`"` {
		t.Fatalf("sub2api-style V1 query response=%d body=%s", sub2apiResponse.Code, sub2apiResponse.Body.String())
	}
	conflicting := fixture.RequestWithHeaders(t, http.MethodPost, "/api.php?"+sub2apiQuery.Encode(), strings.NewReader("act=order&pid="+pid+"&key="+url.QueryEscape(merchantKey)+"&out_trade_no=other"), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	assertEchoRejected(t, conflicting, "V1 api.php query with differing query and form values")
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
		v1Params.Set("return_url", "javascript:alert(1)")
		v1Params.Set("name", "<script>alert(1)</script>")
		v1Params.Set("key", "do-not-echo")
		v1Params.Set("sign", mustV1ContractSignature(v1Params, "rotated-v1-key-after-order"))
		for _, path := range []string{"/submit.php", "/mapi.php"} {
			res := fixture.RequestWithHeaders(t, http.MethodPost, path, strings.NewReader(v1Params.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
			if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "INVALID_URL") {
				t.Fatalf("invalid URL %s: status=%d body=%s", path, res.Code, res.Body.String())
			}
			if path == "/mapi.php" {
				if !strings.HasPrefix(res.Header().Get("Content-Type"), "application/json") {
					t.Fatal("mapi response must be JSON")
				}
				continue
			}
			if res.Header().Get("Content-Type") != "text/html; charset=utf-8" || res.Header().Get("Cache-Control") != "no-store" || strings.Contains(res.Body.String(), "<script>") || strings.Contains(res.Body.String(), "do-not-echo") || strings.Contains(res.Body.String(), v1Params.Get("sign")) {
				t.Fatal("submit error is unsafe or not HTML")
			}
		}
	})

	// Only POST /api.php tolerates an echoed query; the contract forbids any
	// query/form duplicate on the V1 create entries, even with equal values.
	t.Run("V1 create rejects echoed query", func(t *testing.T) {
		echo := url.Values{"pid": {pid}, "out_trade_no": {"return-v1-echo"}, "notify_url": {notifyURL}, "return_url": {"https://merchant.example.test/return"}, "name": {"Echo order"}, "money": {"2.00"}, "type": {"alipay"}, "clientip": {"198.51.100.80"}}
		echo.Set("sign", mustV1ContractSignature(echo, "rotated-v1-key-after-order"))
		query := url.Values{"sign": {echo.Get("sign")}}.Encode()
		mapi := fixture.RequestWithHeaders(t, http.MethodPost, "/mapi.php?"+query, strings.NewReader(echo.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
		assertEchoRejected(t, mapi, "V1 mapi.php with an echoed query")
		submit := fixture.RequestWithHeaders(t, http.MethodPost, "/submit.php?"+query, strings.NewReader(echo.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
		if submit.Code != http.StatusOK || submit.Header().Get("Content-Type") != "text/html; charset=utf-8" || !strings.Contains(submit.Body.String(), "<code>INVALID_SIGNATURE</code>") {
			t.Fatalf("V1 submit.php with an echoed query: status=%d location=%q body=%s", submit.Code, submit.Header().Get("Location"), submit.Body.String())
		}
		accepted := fixture.RequestWithHeaders(t, http.MethodPost, "/mapi.php", strings.NewReader(echo.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
		if accepted.Code != http.StatusOK || !strings.Contains(accepted.Body.String(), `"code":1`) {
			t.Fatalf("the same V1 form without the echo must be valid: status=%d body=%s", accepted.Code, accepted.Body.String())
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
	v2Params := url.Values{"pid": {pid}, "out_trade_no": {"return-v2-amount"}, "notify_url": {notifyURL}, "return_url": {"https://merchant.example.test/v2-return?" + preserved + collisions}, "name": {"One dollar order"}, "money": {"1.00"}, "type": {"alipay"}, "clientip": {"198.51.100.80"}, "timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}}
	v2Params.Set("sign", signV2Independent(t, v2Params, privateKey))
	v2Create := fixture.RequestWithHeaders(t, http.MethodPost, "/api/pay/submit", strings.NewReader(v2Params.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	if v2Create.Code != http.StatusSeeOther || !strings.HasPrefix(v2Create.Header().Get("Location"), "/pay/") {
		t.Fatalf("V2 submit status=%d body=%s", v2Create.Code, v2Create.Body.String())
	}
	v2 := payment.OrderDTO{TradeNo: strings.TrimPrefix(v2Create.Header().Get("Location"), "/pay/")}
	v2Params.Set("notify_url", "https://localhost/notify")
	v2Params.Set("sign", signV2Independent(t, v2Params, privateKey))
	invalidV2 := fixture.RequestWithHeaders(t, http.MethodPost, "/api/pay/create", strings.NewReader(v2Params.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	if invalidV2.Code != http.StatusOK || !strings.Contains(invalidV2.Body.String(), "INVALID_URL") {
		t.Fatalf("invalid V2 notify URL: status=%d body=%s", invalidV2.Code, invalidV2.Body.String())
	}
	v2Query := url.Values{"pid": {pid}, "trade_no": {v2.TradeNo}, "timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}}
	v2Query.Set("sign", signV2Independent(t, v2Query, privateKey))
	v2QueryResponse := fixture.RequestWithHeaders(t, http.MethodPost, "/api/pay/query", strings.NewReader(v2Query.Encode()), "guest", http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}})
	var v2QueryData map[string]any
	if v2QueryResponse.Code != http.StatusOK || json.Unmarshal(v2QueryResponse.Body.Bytes(), &v2QueryData) != nil || v2QueryData["money"] != "1.00" || v2QueryData["actual_money"] != "0.99" {
		t.Fatalf("V2 query amount response=%d body=%s; want money 1.00 and actual_money 0.99", v2QueryResponse.Code, v2QueryResponse.Body.String())
	}
	verifyPlatformV2Response(t, fixture, v2QueryResponse.Body.Bytes())
	// V2 entries reject any query/form duplicate, even with equal values; each
	// form below is valid on its own, so only the echoed sign can fail it.
	v2Echo := url.Values{"pid": {pid}, "out_trade_no": {"return-v2-echo"}, "notify_url": {notifyURL}, "return_url": {"https://merchant.example.test/v2-return"}, "name": {"Echo order"}, "money": {"1.00"}, "type": {"alipay"}, "clientip": {"198.51.100.80"}, "timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}}
	v2Echo.Set("sign", signV2Independent(t, v2Echo, privateKey))
	v2RefundEcho := url.Values{"pid": {pid}, "trade_no": {v2.TradeNo}, "money": {"1.00"}, "timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}}
	v2RefundEcho.Set("sign", signV2Independent(t, v2RefundEcho, privateKey))
	for path, form := range map[string]url.Values{"/api/pay/query": v2Query, "/api/pay/create": v2Echo, "/api/pay/submit": v2Echo, "/api/pay/refund": v2RefundEcho} {
		echoed := fixture.RequestWithHeaders(t, http.MethodPost, path+"?"+url.Values{"sign": {form.Get("sign")}}.Encode(), strings.NewReader(form.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
		assertEchoRejected(t, echoed, "V2 "+path+" with an echoed query")
	}
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
	assertCompatibleQuery(t, v2Notice, "return-v2-amount")
	v2Checkout := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+v2.TradeNo, nil, "guest")
	if v2Checkout.Code != http.StatusOK || json.Unmarshal(v2Checkout.Body.Bytes(), &reservedData) != nil {
		t.Fatalf("read paid V2 checkout: status=%d body=%s", v2Checkout.Code, v2Checkout.Body.String())
	}
	if json.Unmarshal(reservedData.Data["return_url_signed"], &signedURL) != nil || !strings.HasPrefix(signedURL, "https://merchant.example.test/v2-return?") {
		t.Fatalf("V2 signed return did not preserve allowed target: %s", signedURL)
	}
	v2ReturnValues := mustParseURLQuery(t, signedURL)
	assertCompatibleQuery(t, v2ReturnValues, "return-v2-amount")
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

func assertCompatibleQuery(t *testing.T, signed url.Values, outTrade string) {
	t.Helper()
	if signed.Get("order_id") != "38" || signed.Get("resume_token") != "resume" || signed.Get("status") != "success" {
		t.Fatal("lost merchant routing parameters")
	}
	if len(signed["out_trade_no"]) != 1 || signed.Get("out_trade_no") != outTrade || len(signed["sign"]) != 1 || signed.Get("sign") == "old" || signed.Has("param") || signed.Get("key_id") == "old" {
		t.Fatal("platform parameters did not replace merchant collisions")
	}
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
		if verifyV2Independent(values, pub) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("V2 response signature did not verify against published platform keys: %#v", values)
	}
}

func verifyV2Independent(values url.Values, pub *rsa.PublicKey) bool {
	signature, err := base64.StdEncoding.DecodeString(values.Get("sign"))
	if err != nil {
		return false
	}
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
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], signature) == nil
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
		default:
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			values.Set(key, string(encoded))
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

// assertEchoRejected requires the exact protocol rejection for a request whose
// query string repeats a form parameter that the endpoint does not tolerate.
func assertEchoRejected(t *testing.T, res *httptest.ResponseRecorder, what string) {
	t.Helper()
	var body map[string]json.RawMessage
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &body) != nil || string(body["code"]) != "-1" || string(body["error_code"]) != `"INVALID_SIGNATURE"` {
		t.Fatalf("%s: status=%d body=%s; want 200 code=-1 error_code=INVALID_SIGNATURE", what, res.Code, res.Body.String())
	}
}
