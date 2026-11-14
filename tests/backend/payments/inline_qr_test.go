package payments

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_ProtocolInlineQR(t *testing.T) {
	const (
		origin   = "https://pay.example.test"
		clientIP = "198.51.100.80"
	)
	db := testsupport.NewDatabase(t)
	db.Clock.Set(db.Clock.Now().Truncate(time.Second))
	fixture := testsupport.NewHTTPFixture(t, db)
	id, key := db.SeedMerchant(t)
	otherID, otherKey := db.SeedMerchant(t)
	pid, private := protocolMerchantKey(t, db, id)
	otherPID, otherPrivate := protocolMerchantKey(t, db, otherID)
	channels := []struct {
		name, payload string
	}{
		{"alipay", "https://qr.alipay.com/inline-test"},
		{"wxpay", "wxp://inline-test"},
	}
	plan := db.SeedPlan(t, "inline QR", 1000, 365, []string{"alipay", "wxpay"}, nil)
	token := db.PrepareBillingMerchant(t, fixture.App, id, key, plan)
	actor := platform.Actor{Role: "merchant", SubjectID: id, MerchantID: id}
	for _, channel := range channels {
		_, err := fixture.App.Services.Settings.UploadQR(t.Context(), actor, channel.name, bytes.NewReader(inlineQRTestPNG(t, channel.payload)))
		if err != nil {
			t.Fatalf("upload %s fixture QR: %v", channel.name, err)
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `UPDATE merchant_collection_settings
		SET float_limit_cents=1,float_direction='down' WHERE merchant_id=$1`, id); err != nil {
		t.Fatal(err)
	}

	create := func(t *testing.T, protocol, out, channel, money string, optIn, tamper bool, peer string) map[string]any {
		t.Helper()
		params := url.Values{
			"pid": {pid}, "out_trade_no": {out}, "type": {channel}, "money": {money},
			"name": {"Inline QR test"}, "notify_url": {"https://merchant.example.test/notify"}, "clientip": {clientIP},
		}
		if optIn {
			params.Set("inline_qr", "1")
		}
		path := "/mapi.php"
		if protocol == "v2" {
			path = "/api/pay/create"
			params.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
			params.Set("sign_type", "RSA")
			params.Set("sign", signV2Independent(t, params, private))
		} else {
			params.Set("sign", mustV1ContractSignature(params, key))
		}
		if tamper {
			params.Set("inline_qr", "1")
		}
		res := fixture.RequestWithRemoteAddr(t, http.MethodPost, path, strings.NewReader(params.Encode()), "guest",
			http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, peer)
		data := decodeCompatResponse(t, res.Code, res.Body.Bytes())
		if protocol == "v2" {
			verifyPlatformV2Response(t, fixture, res.Body.Bytes())
		}
		return data
	}
	query := func(t *testing.T, protocol, trade string, foreign bool) map[string]any {
		t.Helper()
		params := url.Values{"trade_no": {trade}}
		if protocol == "v2" {
			if foreign {
				return postCompatV2(t, fixture, "/api/pay/query", otherPID, otherPrivate, params)
			}
			return postCompatV2(t, fixture, "/api/pay/query", pid, private, params)
		}
		params.Set("act", "order")
		if foreign {
			return getCompatV1(t, fixture, otherPID, otherKey, params)
		}
		return getCompatV1(t, fixture, pid, key, params)
	}
	assertCashier := func(t *testing.T, protocol string, data map[string]any, inline bool) string {
		t.Helper()
		trade, _ := data["trade_no"].(string)
		wantCode := float64(1)
		if protocol == "v2" {
			wantCode = 0
			if data["pay_type"] != "jump" || data["pay_info"] != data["payurl"] {
				t.Fatalf("invalid V2 cashier response: %v", data)
			}
		}
		if trade == "" || data["code"] != wantCode || data["payurl"] != origin+"/pay/"+url.PathEscape(trade) {
			t.Fatalf("invalid %s cashier response: %v", protocol, data)
		}
		for _, field := range []string{"qrcode", "actual_money", "type", "expires_at"} {
			if _, present := data[field]; present != inline {
				t.Errorf("%s response field %q present=%t, want %t", protocol, field, present, inline)
			}
		}
		return trade
	}
	assertImage := func(t *testing.T, data map[string]any, payload string) *url.URL {
		t.Helper()
		raw, _ := data["qrcode"].(string)
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host != "pay.example.test" || !strings.HasPrefix(u.Path, "/media/") {
			t.Fatalf("expected absolute QR media URL, got %q", raw)
		}
		res := fixture.Request(t, http.MethodGet, u.RequestURI(), nil, "guest")
		if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "image/png" ||
			!strings.Contains(res.Header().Get("Cache-Control"), "no-store") {
			t.Fatalf("QR media: HTTP %d, Content-Type=%q, Cache-Control=%q", res.Code, res.Header().Get("Content-Type"), res.Header().Get("Cache-Control"))
		}
		img, err := png.Decode(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
		if err != nil {
			t.Fatal(err)
		}
		qr, err := qrcode.NewQRCodeReader().DecodeWithoutHints(bitmap)
		if err != nil || qr.GetText() != payload {
			t.Fatalf("QR payload=%v, want %q, err=%v", qr, payload, err)
		}
		return u
	}
	assertMediaDenied := func(t *testing.T, u *url.URL, peer string) {
		t.Helper()
		res := fixture.RequestWithRemoteAddr(t, http.MethodGet, u.RequestURI(), nil, "guest", nil, peer)
		assertCheckoutProblem(t, res, http.StatusNotFound, "NOT_FOUND")
	}

	protocols := []struct {
		name, money, actual string
		actualCents         int64
	}{
		{"v1", "1.00", "0.99", 99},
		{"v2", "2.00", "1.99", 199},
	}
	for i, protocol := range protocols {
		for j, channel := range channels {
			t.Run(protocol.name+"/"+channel.name, func(t *testing.T) {
				out := protocol.name + "-" + channel.name
				legacy := create(t, protocol.name, out+"-legacy", channel.name, protocol.money, false, false, "")
				legacyTrade := assertCashier(t, protocol.name, legacy, false)
				bad := create(t, protocol.name, out+"-bad-sign", channel.name, protocol.money, false, true, "")
				if bad["code"] != float64(-1) || bad["error_code"] != "INVALID_SIGNATURE" || bad["qrcode"] != nil {
					t.Fatalf("adding inline_qr after signing: %v", bad)
				}
				data := create(t, protocol.name, out, channel.name, protocol.money, true, false, "")
				trade := assertCashier(t, protocol.name, data, true)
				var legacyCents int64
				var expires time.Time
				err := db.Pool.QueryRow(t.Context(), `SELECT legacy.actual_cents, current_order.expires_at
					FROM orders legacy CROSS JOIN orders current_order
					WHERE legacy.trade_no=$1 AND current_order.trade_no=$2`, legacyTrade, trade).Scan(&legacyCents, &expires)
				rawExpiry, _ := data["expires_at"].(string)
				gotExpiry, parseErr := time.Parse(time.RFC3339Nano, rawExpiry)
				if err != nil || parseErr != nil || !gotExpiry.Equal(expires) || !gotExpiry.After(db.Clock.Now()) ||
					legacyCents != protocol.actualCents+1 || trade == legacyTrade ||
					data["actual_money"] != protocol.actual || data["type"] != channel.name {
					t.Fatalf("unexpected reservation or expiry: legacy=%d response=%v expiry=%s DB err=%v parse err=%v",
						legacyCents, data, expires, err, parseErr)
				}
				u := assertImage(t, data, channel.payload)
				retry := create(t, protocol.name, out, channel.name, protocol.money, true, false, "")
				for _, field := range []string{"trade_no", "payurl", "qrcode", "actual_money", "type", "expires_at"} {
					if retry[field] != data[field] {
						t.Errorf("retry changed %s: got %v, want %v", field, retry[field], data[field])
					}
				}
				rebound := *u
				values := rebound.Query()
				values.Set("trade_no", legacyTrade)
				rebound.RawQuery = values.Encode()
				assertMediaDenied(t, &rebound, "")
				if foreign := query(t, protocol.name, trade, true); foreign["code"] != float64(-1) || foreign["error_code"] != "NOT_FOUND" ||
					foreign["trade_no"] != nil || foreign["actual_money"] != nil || foreign["qrcode"] != nil {
					t.Fatalf("cross-merchant query exposed order: %v", foreign)
				}

				receipt := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", device.ReceiptRequest{
					EventID: fmt.Sprintf("e0000000-0000-4000-8000-%012d", 1+i*len(channels)+j), Channel: channel.name,
					AmountCents: protocol.actualCents, OccurredAt: db.Clock.Now(),
				}, "device:"+token)
				var matched struct {
					Data device.ReceiptResult `json:"data"`
				}
				if receipt.Code != http.StatusCreated || json.Unmarshal(receipt.Body.Bytes(), &matched) != nil ||
					matched.Data.MatchState != "matched" || matched.Data.TradeNo == nil || *matched.Data.TradeNo != trade {
					t.Fatalf("device receipt: HTTP %d %s", receipt.Code, receipt.Body.String())
				}
				assertMediaDenied(t, u, "")
				paid := query(t, protocol.name, trade, false)
				if paid["status"] != float64(1) || paid["order_status"] != "paid" || paid["trade_no"] != trade ||
					paid["type"] != channel.name || paid["money"] != protocol.money || paid["actual_money"] != protocol.actual {
					t.Fatalf("paid query: %v", paid)
				}
			})
		}
	}

	t.Run("IP blacklist", func(t *testing.T) {
		data := create(t, "v1", "inline-ip", "alipay", "3.00", true, false, "")
		u := assertImage(t, data, channels[0].payload)
		const blockedPeer = "203.0.113.8:443"
		requireRiskError(t, saveRisk(t, fixture, platform.PaymentRisk{IPBlacklist: []string{"203.0.113.8"}}), "")
		assertMediaDenied(t, u, blockedPeer)
		assertImage(t, data, channels[0].payload)
		assertCashier(t, "v1", create(t, "v1", "inline-ip-new", "alipay", "4.00", true, false, blockedPeer), false)
		requireRiskError(t, saveRisk(t, fixture, platform.PaymentRisk{IPBlacklist: []string{clientIP}}), "")
		assertMediaDenied(t, u, "")
		rejected := create(t, "v1", "inline-ip-blocked-client", "alipay", "4.00", true, false, "")
		if rejected["code"] != float64(-1) || rejected["error_code"] != "IP_BLOCKED" || rejected["qrcode"] != nil {
			t.Fatalf("blocked signed client IP: %v", rejected)
		}
		requireRiskError(t, saveRisk(t, fixture, platform.PaymentRisk{}), "")
	})

	t.Run("Turnstile", func(t *testing.T) {
		secret, err := platform.Encrypt(db.Key[:], []byte("local-siteverify"), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Pool.Exec(t.Context(), `INSERT INTO secret_settings(name,encrypted_value) VALUES('turnstile_secret',$1)`, secret); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Pool.Exec(t.Context(), `UPDATE site_settings SET security=security || '{"turnstile_site_key":"local-widget"}'::jsonb WHERE singleton`); err != nil {
			t.Fatal(err)
		}
		oldURLs := make(map[string]*url.URL)
		for _, protocol := range protocols {
			data := create(t, protocol.name, protocol.name+"-pre-turnstile", "wxpay", "5.00", true, false, "")
			oldURLs[protocol.name] = assertImage(t, data, channels[1].payload)
		}
		requireRiskError(t, saveRisk(t, fixture, platform.PaymentRisk{CheckoutTurnstileEnabled: true}), "")
		for _, protocol := range protocols {
			oldURL := oldURLs[protocol.name]
			assertMediaDenied(t, oldURL, "")
			assertCashier(t, protocol.name, create(t, protocol.name, protocol.name+"-pre-turnstile", "wxpay", "5.00", true, false, ""), false)
			data := create(t, protocol.name, protocol.name+"-turnstile", "wxpay", "6.00", true, false, "")
			trade := assertCashier(t, protocol.name, data, false)
			res := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+trade, nil, "guest")
			var checkout struct{ Data payment.CheckoutDTO }
			if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &checkout) != nil ||
				checkout.Data.Verification == nil || !checkout.Data.Verification.Required ||
				checkout.Data.QRImageURL != nil || checkout.Data.CanPay {
				t.Fatalf("Turnstile cashier: HTTP %d %s", res.Code, res.Body.String())
			}
		}
		requireRiskError(t, saveRisk(t, fixture, platform.PaymentRisk{}), "")
	})

	t.Run("cancelled", func(t *testing.T) {
		data := create(t, "v2", "inline-cancelled", "alipay", "7.00", true, false, "")
		trade := assertCashier(t, "v2", data, true)
		u := assertImage(t, data, channels[0].payload)
		closeOrder := postCompatV2(t, fixture, "/api/pay/close", pid, private, url.Values{"trade_no": {trade}})
		if closeOrder["code"] != float64(0) {
			t.Fatalf("cancel order: %v", closeOrder)
		}
		cancelled := query(t, "v2", trade, false)
		if cancelled["order_status"] != "expired" || cancelled["expire_reason"] != "cancelled" {
			t.Fatalf("cancelled query: %v", cancelled)
		}
		assertMediaDenied(t, u, "")
	})

	t.Run("timed out", func(t *testing.T) {
		data := create(t, "v1", "inline-timeout", "alipay", "8.00", true, false, "")
		trade := assertCashier(t, "v1", data, true)
		u := assertImage(t, data, channels[0].payload)
		rawExpiry, _ := data["expires_at"].(string)
		expires, err := time.Parse(time.RFC3339Nano, rawExpiry)
		if err != nil {
			t.Fatal(err)
		}
		db.Clock.Set(expires.Add(time.Second))
		if _, err := db.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2`, db.Clock.Now(), id); err != nil {
			t.Fatal(err)
		}
		assertMediaDenied(t, u, "")
		if _, err := fixture.App.Services.Payment.ExpireBatch(t.Context(), 100); err != nil {
			t.Fatal(err)
		}
		if expired := query(t, "v1", trade, false); expired["order_status"] != "expired" || expired["expire_reason"] != "timeout" {
			t.Fatalf("timed-out query: %v", expired)
		}
		assertMediaDenied(t, u, "")
	})
}

func inlineQRTestPNG(t *testing.T, payload string) []byte {
	t.Helper()
	matrix, err := qrcode.NewQRCodeWriter().Encode(payload, gozxing.BarcodeFormat_QR_CODE, 256, 256, nil)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewPaletted(image.Rect(0, 0, matrix.GetWidth(), matrix.GetHeight()), color.Palette{color.White, color.Black})
	for y := range matrix.GetHeight() {
		for x := range matrix.GetWidth() {
			if matrix.Get(x, y) {
				img.SetColorIndex(x, y, 1)
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
