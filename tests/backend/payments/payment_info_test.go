package payments

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

const paymentInfoBuyerIP = "198.51.100.80"

var paymentInfoChannels = []struct{ name, payload string }{
	{"alipay", "https://qr.alipay.com/payment-info-test"},
	{"wxpay", "wxp://payment-info-test"},
}

type paymentInfoFixture struct {
	*testsupport.HTTPFixture
	merchantID      int64
	pid, key, token string
	notifyURL       string
}

func newPaymentInfoFixture(t *testing.T) *paymentInfoFixture {
	t.Helper()
	db := testsupport.NewDatabase(t)
	db.Clock.Set(db.Clock.Now().Truncate(time.Second).Add(123456789 * time.Nanosecond))
	f := testsupport.NewHTTPFixture(t, db)
	id, key := db.SeedMerchant(t)
	plan := db.SeedPlan(t, "payment info", 1000, 365, []string{"alipay", "wxpay"}, nil)
	token := db.PrepareBillingMerchant(t, f.App, id, key, plan)
	actor := platform.Actor{Role: "merchant", SubjectID: id, MerchantID: id}
	for _, channel := range paymentInfoChannels {
		if _, err := f.App.Services.Settings.UploadQR(t.Context(), actor, channel.name,
			bytes.NewReader(inlineQRTestPNG(t, channel.payload))); err != nil {
			t.Fatalf("upload %s QR: %v", channel.name, err)
		}
	}
	var pid string
	if err := db.Pool.QueryRow(t.Context(), `SELECT pid FROM merchants WHERE id=$1`, id).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return &paymentInfoFixture{HTTPFixture: f, merchantID: id, pid: pid, key: key, token: token,
		notifyURL: "https://merchant.example.test/notify"}
}

// Only the classic V1 fields and their original MD5 signature are sent.
func (f *paymentInfoFixture) create(t *testing.T, out, channel, money, buyerIP, peer string) map[string]any {
	t.Helper()
	params := url.Values{
		"pid": {f.pid}, "out_trade_no": {out}, "type": {channel}, "money": {money},
		"name": {"Payment info + 测试"}, "notify_url": {f.notifyURL}, "clientip": {buyerIP},
		"return_url": {"https://merchant.example.test/return"}, "sign_type": {"MD5"},
	}
	params.Set("sign", mustV1ContractSignature(params, f.key))
	res := f.RequestWithRemoteAddr(t, http.MethodPost, "/mapi.php", strings.NewReader(params.Encode()), "guest",
		http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, peer)
	data := decodeCompatResponse(t, res.Code, res.Body.Bytes())
	if info, ok := data["payment_info"].(map[string]any); ok {
		var expires time.Time
		if err := f.DB.Pool.QueryRow(t.Context(), `SELECT expires_at FROM orders WHERE trade_no=$1`, data["trade_no"]).Scan(&expires); err != nil {
			t.Fatal(err)
		}
		deadline, _ := info["expires_at"].(string)
		if deadline != expires.UTC().Format(time.RFC3339Nano) {
			t.Fatal("creation response did not use the persisted absolute deadline")
		}
	}
	return data
}

func assertPaymentInfo(t *testing.T, data map[string]any, amount string, cashier bool) map[string]any {
	t.Helper()
	trade, _ := data["trade_no"].(string)
	if data["code"] != float64(1) || data["msg"] != "success" || trade == "" ||
		data["payurl"] != "https://pay.example.test/pay/"+url.PathEscape(trade) {
		t.Fatalf("invalid V1 success or same-order cashier; code=%v error=%v", data["code"], data["error_code"])
	}
	info, ok := data["payment_info"].(map[string]any)
	if !ok || info["version"] != float64(1) || info["requires_cashier"] != cashier {
		t.Fatal("invalid payment_info version or cashier requirement")
	}
	for name, value := range info {
		if value == nil {
			t.Fatalf("payment_info contains null %s", name)
		}
		switch name {
		case "version", "actual_amount", "qr_image_url", "expires_at", "requires_cashier":
		default:
			t.Fatalf("unexpected payment_info field %s", name)
		}
	}
	actual, present := info["actual_amount"]
	if present != (amount != "") || present && actual != amount {
		t.Fatalf("actual_amount=%v present=%t, want %q", actual, present, amount)
	}
	raw, hasImage := info["qr_image_url"].(string)
	if cashier && (hasImage || info["qr_content"] != nil || data["qrcode"] != nil) || !cashier && (!hasImage || raw == "") {
		t.Fatal("payment code presence disagrees with cashier requirement")
	}
	expiry, ok := info["expires_at"].(string)
	if _, err := time.Parse(time.RFC3339Nano, expiry); !ok || err != nil {
		t.Fatal("payment_info expiry is not RFC3339")
	}
	return info
}

