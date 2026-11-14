package payments

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/merchant"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func riskMerchant(t *testing.T, f *testsupport.HTTPFixture, min, max, daily *int64) int64 {
	t.Helper()
	id, key := f.DB.SeedMerchant(t)
	plan := f.DB.SeedPlan(t, "Risk test", 1000, 365, []string{"alipay"}, nil)
	if _, err := f.DB.Pool.Exec(t.Context(), `UPDATE plans SET min_order_cents=$2,max_order_cents=$3,daily_limit_cents=$4 WHERE id=$1`, plan, min, max, daily); err != nil {
		t.Fatal(err)
	}
	f.DB.PrepareBillingMerchant(t, f.App, id, key, plan)
	return id
}

func riskOrder(id int64, out string, cents int64) payment.CreateCommand {
	return payment.CreateCommand{
		MerchantID: id, Protocol: "v1", Purpose: "external", OutTradeNo: out,
		Name: "Product", OriginalCents: cents, NotifyURL: "https://a.example.com/notify",
		ReturnURL: "https://a.b.example.com/return",
		Actor:     platform.Actor{Role: "merchant", SubjectID: id, MerchantID: id},
	}
}

func saveRisk(t *testing.T, f *testsupport.HTTPFixture, risk platform.PaymentRisk) error {
	t.Helper()
	if risk.IPBlacklist == nil {
		risk.IPBlacklist = []string{}
	}
	if risk.NameBlockWords == nil {
		risk.NameBlockWords = []string{}
	}
	var version int64
	if err := f.DB.Pool.QueryRow(t.Context(), `SELECT version FROM site_settings WHERE singleton`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(risk)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.App.Services.Settings.UpdateGroup(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, "risk", settings.UpdateGroup{ExpectedVersion: version, Value: data})
	return err
}

func requireRiskError(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" && err == nil {
		return
	}
	var problem *platform.Error
	if !errors.As(err, &problem) || problem.Code != want {
		t.Fatalf("error=%v, want %s", err, want)
	}
}

func TestDB_PlanRiskBoundsSnapshotAndDailyLimit(t *testing.T) {
	db := testsupport.NewDatabase(t)
	db.Clock.Set(time.Date(2026, 2, 3, 16, 10, 0, 0, time.UTC)) // Shanghai 00:10.
	f := testsupport.NewHTTPFixture(t, db)
	minimum, maximum, daily := int64(100), int64(300), int64(500)
	id := riskMerchant(t, f, &minimum, &maximum, &daily)
	create := f.App.Services.Payment.Create
	if _, err := db.Pool.Exec(t.Context(), `UPDATE plans SET min_order_cents=301,max_order_cents=301 WHERE id=(SELECT plan_id FROM subscriptions WHERE merchant_id=$1)`, id); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		out   string
		cents int64
		want  string
	}{
		{"below-min", 99, "AMOUNT_OUT_OF_RANGE"}, {"above-max", 301, "AMOUNT_OUT_OF_RANGE"},
		{"at-min", 100, ""}, {"at-max", 300, ""},
	} {
		_, err := create(t.Context(), riskOrder(id, tc.out, tc.cents))
		requireRiskError(t, err, tc.want)
	}
	for _, statement := range []string{
		`UPDATE orders SET created_at=$2::timestamptz-interval '1 hour' WHERE merchant_id=$1 AND out_trade_no='at-min'`,
		`UPDATE orders SET status='paid',paid_at=$2,channel='alipay',actual_cents=original_cents WHERE merchant_id=$1 AND out_trade_no='at-max'`,
	} {
		if _, err := db.Pool.Exec(t.Context(), statement, id, db.Clock.Now()); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ out, statement string }{
		{"yesterday-paid", `UPDATE orders SET status='paid',paid_at=$2::timestamptz-interval '1 hour',channel='alipay',actual_cents=original_cents WHERE trade_no=$1`},
		{"expired-pending", `UPDATE orders SET created_at=$2::timestamptz-interval '2 minutes',expires_at=$2::timestamptz-interval '1 minute' WHERE trade_no=$1`},
	} {
		order, err := create(t.Context(), riskOrder(id, tc.out, 100))
		requireRiskError(t, err, "")
		if _, err := db.Pool.Exec(t.Context(), tc.statement, order.TradeNo, db.Clock.Now()); err != nil {
			t.Fatal(err)
		}
	}
	_, err := create(t.Context(), riskOrder(id, "daily-plus-one", 101))
	requireRiskError(t, err, "DAILY_LIMIT_EXCEEDED")
	command := riskOrder(id, "at-daily", 100)
	order, err := create(t.Context(), command)
	requireRiskError(t, err, "")
	replay, err := create(t.Context(), command)
	if err != nil || replay.TradeNo != order.TradeNo {
		t.Fatalf("replay failed: %v", err)
	}
	legacy := riskMerchant(t, f, &minimum, &maximum, &daily)
	if _, err := db.Pool.Exec(t.Context(), `UPDATE subscriptions SET snapshot='{"channels":["alipay"]}' WHERE merchant_id=$1`, legacy); err != nil {
		t.Fatal(err)
	}
	_, err = create(t.Context(), riskOrder(legacy, "old-snapshot", 501))
	requireRiskError(t, err, "")
}

