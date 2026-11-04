package billing

import (
	"context"
	"strconv"
	"testing"
	"time"

	"vanillapay/internal/device"
	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_DevicePaymentFulfillsSubscriptionExactlyOnce(t *testing.T) {
	db := testsupport.NewDatabase(t)
	billingID, billingKey := db.SeedMerchant(t)
	buyerID, _ := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "paid plan", 1800, 30, []string{"alipay"}, nil)
	application := db.NewApplication(t)
	deviceToken := db.PrepareBillingMerchant(t, application, billingID, billingKey, planID)
	buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
	purchase, err := application.Services.Subscription.Purchase(context.Background(), buyer, subscription.PurchaseRequest{PlanID: decimal(planID), PlanRevision: 1, ExpectedSubscriptionRevision: 0}, "billing-purchase-001")
	if err != nil || purchase.TradeNo == nil || purchase.State != "pending" {
		t.Fatalf("subscription purchase = %#v, %v", purchase, err)
	}
	checkout, err := application.Services.Payment.SelectChannel(context.Background(), *purchase.TradeNo, "alipay")
	if err != nil || checkout.ActualCents == nil {
		t.Fatalf("select subscription payment channel = %#v, %v", checkout, err)
	}
	identity, err := application.Services.Device.Authenticate(context.Background(), deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	receiptCtx := platform.WithRequestMetadata(context.Background(), platform.RequestMetadata{IP: "::ffff:192.0.2.61", RequestID: "subscription-device-paid-audit"})
	receipt := device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174001", Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: "billing-provider-1"}
	matched, err := application.Services.Device.Ingest(receiptCtx, identity, receipt)
	if err != nil || matched.MatchState != "matched" {
		t.Fatalf("subscription receipt = %#v, %v", matched, err)
	}
	duplicate, err := application.Services.Device.Ingest(receiptCtx, identity, receipt)
	if err != nil || !duplicate.Duplicate || duplicate.ReceiptID != matched.ReceiptID {
		t.Fatalf("receipt replay = %#v, %v", duplicate, err)
	}
	var state string
	var fulfillmentAt *time.Time
	if err := db.Pool.QueryRow(context.Background(), `SELECT state,fulfillment_at FROM purchases WHERE public_id=$1`, purchase.ID).Scan(&state, &fulfillmentAt); err != nil {
		t.Fatal(err)
	}
	if state != "paid" || fulfillmentAt == nil {
		t.Fatalf("purchase fulfillment state=%s at=%v", state, fulfillmentAt)
	}
	current, err := application.Services.Subscription.GetCurrent(context.Background(), buyer)
	if err != nil || !current.Active || current.Source != "purchase" || current.ExpiresAt == nil {
		t.Fatalf("buyer subscription after receipt = %#v, %v", current, err)
	}
	var grants, paidOrders int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM subscription_grants WHERE purchase_id=(SELECT id FROM purchases WHERE public_id=$1)`, purchase.ID).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM orders WHERE trade_no=$1 AND status='paid'`, *purchase.TradeNo).Scan(&paidOrders); err != nil {
		t.Fatal(err)
	}
	if grants != 1 || paidOrders != 1 {
		t.Fatalf("atomic fulfillment grants=%d paid_orders=%d", grants, paidOrders)
	}
	for _, event := range []struct {
		action   string
		target   string
		merchant int64
	}{{"order.paid", *purchase.TradeNo, buyerID}, {"subscription.fulfilled", purchase.ID, buyerID}} {
		var count int
		var role, auditMerchant, ip, requestID string
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*),min(actor_role),min(merchant_id::text),min(ip::text),min(request_id) FROM audit_logs WHERE action=$1 AND target_id=$2`, event.action, event.target).Scan(&count, &role, &auditMerchant, &ip, &requestID); err != nil {
			t.Fatalf("read %s audit after idempotent receipt replay: %v", event.action, err)
		}
		if count != 1 || role != "device" || auditMerchant != strconv.FormatInt(event.merchant, 10) || ip != "192.0.2.61/32" || requestID != "subscription-device-paid-audit" {
			t.Fatalf("%s audit count=%d role=%q merchant=%q want_merchant=%d ip=%q request_id=%q", event.action, count, role, auditMerchant, event.merchant, ip, requestID)
		}
	}
	t.Run("V16.B.r01/v10", func(t *testing.T) {
		var orderPaid, fulfilled int
		if err := db.Pool.QueryRow(receiptCtx, `SELECT count(*) FROM audit_logs WHERE request_id=$1 AND actor_role='device' AND merchant_id=$2 AND ip='192.0.2.61' AND action='order.paid' AND target_id=$3`, "subscription-device-paid-audit", buyerID, *purchase.TradeNo).Scan(&orderPaid); err != nil {
			t.Fatalf("read order.paid audit: %v", err)
		}
		if err := db.Pool.QueryRow(receiptCtx, `SELECT count(*) FROM audit_logs WHERE request_id=$1 AND actor_role='device' AND merchant_id=$2 AND ip='192.0.2.61' AND action='subscription.fulfilled' AND target_id=$3`, "subscription-device-paid-audit", buyerID, purchase.ID).Scan(&fulfilled); err != nil {
			t.Fatalf("read subscription.fulfilled audit: %v", err)
		}
		if orderPaid != 1 || fulfilled != 1 {
			t.Fatalf("device-paid subscription audit events order.paid=%d subscription.fulfilled=%d, want 1/1", orderPaid, fulfilled)
		}
	})
	t.Run("V26.B.r01/v01", func(t *testing.T) {
		billing := platform.Actor{Role: "merchant", SubjectID: billingID, MerchantID: billingID}
		entitlement, err := application.Services.Subscription.GetCurrent(t.Context(), billing)
		if err != nil || !entitlement.Active || entitlement.ExpiresAt == nil {
			t.Fatalf("billing merchant subscription prerequisite = %#v err=%v", entitlement, err)
		}
		if _, err := application.Services.Device.Authenticate(t.Context(), deviceToken); err != nil {
			t.Fatalf("online device with the prepared billing credential did not authenticate: %v", err)
		}
	})
	t.Run("V26.B.r01/v02", func(t *testing.T) {
		var orderMerchant, purchaseBuyer int64
		var purpose string
		if err := db.Pool.QueryRow(t.Context(), `SELECT merchant_id,purpose FROM orders WHERE trade_no=$1`, *purchase.TradeNo).Scan(&orderMerchant, &purpose); err != nil {
			t.Fatalf("read generated billing order: %v", err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT buyer_id FROM purchases WHERE public_id=$1`, purchase.ID).Scan(&purchaseBuyer); err != nil {
			t.Fatalf("read generated subscription purchase: %v", err)
		}
		if orderMerchant != billingID || purchaseBuyer != buyerID || purpose != "subscription" {
			t.Fatalf("buyer purchase billing ownership order_merchant=%d purchase_buyer=%d purpose=%q", orderMerchant, purchaseBuyer, purpose)
		}
	})
	t.Run("V26.B.r02/v01", func(t *testing.T) {
		var orderPaid, purchasePaid, grantCount int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE trade_no=$1 AND status='paid'`, *purchase.TradeNo).Scan(&orderPaid); err != nil {
			t.Fatal("read settled order state")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM purchases WHERE public_id=$1 AND state='paid' AND fulfillment_at IS NOT NULL`, purchase.ID).Scan(&purchasePaid); err != nil {
			t.Fatal("read fulfilled purchase state")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM subscription_grants WHERE purchase_id=(SELECT id FROM purchases WHERE public_id=$1) AND refunded_at IS NULL AND voided_at IS NULL`, purchase.ID).Scan(&grantCount); err != nil {
			t.Fatal("read active purchase grant")
		}
		if orderPaid != 1 || purchasePaid != 1 || grantCount != 1 {
			t.Fatalf("receipt transaction committed order=%d purchase=%d active_grants=%d; want 1/1/1", orderPaid, purchasePaid, grantCount)
		}
	})
	t.Run("V44.B.r03/v01", func(t *testing.T) {
		if !duplicate.Duplicate || duplicate.ReceiptID != matched.ReceiptID || grants != 1 {
			t.Fatalf("receipt replay duplicated fulfillment: duplicate=%t receipt_ids=%q/%q grants=%d", duplicate.Duplicate, duplicate.ReceiptID, matched.ReceiptID, grants)
		}
	})
	t.Run("V26.B.r03/v02", func(t *testing.T) {
		replay, err := application.Services.Device.Ingest(t.Context(), identity, receipt)
		if err != nil || !replay.Duplicate || replay.ReceiptID != matched.ReceiptID {
			t.Fatalf("same subscription receipt replay = %#v err=%v", replay, err)
		}
		var paidCount int
		var totalCents int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*),COALESCE(sum(actual_cents),0) FROM orders WHERE trade_no=$1 AND status='paid'`, *purchase.TradeNo).Scan(&paidCount, &totalCents); err != nil {
			t.Fatal("read paid subscription revenue row")
		}
		if paidCount != 1 || totalCents != *checkout.ActualCents {
			t.Fatalf("receipt replay counted revenue paid_orders=%d total_cents=%d, want 1/%d", paidCount, totalCents, *checkout.ActualCents)
		}
	})
}

