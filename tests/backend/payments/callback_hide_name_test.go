package payments

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"testing"

	"vanillapay/internal/merchant"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_CallbackHideNameReplacesSignedName(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	planID := db.SeedPlan(t, "hide name plan", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	fixture.Network.SetDNS("hide.example.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	var mu sync.Mutex
	var received []url.Values
	callbackURL := fixture.Network.ServeTLS(t, "hide.example.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.URL.Query())
		mu.Unlock()
		_, _ = w.Write([]byte("success"))
	}))
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	deliverPaid := func(outTradeNo string) (url.Values, url.Values) {
		t.Helper()
		order, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: outTradeNo, Name: "会员月卡",
			OriginalCents: 100, NotifyURL: callbackURL + "/notify", ReturnURL: "https://hide.example.test/return", Channel: "alipay", SourceIP: "192.0.2.78", Actor: actor,
		})
		if err != nil {
			t.Fatalf("create %s: %v", outTradeNo, err)
		}
		orderID, err := fixture.App.Services.Payment.OrderID(t.Context(), order.TradeNo)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.Pool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := fixture.App.Services.Payment.MarkPaidTx(t.Context(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
			_ = tx.Rollback(t.Context())
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := fixture.App.Services.Payment.ProcessCallback(t.Context(), orderID); err != nil {
			t.Fatalf("deliver %s: %v", outTradeNo, err)
		}
		mu.Lock()
		notice := received[len(received)-1]
		mu.Unlock()
		checkout := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+order.TradeNo, nil, "guest")
		var envelope struct {
			Data struct {
				ReturnURLSigned *string `json:"return_url_signed"`
			} `json:"data"`
		}
		if err := json.Unmarshal(checkout.Body.Bytes(), &envelope); err != nil || envelope.Data.ReturnURLSigned == nil {
			t.Fatalf("checkout lacks signed return: %s", checkout.Body.String())
		}
		signed, err := url.Parse(*envelope.Data.ReturnURLSigned)
		if err != nil {
			t.Fatal(err)
		}
		return notice, signed.Query()
	}

	settings, err := fixture.App.Services.Merchant.CallbackSettings(t.Context(), actor)
	if err != nil || settings.HideName {
		t.Fatalf("default callback settings = %#v err=%v", settings, err)
	}
	notice, returned := deliverPaid("hide-name-off")
	if notice.Get("name") != "会员月卡" || returned.Get("name") != "会员月卡" || !verifyV1Independent(notice, merchantKey) || !verifyV1Independent(returned, merchantKey) {
		t.Fatalf("default name callback=%q return=%q", notice.Get("name"), returned.Get("name"))
	}

	if _, err := fixture.App.Services.Merchant.SetCallbackSettings(t.Context(), actor, merchant.CallbackSettings{HideName: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.App.Services.Merchant.SetCallbackSettings(t.Context(), actor, merchant.CallbackSettings{HideName: true}); err != nil {
		t.Fatal(err)
	}
	var audits int
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='merchant.update_callback_settings' AND merchant_id=$1`, merchantID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("callback settings audits=%d err=%v; unchanged saves must not audit", audits, err)
	}
	notice, returned = deliverPaid("hide-name-on")
	if notice.Get("name") != "product" || returned.Get("name") != "product" || !verifyV1Independent(notice, merchantKey) || !verifyV1Independent(returned, merchantKey) {
		t.Fatalf("hidden name callback=%q return=%q", notice.Get("name"), returned.Get("name"))
	}
	if _, err := fixture.App.Services.Merchant.SetCallbackSettings(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, merchant.CallbackSettings{}); err == nil {
		t.Fatal("non-merchant actor changed callback settings")
	}
}