func (f *paymentInfoFixture) denyImage(t *testing.T, u *url.URL, peer string) {
	t.Helper()
	res := f.RequestWithRemoteAddr(t, http.MethodGet, u.RequestURI(), nil, "guest", nil, peer)
	assertCheckoutProblem(t, res, http.StatusNotFound, "NOT_FOUND")
}

func assertPaymentInfoError(t *testing.T, data map[string]any, code string) {
	t.Helper()
	if data["code"] != float64(-1) || data["error_code"] != code || data["payment_info"] != nil ||
		data["qrcode"] != nil || data["payurl"] != nil {
		t.Fatalf("expected unchanged V1 error %s without payment instructions; code=%v error=%v", code, data["code"], data["error_code"])
	}
}

func logRedactedPaymentInfo(t *testing.T, data map[string]any) {
	t.Helper()
	info := data["payment_info"].(map[string]any)
	safeInfo := map[string]any{"version": info["version"], "expires_at": info["expires_at"], "requires_cashier": info["requires_cashier"]}
	if amount, ok := info["actual_amount"]; ok {
		safeInfo["actual_amount"] = amount
	}
	if _, ok := info["qr_image_url"]; ok {
		safeInfo["qr_image_url"] = "https://pay.example.test/media/ASSET_REDACTED?inline=1&trade_no=TRADE_REDACTED&expires=EXPIRY_REDACTED&signature=REDACTED"
	}
	safe := map[string]any{"code": data["code"], "msg": data["msg"], "trade_no": "TRADE_REDACTED",
		"payurl": "https://pay.example.test/pay/TRADE_REDACTED", "payment_info": safeInfo}
	encoded, err := json.Marshal(safe)
	if err != nil {
		t.Fatal("encode redacted payment_info response")
	}
	t.Logf("redacted creation response: %s", encoded)
}