func TestDB_SubscriptionPaidAuditFailureRollsBackReceiptAndFulfillment(t *testing.T) {
	db := testsupport.NewDatabase(t)
	billingID, billingKey := db.SeedMerchant(t)
	buyerID, _ := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "audit rollback plan", 1900, 30, []string{"alipay"}, nil)
	application := db.NewApplication(t)
	deviceToken := db.PrepareBillingMerchant(t, application, billingID, billingKey, planID)
	buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
	purchase, err := application.Services.Subscription.Purchase(context.Background(), buyer, subscription.PurchaseRequest{PlanID: decimal(planID), PlanRevision: 1, ExpectedSubscriptionRevision: 0}, "audit-rollback-purchase")
	if err != nil || purchase.TradeNo == nil {
		t.Fatalf("create subscription purchase = %#v, %v", purchase, err)
	}
	checkout, err := application.Services.Payment.SelectChannel(context.Background(), *purchase.TradeNo, "alipay")
	if err != nil || checkout.ActualCents == nil {
		t.Fatalf("select subscription payment channel = %#v, %v", checkout, err)
	}
	identity, err := application.Services.Device.Authenticate(context.Background(), deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	receipt := device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174002", Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: "audit-rollback-provider"}
	if _, err := db.Pool.Exec(context.Background(), `CREATE FUNCTION reject_paid_audit_for_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='order.paid' THEN RAISE EXCEPTION 'injected audit write failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatalf("create audit fault trigger function: %v", err)
	}
	if _, err := db.Pool.Exec(context.Background(), `CREATE TRIGGER reject_paid_audit_for_test BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_paid_audit_for_test()`); err != nil {
		t.Fatalf("create audit fault trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_paid_audit_for_test ON audit_logs`)
		_, _ = db.Pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS reject_paid_audit_for_test()`)
	})
	if _, err := application.Services.Device.Ingest(context.Background(), identity, receipt); err == nil {
		t.Fatal("receipt ingestion succeeded although order.paid audit was rejected")
	}
	var orderStatus, purchaseState string
	var receipts, grants, paidEvents int
	if err := db.Pool.QueryRow(context.Background(), `SELECT status FROM orders WHERE trade_no=$1`, *purchase.TradeNo).Scan(&orderStatus); err != nil {
		t.Fatal("read order after audit rollback")
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT state FROM purchases WHERE public_id=$1`, purchase.ID).Scan(&purchaseState); err != nil {
		t.Fatal("read purchase after audit rollback")
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM receipts WHERE event_id=$1`, receipt.EventID).Scan(&receipts); err != nil {
		t.Fatal("count receipt after audit rollback")
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM subscription_grants WHERE purchase_id=(SELECT id FROM purchases WHERE public_id=$1)`, purchase.ID).Scan(&grants); err != nil {
		t.Fatal("count grants after audit rollback")
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM callback_events WHERE order_id=(SELECT id FROM orders WHERE trade_no=$1)`, *purchase.TradeNo).Scan(&paidEvents); err != nil {
		t.Fatal("count callback events after audit rollback")
	}
	if orderStatus != "pending" || purchaseState != "pending" || receipts != 0 || grants != 0 || paidEvents != 0 {
		t.Fatalf("audit failure left partial fulfillment: order=%s purchase=%s receipts=%d grants=%d callback_events=%d", orderStatus, purchaseState, receipts, grants, paidEvents)
	}
	if _, err := db.Pool.Exec(context.Background(), `DROP TRIGGER reject_paid_audit_for_test ON audit_logs`); err != nil {
		t.Fatalf("remove audit fault trigger: %v", err)
	}
	if _, err := application.Services.Device.Ingest(context.Background(), identity, receipt); err != nil {
		t.Fatalf("retry receipt after removing audit fault: %v", err)
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT status FROM orders WHERE trade_no=$1`, *purchase.TradeNo).Scan(&orderStatus); err != nil || orderStatus != "paid" {
		t.Fatalf("successful retry did not commit payment: order=%s err=%v", orderStatus, err)
	}
}

func decimal(value int64) string { return strconv.FormatInt(value, 10) }
