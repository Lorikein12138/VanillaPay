package payments

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

// Merchant routing segments the callback must forward byte-for-byte: a %20
// escape, a semicolon segment that url.ParseQuery would drop, and a repeated
// merchant key. Reserved fields, including percent-encoded and repeated
// clientip, must be removed without touching the rest.
const (
	rawMerchantRouting  = "order_id=38&routing=primary%20route&legacy=first;second&route=one&route=two"
	rawReservedCollides = "&out_trade_no=old&%6fut_trade_no=older&sign=old&clientip=old&clientip=dup&%63lientip=enc&key_id=old"
)

func TestHTTP_MerchantQueryKeptRawAndReservedFieldsReplaced(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	planID := db.SeedPlan(t, "merchant query plan", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	fixture.Network.SetDNS("merchant.example.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("generate V2 merchant key")
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: mustMarshalPKIX(t, &privateKey.PublicKey)})
	if _, err := db.Pool.Exec(t.Context(), `UPDATE merchants SET rsa_public_key_pem=$1 WHERE id=$2`, string(publicPEM), merchantID); err != nil {
		t.Fatal("install V2 merchant public key")
	}

	type delivery struct {
		rawQuery string
		form     url.Values
	}
	var mu sync.Mutex
	var deliveries []delivery
	callbackURL := fixture.Network.ServeTLS(t, "merchant.example.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routing, signed, found := strings.Cut(r.URL.RawQuery, "&")
		if strings.HasPrefix(r.URL.RawQuery, rawMerchantRouting+"&") {
			routing, signed, found = rawMerchantRouting, strings.TrimPrefix(r.URL.RawQuery, rawMerchantRouting+"&"), true
		}
		form, err := url.ParseQuery(signed)
		if !found || err != nil || r.Method != http.MethodGet || r.ContentLength != 0 {
			http.Error(w, "invalid callback query", http.StatusBadRequest)
			return
		}
		mu.Lock()
		deliveries = append(deliveries, delivery{rawQuery: routing, form: form})
		mu.Unlock()
		_, _ = w.Write([]byte(" Success "))
	}))
	notifyURL := callbackURL + "/notify?" + rawMerchantRouting + rawReservedCollides
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	cents := int64(200)
	createOrder := func(t *testing.T, protocol, outTrade, returnURL, clientIP string) (payment.OrderDTO, error) {
		t.Helper()
		cents++
		return fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: merchantID, Protocol: protocol, Purpose: "external", OutTradeNo: outTrade, Name: "Merchant query order",
			OriginalCents: cents, NotifyURL: notifyURL, ReturnURL: returnURL, ClientIP: clientIP, SourceIP: "192.0.2.80",
			Channel: "alipay", Actor: actor,
		})
	}
	payAndNotify := func(t *testing.T, order payment.OrderDTO) delivery {
		t.Helper()
		orderID, err := fixture.App.Services.Payment.OrderID(t.Context(), order.TradeNo)
		if err != nil {
			t.Fatalf("lookup order %s: %v", order.TradeNo, err)
		}
		tx, err := db.Pool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := fixture.App.Services.Payment.MarkPaidTx(t.Context(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
			_ = tx.Rollback(t.Context())
			t.Fatalf("mark %s paid: %v", order.TradeNo, err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := fixture.App.Services.Payment.ProcessCallback(t.Context(), orderID); err != nil {
			t.Fatalf("deliver callback for %s: %v", order.TradeNo, err)
		}
		mu.Lock()
		defer mu.Unlock()
		return deliveries[len(deliveries)-1]
	}
	signedReturn := func(t *testing.T, order payment.OrderDTO) url.Values {
		t.Helper()
		response := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+order.TradeNo, nil, "guest")
		var envelope struct {
			Data struct {
				ReturnURLSigned *string `json:"return_url_signed"`
			} `json:"data"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.Data.ReturnURLSigned == nil {
			t.Fatalf("paid checkout for %s has no signed return: %d %s", order.TradeNo, response.Code, response.Body.String())
		}
		return mustParseURLQuery(t, *envelope.Data.ReturnURLSigned)
	}

	for _, protocol := range []string{"v1", "v2"} {
		t.Run(protocol+"/callback query raw with order IP", func(t *testing.T) {
			order, err := createOrder(t, protocol, "query-ip-"+protocol, "", "198.51.100.80")
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			got := payAndNotify(t, order)
			if got.rawQuery != rawMerchantRouting {
				t.Fatalf("callback raw query = %q, want %q", got.rawQuery, rawMerchantRouting)
			}
			if got.form.Get("out_trade_no") != "query-ip-"+protocol {
				t.Fatalf("callback form out_trade_no = %q", got.form.Get("out_trade_no"))
			}
			if protocol == "v1" {
				// Strict epay merchants (sub2api) reject any key outside the classic set.
				assertV1Fields(t, got.form, "param")
			} else if got.form.Get("clientip") != "198.51.100.80" || len(got.form["clientip"]) != 1 || !got.form.Has("addtime") || !got.form.Has("endtime") {
				t.Fatalf("V2 callback form lacks extended fields: %v", got.form)
			}
			verifyCallbackForm(t, fixture, protocol, got.form, merchantKey)
		})
		t.Run(protocol+"/callback without order IP omits clientip", func(t *testing.T) {
			order, err := createOrder(t, protocol, "query-noip-"+protocol, "", "")
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			got := payAndNotify(t, order)
			if got.rawQuery != rawMerchantRouting || got.form.Has("clientip") {
				t.Fatalf("callback without IP: raw query %q, form clientip %q", got.rawQuery, got.form["clientip"])
			}
			verifyCallbackForm(t, fixture, protocol, got.form, merchantKey)
		})
		t.Run(protocol+"/return drops stale clientip and signs", func(t *testing.T) {
			order, err := createOrder(t, protocol, "query-return-"+protocol, "https://merchant.example.test/return?order_id=38&status=success"+rawReservedCollides, "198.51.100.80")
			if err != nil {
				t.Fatalf("create with reserved duplicates in return URL: %v", err)
			}
			payAndNotify(t, order)
			values := signedReturn(t, order)
			if values.Has("clientip") || values.Get("order_id") != "38" || values.Get("status") != "success" || len(values["out_trade_no"]) != 1 || values.Get("out_trade_no") != "query-return-"+protocol {
				t.Fatalf("signed return values = %v", values)
			}
			if protocol == "v1" {
				values := cloneValues(values)
				values.Del("order_id")
				values.Del("status")
				assertV1Fields(t, values, "param")
			}
			verifyCallbackForm(t, fixture, protocol, values, merchantKey)
		})
		t.Run(protocol+"/unsignable repeated merchant key is rejected", func(t *testing.T) {
			out := "query-dup-" + protocol
			_, err := createOrder(t, protocol, out, "https://merchant.example.test/return?route=one&route=two", "198.51.100.80")
			if errorCode(err) != "INVALID_URL" {
				t.Fatalf("repeated merchant return key error = %v, want INVALID_URL", err)
			}
			var count int
			if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no=$2`, merchantID, out).Scan(&count); err != nil || count != 0 {
				t.Fatalf("rejected order persisted count=%d err=%v", count, err)
			}
		})
	}
}