func TestHTTP_ProtocolPaymentInfoAmounts(t *testing.T) {
	for i, channel := range paymentInfoChannels {
		for j, scenario := range []struct {
			name, direction, amount string
			cents                   int64
			reservations            int
		}{
			{"exact", "up", "1.00", 100, 0},
			{"up_one", "up", "1.01", 101, 1},
			{"up_two", "up", "1.02", 102, 2},
			{"down", "down", "0.99", 99, 1},
		} {
			t.Run(channel.name+"/"+scenario.name, func(t *testing.T) {
				f := newPaymentInfoFixture(t)
				f.Network.SetDNS("merchant.example.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
				var mu sync.Mutex
				callbacks := map[string]url.Values{}
				f.notifyURL = f.Network.ServeTLS(t, "merchant.example.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					values := r.URL.Query()
					mu.Lock()
					callbacks[values.Get("trade_no")] = values
					mu.Unlock()
					_, _ = w.Write([]byte("success"))
				})) + "/notify"
				if _, err := f.DB.Pool.Exec(t.Context(), `UPDATE merchant_collection_settings
					SET float_limit_cents=2,float_direction=$1 WHERE merchant_id=$2`, scenario.direction, f.merchantID); err != nil {
					t.Fatal(err)
				}
				for n := range scenario.reservations {
					reserved := f.create(t, fmt.Sprintf("reserved-%d", n), channel.name, "1.00", paymentInfoBuyerIP, "")
					if reserved["code"] != float64(1) {
						t.Fatal("reserve actual-amount slot through standard V1 HTTP failed")
					}
				}
				out := channel.name + "-" + scenario.name
				data := f.create(t, out, channel.name, "1.00", paymentInfoBuyerIP, "")
				info := assertPaymentInfo(t, data, scenario.amount, false)
				if scenario.name == "up_two" {
					logRedactedPaymentInfo(t, data)
				}
				trade := data["trade_no"].(string)
				if scenario.cents == 100 {
					if data["qrcode"] != channel.payload {
						t.Fatal("unadjusted legacy qrcode must remain the raw channel payload")
					}
				} else if _, present := data["qrcode"]; present {
					t.Fatal("adjusted order leaked legacy qrcode")
				}
				u := inlineQRTestImage(t, f.HTTPFixture, info["qr_image_url"].(string), channel.payload)
				if u.Query().Get("trade_no") != trade || u.Query().Get("inline") != "1" ||
					u.Query().Get("signature") == "" || u.Query().Get("expires") != info["expires_at"] {
					t.Fatal("image capability is not scoped to the order and absolute expiry")
				}
				var original, actual int64
				var expires time.Time
				if err := f.DB.Pool.QueryRow(t.Context(), `SELECT original_cents,actual_cents,expires_at
					FROM orders WHERE trade_no=$1`, trade).Scan(&original, &actual, &expires); err != nil {
					t.Fatal(err)
				}
				gotExpiry, _ := time.Parse(time.RFC3339Nano, info["expires_at"].(string))
				if original != 100 || actual != scenario.cents || !gotExpiry.Equal(expires) || expires.Nanosecond() == 0 {
					t.Fatal("payment_info does not reflect the independently stored actual amount and original deadline")
				}
				f.DB.Clock.Set(f.DB.Clock.Now().Add(time.Second))
				retry := f.create(t, out, channel.name, "1.00", paymentInfoBuyerIP, "")
				if !reflect.DeepEqual(retry, data) {
					t.Fatal("same-order retry changed payment fields, amount, image, or absolute expiry")
				}
				receipt := f.Request(t, http.MethodPost, "/api/v1/device/receipts", device.ReceiptRequest{
					EventID: fmt.Sprintf("f1000000-0000-4000-8000-%012d", 1+i*4+j), Channel: channel.name,
					AmountCents: scenario.cents, OccurredAt: f.DB.Clock.Now(),
				}, "device:"+f.token)
				var matched struct {
					Data device.ReceiptResult `json:"data"`
				}
				if receipt.Code != http.StatusCreated || json.Unmarshal(receipt.Body.Bytes(), &matched) != nil ||
					matched.Data.MatchState != "matched" || matched.Data.TradeNo == nil || *matched.Data.TradeNo != trade {
					t.Fatalf("actual-amount device receipt did not match this order; HTTP %d", receipt.Code)
				}
				paid := getCompatV1(t, f.HTTPFixture, f.pid, f.key, url.Values{"act": {"order"}, "trade_no": {trade}})
				if paid["status"] != float64(1) || paid["order_status"] != "paid" || paid["money"] != "1.00" ||
					paid["actual_money"] != scenario.amount || paid["trade_no"] != trade {
					t.Fatal("paid query changed original money or did not preserve actual_money")
				}
				orderID, err := f.App.Services.Payment.OrderID(t.Context(), trade)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.App.Services.Payment.ProcessCallback(t.Context(), orderID); err != nil {
					t.Fatal(err)
				}
				mu.Lock()
				notice := callbacks[trade]
				mu.Unlock()
				if notice.Get("money") != "1.00" || notice.Get("trade_no") != trade || notice.Get("type") != channel.name ||
					notice.Get("trade_status") != "TRADE_SUCCESS" || !verifyV1Independent(notice, f.key) {
					t.Fatal("callback did not retain original money and classic MD5 signature")
				}
				f.denyImage(t, u, "")
				assertPaymentInfoError(t, f.create(t, out, channel.name, "1.00", paymentInfoBuyerIP, ""), "DUPLICATE_ORDER")
			})
		}
	}
}

