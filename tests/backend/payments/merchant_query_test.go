package payments

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_CallbackSignsMergedMerchantQuery(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	merchantID, merchantKey := db.SeedMerchant(t)
	plan := db.SeedPlan(t, "merchant query plan", 1000, 365, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, plan)
	service := fixture.App.Services.Payment
	keys, err := service.PlatformPublicKeys(t.Context())
	if err != nil || len(keys) != 1 {
		t.Fatalf("read platform public key: %v", err)
	}
	block, _ := pem.Decode([]byte(keys[0]["pem"]))
	if block == nil {
		t.Fatal("invalid platform public key PEM")
	}
	public, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, ok := public.(*rsa.PublicKey)
	if !ok {
		t.Fatal("expected an RSA platform key")
	}

	cases := []struct {
		name, query string
		merchant    url.Values
		legacy      bool
	}{
		{name: "no-merchant-query"},
		{name: "merchant-route", query: "?route=primary", merchant: url.Values{"route": {"primary"}}},
		{name: "encoded-reserved-fields", query: "?%6F%75t_trade_no=x&out_trade_no=y&sign=old&clientip=old&key_id=old"},
		{name: "empty-merchant-value", query: "?a=", merchant: url.Values{"a": {""}}},
		{name: "legacy-repeated-key", query: "?route=primary&repeat=one&repeat=two", legacy: true},
		{name: "legacy-malformed-query", query: "?route=primary&bad=%ZZ", legacy: true},
	}
	for _, protocol := range []string{"v1", "v2"} {
		for _, test := range cases {
			outTrade := protocol + "-" + test.name
			t.Run(outTrade, func(t *testing.T) {
				received := make(chan url.Values, 1)
				receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					query := r.URL.Query()
					received <- query
					var verified bool
					if protocol == "v1" {
						verified = verifyV1Independent(query, merchantKey)
					} else {
						verified = query.Get("key_id") == keys[0]["id"] && verifyV2Independent(query, publicKey)
					}
					if r.Method != http.MethodGet || r.ContentLength != 0 || !verified {
						http.Error(w, "callback verification failed", http.StatusBadRequest)
						return
					}
					_, _ = w.Write([]byte(" Success "))
				}))
				defer receiver.Close()
				service.SetHTTPClient(platform.PublicHTTPClient{Dialer: callbackTestDialer(receiver.Listener.Addr().String())})
				const callbackURL = "http://93.184.216.34/notify"
				notifyURL := callbackURL + test.query
				if test.legacy {
					notifyURL = callbackURL
				}
				order, err := service.Create(t.Context(), payment.CreateCommand{
					MerchantID: merchantID, Protocol: protocol, Purpose: "external", OutTradeNo: outTrade,
					Name: "Merchant query order", OriginalCents: 200, NotifyURL: notifyURL, Channel: "alipay",
					Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID},
				})
				if err != nil {
					t.Fatal(err)
				}
				orderID, err := service.OrderID(t.Context(), order.TradeNo)
				if err != nil {
					t.Fatal(err)
				}
				if test.legacy {
					if _, err := db.Pool.Exec(t.Context(), `UPDATE orders SET notify_url=$1 WHERE id=$2`, callbackURL+test.query, orderID); err != nil {
						t.Fatal(err)
					}
				}
				tx, err := db.Pool.Begin(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if err := service.MarkPaidTx(t.Context(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
					_ = tx.Rollback(t.Context())
					t.Fatal(err)
				}
				if err := tx.Commit(t.Context()); err != nil {
					t.Fatal(err)
				}
				if err := service.ProcessCallback(t.Context(), orderID); err != nil {
					t.Fatalf("deliver signed callback: %v", err)
				}
				var state string
				if err := db.Pool.QueryRow(t.Context(), `SELECT state FROM callback_events WHERE order_id=$1`, orderID).Scan(&state); err != nil || state != "success" {
					t.Fatalf("callback state=%q err=%v", state, err)
				}
				select {
				case query := <-received:
					if len(query["out_trade_no"]) != 1 || query.Get("out_trade_no") != outTrade || query.Has("clientip") {
						t.Fatal("callback did not replace the reserved query parameters")
					}
					// Check absence as well as empty values, which Encode must preserve.
					for _, key := range []string{"route", "a", "repeat", "bad"} {
						if query.Has(key) != test.merchant.Has(key) || query.Get(key) != test.merchant.Get(key) {
							t.Errorf("unexpected merchant parameter %q", key)
						}
					}
					if protocol == "v1" {
						want := 9 + len(test.merchant) // Classic fields including sign and sign_type.
						if len(query) != want || query.Get("sign_type") != "MD5" {
							t.Error("V1 callback did not preserve its classic field set")
						}
					} else if query.Get("sign_type") != "RSA" || !query.Has("addtime") || !query.Has("endtime") {
						t.Error("V2 callback did not preserve its extended fields")
					}
				default:
					t.Fatal("callback did not reach the receiver")
				}
			})
		}
	}
}

// Redirect the validated public address to the httptest receiver.
type callbackTestDialer string

func (d callbackTestDialer) DialContext(ctx context.Context, network, _ string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, string(d))
}

func TestHTTP_CreateRejectsUnsignableNotifyURL(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	merchantID, merchantKey := db.SeedMerchant(t)
	plan := db.SeedPlan(t, "notify query validation", 1000, 365, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, plan)
	pid, private := protocolMerchantKey(t, db, merchantID)

	for i, raw := range []string{"route=one&r%6Fute=two", "route=one;two", "route=%ZZ", "route=%FF"} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			outPrefix := "invalid-notify-" + strconv.Itoa(i) + "-"
			params := url.Values{
				"pid":        {pid},
				"notify_url": {"https://merchant.example.test/notify?" + raw},
				"return_url": {"https://merchant.example.test/return"},
				"name":       {"Invalid notify URL"}, "money": {"1.00"}, "clientip": {"192.0.2.5"},
			}
			t.Run("mapi", func(t *testing.T) {
				params.Set("out_trade_no", outPrefix+"mapi")
				params.Set("sign", mustV1ContractSignature(params, merchantKey))
				if got := postCompatForm(t, fixture, "/mapi.php", params); got["error_code"] != "INVALID_URL" {
					t.Error("V1 mapi accepted an unsignable notify URL")
				}
			})
			t.Run("submit", func(t *testing.T) {
				params.Set("out_trade_no", outPrefix+"submit")
				params.Set("sign", mustV1ContractSignature(params, merchantKey))
				submit := fixture.Request(t, http.MethodGet, "/submit.php?"+params.Encode(), nil, "guest")
				if submit.Code != http.StatusOK || !strings.HasPrefix(submit.Header().Get("Content-Type"), "text/html") ||
					!strings.Contains(submit.Body.String(), "无法发起支付") || !strings.Contains(submit.Body.String(), "<code>INVALID_URL</code>") {
					t.Error("V1 submit did not render the notify URL error page")
				}
			})
			t.Run("v2-create", func(t *testing.T) {
				params.Set("out_trade_no", outPrefix+"v2")
				if got := postCompatV2(t, fixture, "/api/pay/create", pid, private, params); got["error_code"] != "INVALID_URL" {
					t.Error("V2 create accepted an unsignable notify URL")
				}
			})
			var count int
			if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no LIKE $2`, merchantID, outPrefix+"%").Scan(&count); err != nil || count != 0 {
				t.Fatalf("rejected orders persisted: count=%d err=%v", count, err)
			}
		})
	}
}

func cloneValues(values url.Values) url.Values {
	out := url.Values{}
	for key, entries := range values {
		out[key] = append([]string(nil), entries...)
	}
	return out
}
