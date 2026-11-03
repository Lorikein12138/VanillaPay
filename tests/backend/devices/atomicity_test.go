package devices

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_ReceiptCommitAndRecovery(t *testing.T) {
	t.Run("V44.B.r01/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, merchantKey := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "receipt-concurrency", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		token := db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		order, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "receipt-concurrent-order", Name: "concurrent", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.41", Actor: actor})
		if err != nil {
			t.Fatalf("create pending order: %v", err)
		}
		checkout, err := app.Services.Payment.SelectChannel(t.Context(), order.TradeNo, "alipay")
		if err != nil || checkout.ActualCents == nil {
			t.Fatalf("select payment channel: %#v err=%v", checkout, err)
		}
		identity, err := app.Services.Device.Authenticate(t.Context(), token)
		if err != nil {
			t.Fatal(err)
		}
		barrier := testsupport.NewBarrier(2)
		results := make(chan device.ReceiptResult, 2)
		errors := make(chan error, 2)
		var workers sync.WaitGroup
		for i, eventID := range []string{"123e4567-e89b-12d3-a456-426614174401", "123e4567-e89b-12d3-a456-426614174402"} {
			workers.Add(1)
			go func() {
				defer workers.Done()
				if err := barrier.Wait(t.Context()); err != nil {
					errors <- err
					return
				}
				receipt := device.ReceiptRequest{EventID: eventID, Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: "concurrent-provider-" + strconv.Itoa(i)}
				result, err := app.Services.Device.Ingest(t.Context(), identity, receipt)
				if err != nil {
					errors <- err
					return
				}
				results <- result
			}()
		}
		workers.Wait()
		close(errors)
		for err := range errors {
			t.Fatalf("concurrent receipt ingestion: %v", err)
		}
		close(results)
		matched, unmatched := 0, 0
		var unmatchedTrade *string
		for result := range results {
			if result.MatchState == "matched" && result.TradeNo != nil && *result.TradeNo == order.TradeNo {
				matched++
			} else if result.MatchState == "unmatched" {
				unmatched++
				unmatchedTrade = result.TradeNo
			}
		}
		if matched != 1 || unmatched != 1 {
			t.Fatalf("concurrent receipt outcomes matched=%d unmatched=%d, want 1/1", matched, unmatched)
		}
		var matches, paid int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM receipts WHERE order_id=(SELECT id FROM orders WHERE trade_no=$1)`, order.TradeNo).Scan(&matches); err != nil {
			t.Fatal("count receipts assigned to the order")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE trade_no=$1 AND status='paid'`, order.TradeNo).Scan(&paid); err != nil || matches != 1 || paid != 1 {
			t.Fatalf("receipt/order uniqueness matched receipts=%d paid orders=%d err=%v", matches, paid, err)
		}
		t.Run("V33.B.r02/v02", func(t *testing.T) {
			var unassigned int
			if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM receipts WHERE merchant_id=$1 AND match_state='unmatched' AND order_id IS NULL`, merchantID).Scan(&unassigned); err != nil || unassigned != 1 || unmatchedTrade != nil {
				t.Fatalf("losing receipt was reassigned to another order: unassigned=%d trade=%v err=%v", unassigned, unmatchedTrade, err)
			}
		})
		t.Run("V44.B.r01/v02", func(t *testing.T) {
			var assigned int
			if err := db.Pool.QueryRow(t.Context(), `SELECT count(DISTINCT order_id) FROM receipts WHERE merchant_id=$1 AND order_id IS NOT NULL`, merchantID).Scan(&assigned); err != nil || assigned != 1 || matches != 1 || paid != 1 {
				t.Fatalf("receipt.order/order uniqueness assigned=%d receipt_count=%d paid=%d err=%v", assigned, matches, paid, err)
			}
		})
	})

	t.Run("V44.B.r02/v04", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, merchantKey := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "receipt-audit-failure", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		token := db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
		order, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "receipt-audit-rollback", Name: "audit", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.42", Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}})
		if err != nil {
			t.Fatalf("create order for audit rollback: %v", err)
		}
		checkout, err := app.Services.Payment.SelectChannel(t.Context(), order.TradeNo, "alipay")
		if err != nil || checkout.ActualCents == nil {
			t.Fatalf("select payment channel: %#v err=%v", checkout, err)
		}
		identity, err := app.Services.Device.Authenticate(t.Context(), token)
		if err != nil {
			t.Fatal(err)
		}
		receipt := device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174403", Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: "audit-failure-provider"}
		if _, err := db.Pool.Exec(t.Context(), `CREATE FUNCTION reject_device_paid_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='order.paid' THEN RAISE EXCEPTION 'injected device paid audit failure'; END IF; RETURN NEW; END $$`); err != nil {
			t.Fatalf("create local audit failure function: %v", err)
		}
		if _, err := db.Pool.Exec(t.Context(), `CREATE TRIGGER reject_device_paid_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_device_paid_audit()`); err != nil {
			t.Fatalf("create local audit failure trigger: %v", err)
		}
		t.Cleanup(func() {
			_, _ = db.Pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_device_paid_audit ON audit_logs`)
			_, _ = db.Pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS reject_device_paid_audit()`)
		})
		if _, err := app.Services.Device.Ingest(t.Context(), identity, receipt); err == nil {
			t.Fatal("receipt committed despite failed order.paid audit")
		}
		var status string
		var receipts, paidAudits int
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE trade_no=$1`, order.TradeNo).Scan(&status); err != nil {
			t.Fatal("read order after audit failure")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM receipts WHERE event_id=$1`, receipt.EventID).Scan(&receipts); err != nil {
			t.Fatal("count rolled-back receipt")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='order.paid' AND target_id=$1`, order.TradeNo).Scan(&paidAudits); err != nil {
			t.Fatal("count paid audit")
		}
		if status != "pending" || receipts != 0 || paidAudits != 0 {
			t.Fatalf("audit failure left partial receipt settlement status=%s receipts=%d paid audits=%d", status, receipts, paidAudits)
		}
		if _, err := db.Pool.Exec(t.Context(), `DROP TRIGGER reject_device_paid_audit ON audit_logs`); err != nil {
			t.Fatalf("remove local audit trigger before retry: %v", err)
		}
		retried, err := app.Services.Device.Ingest(t.Context(), identity, receipt)
		if err != nil {
			t.Fatalf("retry receipt after audit recovery: %v", err)
		}
		if retried.MatchState != "matched" || retried.Duplicate {
			t.Fatalf("retry receipt after audit recovery = %#v, want a newly matched receipt", retried)
		}
		var committedAudit, committedReceipt, committedPaid int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='order.paid' AND target_id=$1`, order.TradeNo).Scan(&committedAudit); err != nil {
			t.Fatal("read committed paid audit after recovery")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM receipts WHERE event_id=$1`, receipt.EventID).Scan(&committedReceipt); err != nil {
			t.Fatal("read committed receipt after recovery")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE trade_no=$1 AND status='paid'`, order.TradeNo).Scan(&committedPaid); err != nil || committedAudit != 1 || committedReceipt != 1 || committedPaid != 1 {
			t.Fatalf("successful retry did not commit payment, receipt and audit together: paid=%d receipts=%d audits=%d err=%v", committedPaid, committedReceipt, committedAudit, err)
		}
	})
}