func TestHTTP_ProtocolPaymentInfoSafety(t *testing.T) {
	f := newPaymentInfoFixture(t)
	secret, err := platform.Encrypt(f.DB.Key[:], []byte("local-siteverify"), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Pool.Exec(t.Context(), `INSERT INTO secret_settings(name,encrypted_value) VALUES('turnstile_secret',$1)`, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Pool.Exec(t.Context(), `UPDATE site_settings SET security=security || '{"turnstile_site_key":"local-widget"}'::jsonb WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	for _, channel := range paymentInfoChannels {
		t.Run(channel.name, func(t *testing.T) {
			out := channel.name + "-safety"
			data := f.create(t, out, channel.name, "3.00", paymentInfoBuyerIP, "")
			info := assertPaymentInfo(t, data, "3.00", false)
			image := inlineQRTestImage(t, f.HTTPFixture, info["qr_image_url"].(string), channel.payload)
			trade := data["trade_no"].(string)
			other := f.create(t, out+"-other", channel.name, "4.00", paymentInfoBuyerIP, "")
			assertPaymentInfo(t, other, "4.00", false)
			for _, field := range []string{"signature", "trade_no", "expires"} {
				t.Run("tamper_"+field, func(t *testing.T) {
					tampered := *image
					values := tampered.Query()
					switch field {
					case "signature":
						values.Set(field, "tampered")
					case "trade_no":
						values.Set(field, other["trade_no"].(string))
					case "expires":
						values.Set(field, f.DB.Clock.Now().Add(time.Hour).Format(time.RFC3339Nano))
					}
					tampered.RawQuery = values.Encode()
					f.denyImage(t, &tampered, "")
				})
			}
			assertFallback := func(t *testing.T, result map[string]any) {
				t.Helper()
				fallback := assertPaymentInfo(t, result, "3.00", true)
				if result["trade_no"] != trade || fallback["expires_at"] != info["expires_at"] {
					t.Fatal("cashier fallback changed the existing order or deadline")
				}
			}
			t.Run("CF", func(t *testing.T) {
				requireRiskError(t, saveRisk(t, f.HTTPFixture, platform.PaymentRisk{CheckoutTurnstileEnabled: true}), "")
				f.denyImage(t, image, "")
				assertFallback(t, f.create(t, out, channel.name, "3.00", paymentInfoBuyerIP, ""))
				created := f.create(t, out+"-cf", channel.name, "5.00", paymentInfoBuyerIP, "")
				assertPaymentInfo(t, created, "5.00", true)
				logRedactedPaymentInfo(t, created)
				requireRiskError(t, saveRisk(t, f.HTTPFixture, platform.PaymentRisk{}), "")
			})
			t.Run("source_IP", func(t *testing.T) {
				const peer = "203.0.113.8:443"
				requireRiskError(t, saveRisk(t, f.HTTPFixture, platform.PaymentRisk{IPBlacklist: []string{"203.0.113.8"}}), "")
				f.denyImage(t, image, peer)
				assertFallback(t, f.create(t, out, channel.name, "3.00", paymentInfoBuyerIP, peer))
				assertPaymentInfo(t, f.create(t, out+"-ip", channel.name, "6.00", paymentInfoBuyerIP, peer), "6.00", true)
				inlineQRTestImage(t, f.HTTPFixture, info["qr_image_url"].(string), channel.payload)
				requireRiskError(t, saveRisk(t, f.HTTPFixture, platform.PaymentRisk{}), "")
			})
			t.Run("stored_buyer_IP", func(t *testing.T) {
				requireRiskError(t, saveRisk(t, f.HTTPFixture, platform.PaymentRisk{IPBlacklist: []string{paymentInfoBuyerIP}}), "")
				f.denyImage(t, image, "203.0.113.9:443")
				assertFallback(t, f.create(t, out, channel.name, "3.00", "198.51.100.81", "203.0.113.9:443"))
				assertPaymentInfoError(t, f.create(t, out+"-blocked", channel.name, "7.00", paymentInfoBuyerIP, ""), "IP_BLOCKED")
				requireRiskError(t, saveRisk(t, f.HTTPFixture, platform.PaymentRisk{}), "")
			})
			t.Run("offline", func(t *testing.T) {
				if _, err := f.DB.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2`, f.DB.Clock.Now().Add(-90*time.Second), f.merchantID); err != nil {
					t.Fatal(err)
				}
				f.denyImage(t, image, "")
				assertFallback(t, f.create(t, out, channel.name, "3.00", paymentInfoBuyerIP, ""))
				assertPaymentInfoError(t, f.create(t, out+"-offline", channel.name, "8.00", paymentInfoBuyerIP, ""), "DEVICE_OFFLINE")
				if _, err := f.DB.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2`, f.DB.Clock.Now(), f.merchantID); err != nil {
					t.Fatal(err)
				}
			})
			if retry := f.create(t, out, channel.name, "3.00", paymentInfoBuyerIP, ""); !reflect.DeepEqual(retry, data) {
				t.Fatal("restoring eligibility changed the original payment information")
			}
		})
	}
	t.Run("unselected_channel", func(t *testing.T) {
		data := f.create(t, "unselected", "", "9.00", paymentInfoBuyerIP, "")
		trade, _ := data["trade_no"].(string)
		if trade == "" || data["payurl"] != "https://pay.example.test/pay/"+trade {
			t.Fatal("unselected channel did not return its own cashier link")
		}
		for _, field := range []string{"payment_info", "qrcode"} {
			if _, present := data[field]; present {
				t.Fatalf("unselected channel unexpectedly returned %s", field)
			}
		}
	})
}

type paymentInfoRetryClock struct {
	first, deadline time.Time
}

func (c *paymentInfoRetryClock) Now() time.Time {
	now := c.first
	c.first = c.deadline
	return now
}

func TestHTTP_ProtocolPaymentInfoExpiresDuringRetry(t *testing.T) {
	f := newPaymentInfoFixture(t)
	const out = "expires-during-retry"
	data := f.create(t, out, "alipay", "1.00", paymentInfoBuyerIP, "")
	info := assertPaymentInfo(t, data, "1.00", false)
	deadline, err := time.Parse(time.RFC3339Nano, info["expires_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	clock := &paymentInfoRetryClock{first: deadline.Add(-time.Microsecond), deadline: deadline}
	h := payment.Handler{Service: payment.NewService(f.DB.Pool, f.DB.Key, "https://pay.example.test", clock, nil, nil, nil), PublicBaseURL: "https://pay.example.test"}
	params := url.Values{
		"pid": {f.pid}, "out_trade_no": {out}, "type": {"alipay"}, "money": {"1.00"},
		"name": {"Payment info + 测试"}, "notify_url": {f.notifyURL}, "clientip": {paymentInfoBuyerIP},
		"return_url": {"https://merchant.example.test/return"}, "sign_type": {"MD5"},
	}
	params.Set("sign", mustV1ContractSignature(params, f.key))
	req := httptest.NewRequest(http.MethodPost, "/mapi.php", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	h.V1MAPI(res, req)
	retry := decodeCompatResponse(t, res.Code, res.Body.Bytes())
	fallback := assertPaymentInfo(t, retry, "1.00", true)
	if retry["trade_no"] != data["trade_no"] || fallback["expires_at"] != info["expires_at"] {
		t.Fatal("retry did not retain the order and its absolute deadline")
	}
	var status string
	if err := f.DB.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE trade_no=$1`, data["trade_no"]).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("presenting an expired order changed its stored state to %q", status)
	}
}