func TestDB_RiskLimitsSerializeParallelCreates(t *testing.T) {
	db := testsupport.NewDatabase(t)
	f := testsupport.NewHTTPFixture(t, db)
	daily := int64(300)
	withDaily := riskMerchant(t, f, nil, nil, &daily)
	second := riskMerchant(t, f, nil, nil, nil)
	third := riskMerchant(t, f, nil, nil, nil)
	for _, tc := range []struct {
		name, code string
		ipLimit    int32
		merchants  []int64
	}{
		{"merchant daily", "DAILY_LIMIT_EXCEEDED", 0, []int64{withDaily}},
		{"cross merchant IP", "IP_DAILY_LIMIT_EXCEEDED", 3, []int64{second, third}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireRiskError(t, saveRisk(t, f, platform.PaymentRisk{IPDailyOrderLimit: tc.ipLimit}), "")
			results := make(chan error, 8)
			var group sync.WaitGroup
			for i := range 8 {
				group.Go(func() {
					command := riskOrder(tc.merchants[i%len(tc.merchants)], fmt.Sprintf("parallel-%d", i), 100)
					command.ClientIP = "192.0.2.18"
					if i%2 == 1 {
						command.ClientIP = "::ffff:192.0.2.18"
					}
					if tc.ipLimit == 0 {
						command.ClientIP = ""
					}
					_, err := f.App.Services.Payment.Create(t.Context(), command)
					results <- err
				})
			}
			group.Wait()
			close(results)
			var succeeded int
			for err := range results {
				if err == nil {
					succeeded++
				} else {
					requireRiskError(t, err, tc.code)
				}
			}
			var count, total int
			if err := db.Pool.QueryRow(t.Context(), `SELECT count(*),COALESCE(sum(original_cents),0) FROM orders WHERE merchant_id=ANY($1) AND purpose='external'`, tc.merchants).Scan(&count, &total); err != nil || succeeded != 3 || count != 3 || total != 300 {
				t.Fatalf("successful=%d count=%d total=%d err=%v", succeeded, count, total, err)
			}
		})
	}
	_, err := f.App.Services.Payment.Create(t.Context(), riskOrder(second, "no-ip", 100))
	requireRiskError(t, err, "")
}

