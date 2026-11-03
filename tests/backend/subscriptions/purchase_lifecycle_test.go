package subscriptions

import (
	"crypto/sha256"
	"testing"
	"time"

	"vanillapay/internal/app"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_PurchaseCountersFollowPurchaseLifecycle(t *testing.T) {
	t.Run("V25.B.r02/v01", func(t *testing.T) {
		db, app, buyer, planID, _ := newPurchaseCounterFixture(t, 3)
		actor := platform.Actor{Role: "merchant", SubjectID: buyer, MerchantID: buyer}
		purchase, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(planID), PlanRevision: 1}, "counter-cancel-001")
		if err != nil {
			t.Fatal("create pending purchase before cancellation")
		}
		assertPurchaseCounters(t, db, buyer, planID, 0, 1)
		if _, err := app.Services.Subscription.CancelPurchase(t.Context(), actor, purchase.ID); err != nil {
			t.Fatalf("cancel pending purchase: %v", err)
		}
		assertPurchaseCounters(t, db, buyer, planID, 0, 0)
		expiring, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(planID), PlanRevision: 1}, "counter-expire-001")
		if err != nil {
			t.Fatal("create pending purchase before expiration")
		}
		db.Clock.Set(expiring.ExpiresAt.Add(time.Second))
		if _, err := app.Services.Subscription.CloseExpiredPurchases(t.Context(), 100); err != nil {
			t.Fatalf("close expired purchase: %v", err)
		}
		assertPurchaseCounters(t, db, buyer, planID, 0, 0)
		var state string
		if err := db.Pool.QueryRow(t.Context(), `SELECT state FROM purchases WHERE public_id=$1`, expiring.ID).Scan(&state); err != nil || state != "expired" {
			t.Fatalf("expired purchase state=%q err=%v", state, err)
		}
	})

	t.Run("V25.B.r02/v02", func(t *testing.T) {
		db, app, buyer, planID, deviceToken := newPurchaseCounterFixture(t, 3)
		actor := platform.Actor{Role: "merchant", SubjectID: buyer, MerchantID: buyer}
		purchase, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(planID), PlanRevision: 1}, "counter-paid-001")
		if err != nil {
			t.Fatal("create purchase for paid counter")
		}
		completeEntitlementPurchase(t, db, app, deviceToken, purchase, "123e4567-e89b-12d3-a456-426614174521")
		assertPurchaseCounters(t, db, buyer, planID, 1, 0)
	})

	t.Run("V25.B.r02/v03", func(t *testing.T) {
		db, app, buyer, planID, deviceToken := newPurchaseCounterFixture(t, 3)
		actor := platform.Actor{Role: "merchant", SubjectID: buyer, MerchantID: buyer}
		purchase, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(planID), PlanRevision: 1}, "counter-renew-base-001")
		if err != nil {
			t.Fatal("create base purchase before renewal")
		}
		completeEntitlementPurchase(t, db, app, deviceToken, purchase, "123e4567-e89b-12d3-a456-426614174521")
		current, err := app.Services.Subscription.GetCurrent(t.Context(), actor)
		if err != nil {
			t.Fatal("read base purchase entitlement before renewal")
		}
		renewal, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(planID), PlanRevision: 1, ExpectedSubscriptionRevision: current.Revision}, "counter-renew-001")
		if err != nil || renewal.PurchaseKind != "renew" {
			t.Fatalf("renewal purchase=%#v err=%v", renewal, err)
		}
		completeEntitlementPurchase(t, db, app, deviceToken, renewal, "123e4567-e89b-12d3-a456-426614174522")
		assertPurchaseCounters(t, db, buyer, planID, 2, 0)
	})

	t.Run("V25.B.r02/v04", func(t *testing.T) {
		db, app, buyer, planID, _ := newPurchaseCounterFixture(t, 1)
		if _, err := app.Services.Subscription.Grant(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, buyer, subscription.GrantRequest{
			PlanID: itoa(planID), ExpiresAt: db.Clock.Now().Add(30 * 24 * time.Hour), Channels: []string{"alipay"}, ExpectedRevision: 0, Reason: "manual grants do not consume purchase limit",
		}); err != nil {
			t.Fatal("grant manual entitlement")
		}
		assertPurchaseCounters(t, db, buyer, planID, 0, 0)
	})

	t.Run("V25.B.r02/v05", func(t *testing.T) {
		db, app, buyer, planID, deviceToken := newPurchaseCounterFixture(t, 1)
		actor := platform.Actor{Role: "merchant", SubjectID: buyer, MerchantID: buyer}
		purchase, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(planID), PlanRevision: 1}, "counter-refund-001")
		if err != nil {
			t.Fatal("create purchase for refund counter check")
		}
		completeEntitlementPurchase(t, db, app, deviceToken, purchase, "123e4567-e89b-12d3-a456-426614174523")
		var billingID int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT merchant_id FROM orders WHERE trade_no=$1`, *purchase.TradeNo).Scan(&billingID); err != nil {
			t.Fatalf("read subscription billing merchant for refund: %v", err)
		}
		billingActor := platform.Actor{Role: "merchant", SubjectID: billingID, MerchantID: billingID}
		digest := sha256.Sum256([]byte("purchase-counter-refund"))
		request, err := app.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{
			MerchantID: billingID, TradeNo: *purchase.TradeNo, OutRefundNo: "purchase-counter-refund", AmountCents: purchase.PriceCents,
			Reason: "counter lifecycle check", RequestDigest: digest[:], Actor: billingActor,
		})
		if err != nil {
			t.Fatalf("request full paid purchase refund: %v", err)
		}
		if err := app.Services.Payment.ResolveRefund(t.Context(), billingActor, request.ID, payment.RefundResolution{Action: "mark_refunded", Reason: "provider refund confirmed", Confirm: true}); err != nil {
			t.Fatalf("confirm full purchase refund: %v", err)
		}
		assertPurchaseCounters(t, db, buyer, planID, 1, 0)
	})
}

func TestDB_PurchaseIdempotencyConflictsOnDifferentContent(t *testing.T) {
	db := testsupport.NewDatabase(t)
	billingID, billingKey := db.SeedMerchant(t)
	buyer, _ := db.SeedMerchant(t)
	firstPlan := db.SeedPlan(t, "purchase-idempotency-one", 1000, 30, []string{"alipay"}, nil)
	secondPlan := db.SeedPlan(t, "purchase-idempotency-two", 1500, 45, []string{"wxpay"}, nil)
	application := db.NewApplication(t)
	db.PrepareBillingMerchant(t, application, billingID, billingKey, firstPlan)
	actor := platform.Actor{Role: "merchant", SubjectID: buyer, MerchantID: buyer}
	request := subscription.PurchaseRequest{PlanID: itoa(firstPlan), PlanRevision: 1}
	first, err := application.Services.Subscription.Purchase(t.Context(), actor, request, "purchase-idempotency-001")
	if err != nil {
		t.Fatalf("create first idempotent purchase: %v", err)
	}
	t.Run("V25.B.r04/v01", func(t *testing.T) {
		replayed, replayErr := application.Services.Subscription.Purchase(t.Context(), actor, request, "purchase-idempotency-001")
		if replayErr != nil || replayed.ID != first.ID || replayed.TradeNo == nil || first.TradeNo == nil || *replayed.TradeNo != *first.TradeNo {
			t.Fatalf("same idempotency key/content replay=%#v first=%#v err=%v", replayed, first, replayErr)
		}
	})
	t.Run("V25.B.r04/v02", func(t *testing.T) {
		_, conflictErr := application.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(secondPlan), PlanRevision: 1}, "purchase-idempotency-001")
		if !hasErrorCode(conflictErr, "IDEMPOTENCY_CONFLICT") {
			t.Fatalf("different content with same idempotency key error=%v, want IDEMPOTENCY_CONFLICT", conflictErr)
		}
	})
}

func newPurchaseCounterFixture(t *testing.T, limit int) (*testsupport.Database, *app.App, int64, int64, string) {
	t.Helper()
	db := testsupport.NewDatabase(t)
	billingID, billingKey := db.SeedMerchant(t)
	buyer, _ := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "purchase-counter-lifecycle", 1000, 30, []string{"alipay"}, &limit)
	application := db.NewApplication(t)
	deviceToken := db.PrepareBillingMerchant(t, application, billingID, billingKey, planID)
	return db, application, buyer, planID, deviceToken
}

func assertPurchaseCounters(t *testing.T, db *testsupport.Database, buyer, planID int64, wantPaid, wantReserved int) {
	t.Helper()
	var paid, reserved int
	if err := db.Pool.QueryRow(t.Context(), `SELECT COALESCE((SELECT paid_count FROM plan_purchase_counters WHERE merchant_id=$1 AND plan_id=$2),0),COALESCE((SELECT reserved_count FROM plan_purchase_counters WHERE merchant_id=$1 AND plan_id=$2),0)`, buyer, planID).Scan(&paid, &reserved); err != nil {
		t.Fatalf("read plan purchase counters: %v", err)
	}
	if paid != wantPaid || reserved != wantReserved {
		t.Fatalf("purchase counters paid=%d reserved=%d, want %d/%d", paid, reserved, wantPaid, wantReserved)
	}
}