func TestHTTP_ProtocolPaymentInfoLifecycle(t *testing.T) {
	f := newPaymentInfoFixture(t)
	pid, private := protocolMerchantKey(t, f.DB, f.merchantID)
	for _, channel := range paymentInfoChannels {
		for _, state := range []string{"cancelled", "timeout"} {
			t.Run(channel.name+"/"+state, func(t *testing.T) {
				if _, err := f.DB.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2`, f.DB.Clock.Now(), f.merchantID); err != nil {
					t.Fatal(err)
				}
				money := "10.00"
				if state == "timeout" {
					money = "11.00"
				}
				out := channel.name + "-" + state
				data := f.create(t, out, channel.name, money, paymentInfoBuyerIP, "")
				info := assertPaymentInfo(t, data, money, false)
				u := inlineQRTestImage(t, f.HTTPFixture, info["qr_image_url"].(string), channel.payload)
				trade := data["trade_no"].(string)
				if state == "cancelled" {
					closed := postCompatV2(t, f.HTTPFixture, "/api/pay/close", pid, private, url.Values{"trade_no": {trade}})
					if closed["code"] != float64(0) {
						t.Fatal("cancel payment-info order failed")
					}
				} else {
					expires, _ := time.Parse(time.RFC3339Nano, info["expires_at"].(string))
					f.DB.Clock.Set(expires.Add(-time.Microsecond))
					if _, err := f.DB.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2`, f.DB.Clock.Now(), f.merchantID); err != nil {
						t.Fatal(err)
					}
					inlineQRTestImage(t, f.HTTPFixture, info["qr_image_url"].(string), channel.payload)
					f.DB.Clock.Set(expires)
					f.denyImage(t, u, "")
					assertPaymentInfoError(t, f.create(t, out, channel.name, money, paymentInfoBuyerIP, ""), "DUPLICATE_ORDER")
					if _, err := f.App.Services.Payment.ExpireBatch(t.Context(), 100); err != nil {
						t.Fatal(err)
					}
				}
				f.denyImage(t, u, "")
				assertPaymentInfoError(t, f.create(t, out, channel.name, money, paymentInfoBuyerIP, ""), "DUPLICATE_ORDER")
				got := getCompatV1(t, f.HTTPFixture, f.pid, f.key, url.Values{"act": {"order"}, "trade_no": {trade}})
				if got["order_status"] != "expired" || got["expire_reason"] != state || got["payurl"] != nil {
					t.Fatal("terminal order still exposes a payment entry or has the wrong state")
				}
				var expiry time.Time
				if err := f.DB.Pool.QueryRow(t.Context(), `SELECT expires_at FROM orders WHERE trade_no=$1`, trade).Scan(&expiry); err != nil {
					t.Fatal(err)
				}
				original, _ := time.Parse(time.RFC3339Nano, info["expires_at"].(string))
				if !expiry.Equal(original) {
					t.Fatal("terminal or expired retries renewed the order")
				}
			})
		}
	}
}

