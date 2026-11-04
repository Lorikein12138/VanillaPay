package payments

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_PaymentAtomicityAndRecovery(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	orderID := seedPendingExternalOrder(t, db, merchantID, "audit-rollback-trade", "audit-rollback-out", "https://merchant.example/notify")
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	request := payment.SupplementRequest{Reason: "verified offline payment", Channel: "alipay"}
	if _, err := db.Pool.Exec(t.Context(), `CREATE FUNCTION reject_payment_paid_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='order.paid' AND NEW.target_id='audit-rollback-trade' THEN RAISE EXCEPTION 'injected paid audit failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatalf("create local audit failure function: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `CREATE TRIGGER reject_payment_paid_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_payment_paid_audit()`); err != nil {
		t.Fatalf("create local paid audit failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_payment_paid_audit ON audit_logs`)
		_, _ = db.Pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS reject_payment_paid_audit()`)
	})
	t.Run("V38.B.r02/v01", func(t *testing.T) {
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "audit-rollback-trade", request, "audit-rollback-key-01"); err == nil {
			t.Fatal("supplement committed although order.paid audit insert failed")
		}
		var status string
		var events int
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&status); err != nil {
			t.Fatal("read order after audit failure")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM callback_events WHERE order_id=$1`, orderID).Scan(&events); err != nil {
			t.Fatal("count callback work after audit failure")
		}
		if status != "pending" || events != 0 {
			t.Fatalf("audit failure left partial payment: order=%s callback_events=%d", status, events)
		}
	})
	if _, err := db.Pool.Exec(t.Context(), `DROP TRIGGER reject_payment_paid_audit ON audit_logs`); err != nil {
		t.Fatalf("remove local audit failure trigger: %v", err)
	}
	t.Run("V38.B.r02/retry_commits_one_paid_action", func(t *testing.T) {
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "audit-rollback-trade", request, "audit-rollback-key-01"); err != nil {
			t.Fatalf("retry after removing audit failure: %v", err)
		}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "audit-rollback-trade", request, "audit-rollback-key-01"); err != nil {
			t.Fatalf("same idempotent paid request: %v", err)
		}
		var status string
		var events int
		var role string
		var actorID string
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&status); err != nil {
			t.Fatal("read recovered order")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*),min(actor_role),min(actor_id::text) FROM audit_logs WHERE action='order.paid' AND target_id='audit-rollback-trade'`).Scan(&events, &role, &actorID); err != nil {
			t.Fatal("read recovered paid audit")
		}
		if status != "paid" || events != 1 || role != "merchant" || actorID != strconv.FormatInt(merchantID, 10) {
			t.Fatalf("recovered payment status=%s paid_audits=%d role=%s actor=%s", status, events, role, actorID)
		}
	})
}

func TestDB_PaymentReceiptSupplementRefundAndExpiryInterleaving(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	planID := db.SeedPlan(t, "interleaving payment plan", 1000, 3650, []string{"alipay"}, nil)
	deviceToken := db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	order, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
		MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "v38-interleaved-out", Name: "interleaved order",
		OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", ReturnURL: "https://merchant.example.test/return",
		Channel: "alipay", ClientIP: "198.51.100.103", SourceIP: "192.0.2.103", Actor: actor,
	})
	if err != nil {
		t.Fatalf("create interleaving order: %v", err)
	}
	orderID, err := fixture.App.Services.Payment.OrderID(t.Context(), order.TradeNo)
	if err != nil {
		t.Fatal("lookup interleaving order")
	}
	if _, err := db.Pool.Exec(t.Context(), `UPDATE orders SET expires_at=$1 WHERE id=$2`, db.Clock.Now(), orderID); err != nil {
		t.Fatal("make order due for the concurrent expiry transition")
	}
	barrier := testsupport.NewBarrier(4)
	type outcome struct {
		name   string
		status int
		err    error
	}
	results := make(chan outcome, 4)
	var workers sync.WaitGroup
	start := func(name string, run func() outcome) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := barrier.Wait(t.Context()); err != nil {
				results <- outcome{name: name, err: err}
				return
			}
			results <- run()
		}()
	}
	start("receipt-match", func() outcome {
		response := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", map[string]any{
			"event_id": "e0000000-0000-4000-8000-000000000381", "channel": "alipay", "amount_cents": 100,
			"occurred_at": db.Clock.Now().Format(time.RFC3339Nano), "raw_text": "V38 interleaving receipt",
		}, "device:"+deviceToken)
		return outcome{name: "receipt-match", status: response.Code}
	})
	start("merchant-supplement", func() outcome {
		err := fixture.App.Services.Payment.SupplementIdempotent(t.Context(), actor, order.TradeNo, payment.SupplementRequest{Reason: "confirmed offline payment", Channel: "alipay"}, "v38-interleave-supplement")
		return outcome{name: "merchant-supplement", err: err}
	})
	start("refund-request", func() outcome {
		digest := sha256.Sum256([]byte("v38 concurrent refund"))
		_, err := fixture.App.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{
			MerchantID: merchantID, TradeNo: order.TradeNo, OutRefundNo: "v38-interleave-refund", AmountCents: 100,
			Reason: "customer requested refund", RequestDigest: digest[:], Actor: actor,
		})
		return outcome{name: "refund-request", err: err}
	})
	start("checkout-expiry", func() outcome {
		response := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+order.TradeNo, nil, "guest")
		return outcome{name: "checkout-expiry", status: response.Code}
	})
	workers.Wait()
	close(results)
	observed := make(map[string]outcome, 4)
	for result := range results {
		if result.err != nil && (result.name != "merchant-supplement" && result.name != "refund-request") {
			t.Fatalf("interleaving participant %s failed before business outcome: %v", result.name, result.err)
		}
		observed[result.name] = result
	}
	if len(observed) != 4 || observed["receipt-match"].status != http.StatusCreated && observed["receipt-match"].status != http.StatusOK || observed["checkout-expiry"].status != http.StatusOK {
		t.Fatalf("interleaving HTTP outcomes = %#v", observed)
	}
	var state string
	var orders, paidAudits, callbackEvents, linkedReceipts int
	if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&state); err != nil {
		t.Fatal("read final interleaved order state")
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no='v38-interleaved-out'`, merchantID).Scan(&orders); err != nil {
		t.Fatal("count interleaved order identities")
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='order.paid' AND target_id=$1`, order.TradeNo).Scan(&paidAudits); err != nil {
		t.Fatal("count interleaved paid audits")
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM callback_events WHERE order_id=$1`, orderID).Scan(&callbackEvents); err != nil {
		t.Fatal("count interleaved callback generations")
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM receipts WHERE order_id=$1`, orderID).Scan(&linkedReceipts); err != nil {
		t.Fatal("count receipts linked to the interleaved order")
	}
	if state != "paid" && state != "expired" || orders != 1 || paidAudits > 1 || callbackEvents > 1 || linkedReceipts > 1 || (state == "paid" && paidAudits != 1) || (state == "expired" && paidAudits != 0) {
		t.Fatalf("illegal interleaved transition state=%s orders=%d paid_audits=%d callbacks=%d linked_receipts=%d", state, orders, paidAudits, callbackEvents, linkedReceipts)
	}
	if result := observed["refund-request"]; result.err == nil {
		var pending int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM refunds r JOIN orders o ON o.id=r.order_id WHERE o.id=$1 AND r.status='pending'`, orderID).Scan(&pending); err != nil || pending != 1 || state != "paid" {
			t.Fatalf("successful concurrent refund request lacks paid order/pending refund: state=%s pending=%d err=%v", state, pending, err)
		}
	}
	t.Run("V38.B.r01/v01", func(t *testing.T) {
		if orders != 1 || linkedReceipts > 1 || (state == "paid" && callbackEvents != 1) {
			t.Fatalf("receipt/order uniqueness or terminal transition invariant failed: orders=%d receipts=%d callbacks=%d state=%s", orders, linkedReceipts, callbackEvents, state)
		}
	})
}
