package subscriptions

import (
	"context"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_PendingPurchaseReservesLimitAndCancellationReleasesIt(t *testing.T) {
	db := testsupport.NewDatabase(t)
	billingID, billingKey := db.SeedMerchant(t)
	buyerID, _ := db.SeedMerchant(t)
	limit := 1
	planID := db.SeedPlan(t, "limited", 1500, 30, []string{"alipay"}, &limit)
	application := db.NewApplication(t)
	db.PrepareBillingMerchant(t, application, billingID, billingKey, planID)
	actor := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
	request := subscription.PurchaseRequest{PlanID: itoa(planID), PlanRevision: 1, ExpectedSubscriptionRevision: 0}
	first, err := application.Services.Subscription.Purchase(context.Background(), actor, request, "buyer-purchase-001")
	if err != nil || first.State != "pending" || first.TradeNo == nil {
		t.Fatalf("first purchase = %#v, %v", first, err)
	}
	replayed, err := application.Services.Subscription.Purchase(context.Background(), actor, request, "buyer-purchase-001")
	if err != nil || replayed.ID != first.ID || replayed.TradeNo == nil || *replayed.TradeNo != *first.TradeNo {
		t.Fatalf("idempotent purchase replay = %#v, %v", replayed, err)
	}
	if _, err := application.Services.Subscription.Purchase(context.Background(), actor, request, "buyer-purchase-002"); !hasErrorCode(err, "PURCHASE_PENDING") {
		t.Fatalf("second pending purchase error = %v", err)
	}
	if _, err := application.Services.Subscription.CancelPurchase(context.Background(), actor, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := application.Services.Subscription.Purchase(context.Background(), actor, request, "buyer-purchase-003")
	if err != nil || second.State != "pending" {
		t.Fatalf("purchase after cancellation = %#v, %v", second, err)
	}
	var reserved int
	if err := db.Pool.QueryRow(context.Background(), `SELECT reserved_count FROM plan_purchase_counters WHERE merchant_id=$1 AND plan_id=$2`, buyerID, planID).Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if reserved != 1 {
		t.Fatalf("reserved count after new pending purchase = %d", reserved)
	}
}