// Exercise the schema pattern through kin-openapi, rather than a separate regexp.
func TestUnit_ProtocolPaymentInfoAmountSchema(t *testing.T) {
	schema := testsupport.OpenAPI(t).Components.Schemas["V1PaymentInfo"].Value
	for _, sample := range []struct {
		amount string
		valid  bool
	}{
		{"1", true}, {"1.00", true}, {"1.02", true}, {"0.99", true}, {"0.01", true},
		{"0.1", true}, {"999999999999.99", true}, {"0", false}, {"0.00", false},
		{"0.0", false}, {"01.00", false}, {"1.001", false}, {"1e2", false}, {"-1", false},
		{"+1", false}, {"1000000000000", false}, {"1\n", false},
	} {
		t.Run(sample.amount, func(t *testing.T) {
			value := map[string]any{"version": 1, "actual_amount": sample.amount,
				"expires_at": "2030-01-01T12:00:00Z", "requires_cashier": true}
			if err := schema.VisitJSON(value); (err == nil) != sample.valid {
				t.Fatalf("amount %q valid=%t, schema error=%v", sample.amount, sample.valid, err)
			}
		})
	}
	for _, field := range []string{"actual_amount", "qr_image_url"} {
		if err := schema.VisitJSON(map[string]any{"version": 1, "expires_at": "2030-01-01T12:00:00Z", "requires_cashier": true, field: nil}); err == nil {
			t.Errorf("schema accepted null %s", field)
		}
	}
	if err := schema.VisitJSON(map[string]any{"version": 1, "actual_amount": "1.00", "expires_at": "2030-01-01T12:00:00Z", "requires_cashier": true, "unexpected": true}); err == nil {
		t.Error("payment_info schema is not closed")
	}
}