func TestHTTP_OrderRiskRulesAndCheckoutSourceIP(t *testing.T) {
	db := testsupport.NewDatabase(t)
	f := testsupport.NewHTTPFixtureWithTrustedProxyCIDRs(t, db, []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")})
	id := riskMerchant(t, f, nil, nil, nil)
	actor := riskOrder(id, "", 0).Actor
	rules, err := f.App.Services.Merchant.SetNotifyDomains(t.Context(), actor, merchant.NotifyDomains{
		Enabled: true, Entries: []string{"*.EXAMPLE.com", "例子.中国"},
	})
	if err != nil || strings.Join(rules.Entries, ",") != "*.example.com,xn--fsqu00a.xn--fiqs8s" {
		t.Fatalf("normalized allowlist=%v err=%v", rules, err)
	}
	requireRiskError(t, saveRisk(t, f, platform.PaymentRisk{
		IPBlacklist:    []string{"198.51.100.42/24", "2001:0DB8::3"},
		NameBlockWords: []string{"ＢｌｏｃｋＭｅ"},
	}), "")
	for i, tc := range []struct {
		ip, name, notify, back, want string
	}{
		{"198.51.100.1", "", "", "", "IP_BLOCKED"},
		{"2001:db8::3", "", "", "", "IP_BLOCKED"},
		{"2001:db8::4", strings.Repeat("a", 140) + "BLOCKme", "", "", "NAME_BLOCKED"},
		{"", "ｂｌｏｃｋｍｅ", "", "", "NAME_BLOCKED"},
		{"", "", "https://example.com/notify", "", "URL_DOMAIN_NOT_ALLOWED"},
		{"", "", "https://notexample.com/notify", "", "URL_DOMAIN_NOT_ALLOWED"},
		{"", "", "", "https://example.com/return", "URL_DOMAIN_NOT_ALLOWED"},
		{"", "", "https://example.com.evil.test/notify", "", "URL_DOMAIN_NOT_ALLOWED"},
		{"", "", "https://xn--fsqu00a.xn--fiqs8s/notify", "", ""},
	} {
		command := riskOrder(id, fmt.Sprintf("risk-rule-%d", i), 100)
		command.ClientIP = tc.ip
		if tc.name != "" {
			command.Name = tc.name
		}
		if tc.notify != "" {
			command.NotifyURL = tc.notify
		}
		if tc.back != "" {
			command.ReturnURL = tc.back
		}
		_, err := f.App.Services.Payment.Create(t.Context(), command)
		requireRiskError(t, err, tc.want)
	}
	order, err := f.App.Services.Payment.Create(t.Context(), riskOrder(id, "trusted-checkout", 100))
	requireRiskError(t, err, "")
	path := "/api/v1/checkout/" + order.TradeNo
	headers := http.Header{"X-Forwarded-For": {"198.51.100.20"}, "Content-Type": {"application/json"}}
	for _, response := range []*httptest.ResponseRecorder{
		f.RequestWithRemoteAddr(t, http.MethodGet, path, nil, "guest", headers, "192.0.2.1:443"),
		f.RequestWithRemoteAddr(t, http.MethodPost, path+"/channel", strings.NewReader(`{"channel":"alipay"}`), "guest", headers, "192.0.2.1:443"),
		f.RequestWithRemoteAddr(t, http.MethodGet, path, nil, "guest", nil, "[2001:db8::3]:443"),
	} {
		assertCheckoutProblem(t, response, 403, "IP_BLOCKED")
	}
	if response := f.RequestWithRemoteAddr(t, http.MethodGet, path, nil, "guest", headers, "203.0.113.9:443"); response.Code != 200 {
		t.Fatalf("untrusted forwarded IP affected checkout: %s", response.Body.String())
	}
}

func assertCheckoutProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var envelope struct {
		Error platform.Error `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != status || envelope.Error.Code != code {
		t.Fatalf("status=%d want=%d code=%q want=%q err=%v", response.Code, status, envelope.Error.Code, code, err)
	}
}

func TestHTTP_CheckoutVerificationCookieAndMedia(t *testing.T) {
	db := testsupport.NewDatabase(t)
	f := testsupport.NewHTTPFixture(t, db)
	id := riskMerchant(t, f, nil, nil, nil)
	risk := platform.PaymentRisk{CheckoutTurnstileEnabled: true}
	err := saveRisk(t, f, risk)
	requireRiskError(t, err, "VALIDATION_ERROR")
	var problem *platform.Error
	if !errors.As(err, &problem) || problem.Fields["value.checkout_turnstile_enabled"] == "" {
		t.Fatal("missing Turnstile field error")
	}
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
	create := f.App.Services.Payment.Create
	first := riskOrder(id, "verify-first", 100)
	first.Channel = "alipay"
	order, err := create(t.Context(), first)
	requireRiskError(t, err, "")
	other, err := create(t.Context(), riskOrder(id, "verify-other", 200))
	requireRiskError(t, err, "")
	path, otherPath := "/api/v1/checkout/"+order.TradeNo, "/api/v1/checkout/"+other.TradeNo
	get := func(path string, cookie *http.Cookie) payment.CheckoutDTO {
		t.Helper()
		headers := http.Header{}
		if cookie != nil {
			headers.Set("Cookie", cookie.Name+"="+cookie.Value)
		}
		response := f.RequestWithHeaders(t, http.MethodGet, path, nil, "guest", headers)
		var envelope struct {
			Data payment.CheckoutDTO `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != 200 {
			t.Fatalf("checkout GET: status=%d err=%v", response.Code, err)
		}
		return envelope.Data
	}
	oldURL := get(path, nil).QRImageURL
	if oldURL == nil {
		t.Fatal("fixture order has no QR URL before enabling verification")
	}
	var imagePath string
	if err := db.Pool.QueryRow(t.Context(), `SELECT file_path FROM media_assets WHERE owner_merchant_id=$1 AND channel='alipay'`, id).Scan(&imagePath); err != nil {
		t.Fatal(err)
	}
	imagePath = filepath.Join(f.DataDir, filepath.FromSlash(imagePath))
	if err := os.MkdirAll(filepath.Dir(imagePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("test QR asset"), 0600); err != nil {
		t.Fatal(err)
	}
	requireRiskError(t, saveRisk(t, f, risk), "")
	gated := get(path, nil)
	if gated.Verification == nil || !gated.Verification.Required || gated.Verification.SiteKey != "local-widget" ||
		gated.QRImageURL != nil || gated.ActualCents != nil || gated.CanPay || gated.Channel != nil || len(gated.AvailableChannels) != 0 ||
		gated.OriginalCents != 100 || gated.Name != "Product" || gated.TradeNo != order.TradeNo {
		t.Fatalf("gated checkout: %#v", gated)
	}
	if response := f.Request(t, http.MethodGet, *oldURL, nil, "guest"); response.Code != 404 {
		t.Fatalf("unverified previously issued QR URL returned %d", response.Code)
	}
	assertCheckoutProblem(t, f.Request(t, http.MethodPost, path+"/channel", map[string]string{"channel": "alipay"}, "guest"), 403, "CHECKOUT_VERIFICATION_REQUIRED")
	failed := f.Request(t, http.MethodPost, path+"/verify", map[string]string{"turnstile_token": "ok:login"}, "guest")
	if failed.Code == 204 || len(failed.Result().Cookies()) != 0 {
		t.Fatal("incorrect action verified checkout")
	}
	response := f.Request(t, http.MethodPost, path+"/verify", map[string]string{"turnstile_token": "ok:checkout"}, "guest")
	cookies := response.Result().Cookies()
	if response.Code != 204 || len(cookies) != 1 {
		t.Fatalf("verify status=%d cookie count=%d", response.Code, len(cookies))
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Expires.After(order.ExpiresAt) {
		t.Fatal("checkout cookie attributes incorrect")
	}
	verified := get(path, cookie)
	if verified.Verification == nil || verified.Verification.Required || !verified.CanPay || verified.QRImageURL == nil {
		t.Fatalf("verified checkout: %#v", verified)
	}
	headers := http.Header{"Content-Type": {"application/json"}, "Cookie": {cookie.Name + "=" + cookie.Value}}
	if selected := f.RequestWithHeaders(t, http.MethodPost, path+"/channel", strings.NewReader(`{"channel":"alipay"}`), "guest", headers); selected.Code != 200 {
		t.Fatalf("verified channel POST: %d", selected.Code)
	}
	if media := f.RequestWithHeaders(t, http.MethodGet, *verified.QRImageURL, nil, "guest", headers); media.Code != 200 {
		t.Fatalf("verified QR media: %d", media.Code)
	}
	forged := &http.Cookie{Name: platform.CheckoutCookieName(other.TradeNo), Value: cookie.Value}
	if dto := get(otherPath, forged); dto.Verification == nil || !dto.Verification.Required {
		t.Fatal("verification cookie accepted for another trade")
	}
	if _, err := db.Pool.Exec(t.Context(), `UPDATE orders SET expires_at=expires_at+interval '5 minutes' WHERE trade_no=$1`, order.TradeNo); err != nil {
		t.Fatal(err)
	}
	db.Clock.Set(other.ExpiresAt)
	if dto := get(path, cookie); dto.Verification == nil || !dto.Verification.Required {
		t.Fatal("expired cookie accepted for pending trade")
	}
	if dto := get(otherPath, nil); dto.Status != "expired" || dto.Verification != nil {
		t.Fatalf("terminal trade required verification: status=%s", dto.Status)
	}
	requireRiskError(t, saveRisk(t, f, platform.PaymentRisk{}), "")
	if dto := get(path, nil); dto.Verification != nil {
		t.Fatal("disabled verification still required")
	}
}
