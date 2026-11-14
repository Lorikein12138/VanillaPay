package payments

import (
	"net/http"
	"net/netip"
	"sync/atomic"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_CallbackFailureAlertOncePerEpisode(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	planID := db.SeedPlan(t, "callback alert plan", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	fixture.Network.SetDNS("alert.example.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	var failing atomic.Bool
	failing.Store(true)
	callbackURL := fixture.Network.ServeTLS(t, "alert.example.test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			http.Error(w, "receiver down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("success"))
	}))
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	paidOrder := func(outTradeNo string) (int64, string) {
		t.Helper()
		order, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: outTradeNo, Name: "alert order",
			OriginalCents: 100, NotifyURL: callbackURL + "/notify", Channel: "alipay", SourceIP: "192.0.2.77", Actor: actor,
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
			t.Fatalf("mark %s paid: %v", outTradeNo, err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		return orderID, order.TradeNo
	}
	deliver := func(orderID int64, times int) {
		t.Helper()
		for range times {
			if _, err := db.Pool.Exec(t.Context(), `UPDATE callback_events SET next_attempt_at=$1 WHERE order_id=$2 AND state IN ('pending','failed')`, db.Clock.Now(), orderID); err != nil {
				t.Fatal(err)
			}
			_ = fixture.App.Services.Payment.ProcessCallback(t.Context(), orderID)
		}
	}
	alerts := func() (notifications, mails int, armed bool) {
		t.Helper()
		if err := db.Pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM notifications WHERE merchant_id=$1 AND kind='callback_failed'),(SELECT count(*) FROM mail_deliveries WHERE template='callback_failed'),(SELECT callback_alert_armed FROM merchants WHERE id=$1)`, merchantID).Scan(&notifications, &mails, &armed); err != nil {
			t.Fatal(err)
		}
		return notifications, mails, armed
	}

	first, firstTrade := paidOrder("alert-first")
	deliver(first, 2)
	if n, m, armed := alerts(); n != 0 || m != 0 || !armed {
		t.Fatalf("two failures raised an alert: notifications=%d mails=%d armed=%v", n, m, armed)
	}
	deliver(first, 1)
	if n, m, armed := alerts(); n != 1 || m != 1 || armed {
		t.Fatalf("third failure alert: notifications=%d mails=%d armed=%v", n, m, armed)
	}
	var payload string
	if err := db.Pool.QueryRow(t.Context(), `SELECT safe_payload->'action'->>'entity_id' FROM notifications WHERE merchant_id=$1 AND kind='callback_failed'`, merchantID).Scan(&payload); err != nil || payload != firstTrade {
		t.Fatalf("alert action entity=%q err=%v", payload, err)
	}
	deliver(first, 2)
	second, _ := paidOrder("alert-second")
	deliver(second, 3)
	if n, m, _ := alerts(); n != 1 || m != 1 {
		t.Fatalf("same failure episode alerted again: notifications=%d mails=%d", n, m)
	}

	failing.Store(false)
	deliver(second, 1)
	if _, _, armed := alerts(); !armed {
		t.Fatal("successful callback did not re-arm the alert")
	}
	failing.Store(true)
	third, _ := paidOrder("alert-third")
	deliver(third, 3)
	if n, m, armed := alerts(); n != 2 || m != 2 || armed {
		t.Fatalf("new failure episode alert: notifications=%d mails=%d armed=%v", n, m, armed)
	}
}