// v1Fields is the classic epay notify field set; param is sent only when set.
var v1Fields = []string{"pid", "trade_no", "out_trade_no", "type", "name", "money", "trade_status", "sign", "sign_type"}

func assertV1Fields(t *testing.T, values url.Values, optional ...string) {
	t.Helper()
	allowed := map[string]bool{}
	for _, key := range v1Fields {
		allowed[key] = true
		if !values.Has(key) {
			t.Fatalf("V1 values lack %q: %v", key, values)
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range values {
		if !allowed[key] {
			t.Fatalf("V1 values carry non-standard field %q: %v", key, values)
		}
	}
}

func cloneValues(values url.Values) url.Values {
	out := url.Values{}
	for key, entries := range values {
		out[key] = append([]string(nil), entries...)
	}
	return out
}

func verifyCallbackForm(t *testing.T, fixture *testsupport.HTTPFixture, protocol string, values url.Values, merchantKey string) {
	t.Helper()
	for key, entries := range values {
		if len(entries) != 1 {
			t.Fatalf("signed values repeat %q: %v", key, entries)
		}
	}
	if protocol == "v1" {
		if !verifyV1Independent(values, merchantKey) {
			t.Fatalf("V1 signature does not verify: %v", values)
		}
		return
	}
	if strings.TrimSpace(values.Get("key_id")) == "" || values.Get("key_id") == "old" {
		t.Fatalf("V2 key_id = %q", values.Get("key_id"))
	}
	verifyPlatformV2Values(t, fixture, values)
}
