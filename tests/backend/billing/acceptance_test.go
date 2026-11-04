package billing

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_BillingRemainingAcceptance(t *testing.T) {
	t.Run("V26.B.r04/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		billingID, billingKey := db.SeedMerchant(t)
		buyerID, _ := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "billing-disabled-seller", 1200, 30, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
		if _, err := db.Pool.Exec(t.Context(), `UPDATE merchants SET banned_at=$1,ban_reason='fixture' WHERE id=$2`, db.Clock.Now(), billingID); err != nil {
			t.Fatalf("disable billing merchant for readiness case: %v", err)
		}
		_, err := app.Services.Subscription.Purchase(t.Context(), platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}, subscription.PurchaseRequest{PlanID: decimal(planID), PlanRevision: 1}, "billing-seller-disabled")
		if !hasBillingCode(err, "BILLING_NOT_READY") {
			t.Fatalf("purchase with banned billing seller error=%v, want BILLING_NOT_READY", err)
		}
		assertNoPartialBillingPurchase(t, db, buyerID, planID)
	})

	t.Run("V26.B.r05/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		billingID, billingKey := db.SeedMerchant(t)
		buyerID, _ := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "billing-stale-seller-key", 1200, 30, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
		cipher, err := platform.Encrypt(db.Key[:], []byte(strings.Repeat("b", 64)), rand.Reader)
		if err != nil {
			t.Fatalf("encrypt mismatched billing merchant key fixture: %v", err)
		}
		if _, err := db.Pool.Exec(t.Context(), `UPDATE merchants SET key_cipher=$1 WHERE id=$2`, cipher, billingID); err != nil {
			t.Fatalf("rotate billing merchant key after configuration: %v", err)
		}
		_, err = app.Services.Subscription.Purchase(t.Context(), platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}, subscription.PurchaseRequest{PlanID: decimal(planID), PlanRevision: 1}, "billing-stale-seller-key")
		if !hasBillingCode(err, "BILLING_CREDENTIAL_STALE") {
			t.Fatalf("purchase with stale billing key error=%v, want BILLING_CREDENTIAL_STALE", err)
		}
		assertNoPartialBillingPurchase(t, db, buyerID, planID)
	})
}

func assertNoPartialBillingPurchase(t *testing.T, db *testsupport.Database, buyerID, planID int64) {
	t.Helper()
	var purchases, orders int
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM purchases WHERE buyer_id=$1 AND plan_id=$2`, buyerID, planID).Scan(&purchases); err != nil {
		t.Fatal("count purchases after rejected billing transaction")
	}
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE buyer_id=$1 AND purchase_id IS NOT NULL`, buyerID).Scan(&orders); err != nil {
		t.Fatal("count billing orders after rejected billing transaction")
	}
	if purchases != 0 || orders != 0 {
		t.Fatalf("rejected billing transaction left purchases=%d orders=%d", purchases, orders)
	}
}

func hasBillingCode(err error, code string) bool {
	apiErr := &platform.Error{}
	ok := errors.As(err, &apiErr)
	return ok && apiErr.Code == code
}
