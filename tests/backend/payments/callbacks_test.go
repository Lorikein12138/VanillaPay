package payments

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_CallbackReplayCreatesDurableGenerationAndDeduplicatesRequest(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	orderID := seedPendingExternalOrder(t, db, merchantID, "callback-trade", "callback-out", "https://merchant.example/notify")
	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := application.Services.Payment.MarkPaidTx(context.Background(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(context.Background(), `UPDATE callback_events SET state='failed' WHERE order_id=$1 AND generation=1`, orderID); err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	first, err := application.Services.Payment.ReplayCallback(context.Background(), actor, "callback-trade", "callback-replay-001")
	if err != nil || first["callback_status"] != "pending" {
		t.Fatalf("callback replay = %#v, %v", first, err)
	}
	replayed, err := application.Services.Payment.ReplayCallback(context.Background(), actor, "callback-trade", "callback-replay-001")
	if err != nil || replayed["event_id"] != first["event_id"] {
		t.Fatalf("idempotent callback replay = %#v, %v", replayed, err)
	}
	if _, err := application.Services.Payment.ReplayCallback(context.Background(), actor, "callback-trade", "callback-replay-002"); errorCode(err) != "CALLBACK_PENDING" {
		t.Fatalf("callback pending guard error = %v", err)
	}
	var generations int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM callback_events WHERE order_id=$1`, orderID).Scan(&generations); err != nil {
		t.Fatal(err)
	}
	if generations != 2 {
		t.Fatalf("callback generations=%d, want 2", generations)
	}
	t.Run("V34.B.r04/v02", func(t *testing.T) {
		if first["event_id"] == "" || replayed["event_id"] != first["event_id"] || generations != 2 {
			t.Fatalf("pending callback replay was not idempotent: first=%#v replay=%#v generations=%d", first, replayed, generations)
		}
		if _, err := application.Services.Payment.ReplayCallback(t.Context(), actor, "callback-trade", "callback-replay-003"); errorCode(err) != "CALLBACK_PENDING" {
			t.Fatalf("pending generation accepted a second request key: %v", err)
		}
	})
	t.Run("V34.B.r04/v01", func(t *testing.T) {
		orderID := seedPendingExternalOrder(t, db, merchantID, "callback-successful-trade", "callback-successful-out", "https://merchant.example/notify")
		paidTx, err := db.Pool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := application.Services.Payment.MarkPaidTx(t.Context(), paidTx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
			_ = paidTx.Rollback(t.Context())
			t.Fatal(err)
		}
		if err := paidTx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Pool.Exec(t.Context(), `UPDATE callback_events SET state='success' WHERE order_id=$1 AND generation=1`, orderID); err != nil {
			t.Fatal("mark initial callback generation delivered")
		}
		result, err := application.Services.Payment.ReplayCallback(t.Context(), actor, "callback-successful-trade", "callback-successful-replay")
		if err != nil || result["callback_status"] != "pending" {
			t.Fatalf("manual replay after successful callback = %#v err=%v", result, err)
		}
		var generations int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM callback_events WHERE order_id=$1`, orderID).Scan(&generations); err != nil || generations != 2 {
			t.Fatalf("successful callback replay generations=%d err=%v, want 2", generations, err)
		}
	})
	t.Run("V34.B.r04/v03", func(t *testing.T) {
		unpaidID := seedPendingExternalOrder(t, db, merchantID, "callback-unpaid-trade", "callback-unpaid-out", "https://merchant.example/notify")
		if _, err := application.Services.Payment.ReplayCallback(t.Context(), actor, "callback-unpaid-trade", "callback-unpaid-replay"); errorCode(err) != "ORDER_NOT_PAID" {
			t.Fatalf("unpaid callback replay error=%v, want ORDER_NOT_PAID", err)
		}
		var unpaidStatus string
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id=$1`, unpaidID).Scan(&unpaidStatus); err != nil || unpaidStatus != "pending" {
			t.Fatalf("unpaid replay changed order state=%q err=%v", unpaidStatus, err)
		}
		refundedID := seedPendingExternalOrder(t, db, merchantID, "callback-refunded-trade", "callback-refunded-out", "https://merchant.example/notify")
		paidTx, err := db.Pool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := application.Services.Payment.MarkPaidTx(t.Context(), paidTx, refundedID, payment.PaidCommand{Channel: "alipay"}); err != nil {
			_ = paidTx.Rollback(t.Context())
			t.Fatal(err)
		}
		if err := paidTx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte("callback refunded replay request"))
		refund, err := application.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: merchantID, TradeNo: "callback-refunded-trade", OutRefundNo: "callback-refunded-refund", AmountCents: 100, Reason: "customer request", RequestDigest: digest[:], Actor: actor})
		if err != nil {
			t.Fatalf("request full refund: %v", err)
		}
		if err := application.Services.Payment.ResolveRefund(t.Context(), actor, refund.ID, payment.RefundResolution{Action: "mark_refunded", Reason: "settled", Confirm: true}); err != nil {
			t.Fatalf("resolve full refund: %v", err)
		}
		if _, err := application.Services.Payment.ReplayCallback(t.Context(), actor, "callback-refunded-trade", "callback-refunded-replay"); errorCode(err) != "ALREADY_REFUNDED" {
			t.Fatalf("refunded callback replay error=%v, want ALREADY_REFUNDED", err)
		}
	})
}

func TestDB_CallbackRetryKeepsTradeNumberIdempotencyKey(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	planID := db.SeedPlan(t, "callback retry plan", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	fixture.Network.SetDNS("callback.example.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	var mu sync.Mutex
	var received []url.Values
	callbackURL := fixture.Network.ServeTLS(t, "callback.example.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		mu.Lock()
		values := make(url.Values, len(r.URL.Query()))
		for key, entries := range r.URL.Query() {
			values[key] = append([]string(nil), entries...)
		}
		received = append(received, values)
		attempt := len(received)
		mu.Unlock()
		if attempt == 1 {
			http.Error(w, "temporary receiver failure", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(" \tSUCCESS\n"))
	}))
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	order, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
		MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "callback-retry-idempotency", Name: "retryable callback",
		OriginalCents: 100, NotifyURL: callbackURL + "/notify", ReturnURL: "https://merchant.example.test/return", Channel: "alipay",
		ClientIP: "198.51.100.99", SourceIP: "192.0.2.99", Actor: actor,
	})
	if err != nil {
		t.Fatalf("create callback retry order: %v", err)
	}
	orderID, err := fixture.App.Services.Payment.OrderID(t.Context(), order.TradeNo)
	if err != nil {
		t.Fatal("lookup callback retry order")
	}
	paidTx, err := db.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal("begin callback retry paid transaction")
	}
	if err := fixture.App.Services.Payment.MarkPaidTx(t.Context(), paidTx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
		_ = paidTx.Rollback(t.Context())
		t.Fatalf("mark callback retry order paid: %v", err)
	}
	if err := paidTx.Commit(t.Context()); err != nil {
		t.Fatal("commit callback retry paid transition")
	}
	t.Run("V38.B.r04/v01", func(t *testing.T) {
		if err := fixture.App.Services.Payment.ProcessCallback(t.Context(), orderID); err == nil {
			t.Fatal("first callback unexpectedly received strict success")
		}
		if _, err := db.Pool.Exec(t.Context(), `UPDATE callback_events SET next_attempt_at=$1 WHERE order_id=$2 AND generation=1`, db.Clock.Now(), orderID); err != nil {
			t.Fatal("make failed callback due using deterministic fixture clock")
		}
		if err := fixture.App.Services.Payment.ProcessCallback(t.Context(), orderID); err != nil {
			t.Fatalf("retry callback after local receiver recovery: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(received) != 2 || received[0].Get("trade_no") != order.TradeNo || received[1].Get("trade_no") != order.TradeNo {
			t.Fatalf("callback attempt trade_no payloads = %#v, want the identical order trade number", received)
		}
	})
	t.Run("V38.B.r04/v02", func(t *testing.T) {
		mu.Lock()
		defer mu.Unlock()
		if len(received) != 2 || received[0].Get("out_trade_no") != "callback-retry-idempotency" || received[1].Get("out_trade_no") != received[0].Get("out_trade_no") {
			t.Fatalf("callback receiver idempotency identifiers changed across delivery: %#v", received)
		}
		var paidAudits, generations, attempts int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='order.paid' AND target_id=$1`, order.TradeNo).Scan(&paidAudits); err != nil {
			t.Fatal("count single paid business audit")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*),max(attempt_count) FROM callback_events WHERE order_id=$1`, orderID).Scan(&generations, &attempts); err != nil {
			t.Fatal("count callback generations and delivery attempts")
		}
		if paidAudits != 1 || generations != 1 || attempts != 2 {
			t.Fatalf("at-least-once callback changed business state: paid_audits=%d generations=%d attempts=%d", paidAudits, generations, attempts)
		}
	})
}
