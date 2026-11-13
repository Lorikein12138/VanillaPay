package subscriptions

import (
	"context"
	"sync"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_SubscriptionRemainingAcceptance(t *testing.T) {
	t.Run("V23.B.r01/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		app := db.NewApplication(t)
		admin := platform.Actor{Role: "admin", SubjectID: 1}
		created, err := app.Services.Subscription.SavePlan(t.Context(), admin, subscription.PlanCommand{
			Name: subscription.LocalizedName{ZhCN: "基础", En: "Basic"}, PriceCents: 1200, DurationDays: 30, Channels: []string{"alipay"},
		})
		if err != nil || created.ID == "" || created.Revision != 1 {
			t.Fatalf("create plan = %#v, %v", created, err)
		}
		updated, err := app.Services.Subscription.SavePlan(t.Context(), admin, subscription.PlanCommand{
			ID: created.ID, Revision: created.Revision, Name: subscription.LocalizedName{ZhCN: "升级", En: "Upgrade"},
			PriceCents: 1500, DurationDays: 60, Channels: []string{"alipay", "wxpay"},
		})
		if err != nil || updated.Revision != created.Revision+1 || updated.PriceCents != 1500 || len(updated.Channels) != 2 {
			t.Fatalf("update plan = %#v, %v", updated, err)
		}
		deleted, err := app.Services.Subscription.DeletePlan(t.Context(), admin, updated.ID, updated.Revision)
		if err != nil || !deleted.Deleted {
			t.Fatalf("soft-delete plan = %#v, %v", deleted, err)
		}
		plans, err := app.Services.Subscription.ListPlans(t.Context(), true, admin)
		if err != nil {
			t.Fatal(err)
		}
		foundDeleted := false
		for _, plan := range plans {
			if plan.ID == updated.ID {
				foundDeleted = plan.Deleted
			}
		}
		if !foundDeleted {
			t.Fatal("soft-deleted plan history disappeared from the administrative plan list")
		}
	})

	t.Run("V23.B.r01/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		app := db.NewApplication(t)
		admin := platform.Actor{Role: "admin", SubjectID: 1}
		unlimited, err := app.Services.Subscription.SavePlan(t.Context(), admin, subscription.PlanCommand{
			Name:       subscription.LocalizedName{ZhCN: "双渠道不限购", En: "All channels, unlimited"},
			PriceCents: 1800, DurationDays: 90, Channels: []string{"alipay", "wxpay"}, PurchaseLimit: nil,
		})
		if err != nil || len(unlimited.Channels) != 2 || unlimited.PurchaseLimit != nil {
			t.Fatalf("valid unlimited two-channel plan = %#v, %v", unlimited, err)
		}
		limit := 2
		limited, err := app.Services.Subscription.SavePlan(t.Context(), admin, subscription.PlanCommand{
			Name: subscription.LocalizedName{ZhCN: "正限购", En: "Positive limit"}, PriceCents: 2000,
			DurationDays: 30, Channels: []string{"alipay", "wxpay"}, PurchaseLimit: &limit,
		})
		if err != nil || len(limited.Channels) != 2 || limited.PurchaseLimit == nil || *limited.PurchaseLimit != limit {
			t.Fatalf("valid positive limited plan = %#v, %v", limited, err)
		}
	})

	t.Run("V23.B.r01/v03", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		app := db.NewApplication(t)
		admin := platform.Actor{Role: "admin", SubjectID: 1}
		for _, invalid := range []struct {
			name string
			cmd  subscription.PlanCommand
		}{
			{name: "empty_channels", cmd: subscription.PlanCommand{Name: subscription.LocalizedName{ZhCN: "无渠道", En: "No channels"}, PriceCents: 100, DurationDays: 1, Channels: []string{}}},
			{name: "zero_price", cmd: subscription.PlanCommand{Name: subscription.LocalizedName{ZhCN: "零价", En: "Zero price"}, PriceCents: 0, DurationDays: 1, Channels: []string{"alipay"}}},
			{name: "zero_duration", cmd: subscription.PlanCommand{Name: subscription.LocalizedName{ZhCN: "零时长", En: "Zero duration"}, PriceCents: 100, DurationDays: 0, Channels: []string{"alipay"}}},
			{name: "unknown_channel", cmd: subscription.PlanCommand{Name: subscription.LocalizedName{ZhCN: "未知渠道", En: "Unknown channel"}, PriceCents: 100, DurationDays: 1, Channels: []string{"bank-transfer"}}},
		} {
			if _, err := app.Services.Subscription.SavePlan(t.Context(), admin, invalid.cmd); err == nil {
				t.Errorf("invalid plan %s was accepted", invalid.name)
			}
		}
	})

	t.Run("V23.B.r02/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, _ := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "snapshot-before-edit", 1000, 30, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		admin := platform.Actor{Role: "admin", SubjectID: 1}
		before := db.Clock.Now().AddDate(0, 0, 30)
		if _, err := app.Services.Subscription.Grant(t.Context(), admin, merchantID, subscription.GrantRequest{
			PlanID: itoa(planID), ExpiresAt: before, Channels: []string{"alipay"}, ExpectedRevision: 0, Reason: "snapshot acceptance",
		}); err != nil {
			t.Fatal(err)
		}
		var revision int64
		var nameZH, nameEN string
		var price int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT revision,name_zh,name_en,price_cents FROM plans WHERE id=$1`, planID).Scan(&revision, &nameZH, &nameEN, &price); err != nil {
			t.Fatal("read plan before update")
		}
		if _, err := app.Services.Subscription.SavePlan(t.Context(), admin, subscription.PlanCommand{
			ID: itoa(planID), Revision: revision, Name: subscription.LocalizedName{ZhCN: "changed", En: "Changed"}, PriceCents: 2500, DurationDays: 90, Channels: []string{"wxpay"},
		}); err != nil {
			t.Fatal("edit plan after entitlement grant")
		}
		current, err := app.Services.Subscription.GetCurrent(t.Context(), platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID})
		if err != nil || current.Plan == nil || current.Plan.Name.ZhCN != nameZH || current.Plan.Name.En != nameEN || current.Plan.PriceCents != price || len(current.Channels) != 1 || current.Channels[0] != "alipay" {
			t.Fatalf("existing entitlement changed with the source plan: current=%#v err=%v", current, err)
		}
	})

	t.Run("V23.B.r02/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		billingID, billingKey := db.SeedMerchant(t)
		newBuyerID, _ := db.SeedMerchant(t)
		renewBuyerID, _ := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "deleted-purchase-plan", 1000, 30, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
		admin := platform.Actor{Role: "admin", SubjectID: 1}
		if _, err := app.Services.Subscription.Grant(t.Context(), admin, renewBuyerID, subscription.GrantRequest{
			PlanID: itoa(planID), ExpiresAt: db.Clock.Now().AddDate(0, 0, 30), Channels: []string{"alipay"}, ExpectedRevision: 0, Reason: "prepare renewal buyer",
		}); err != nil {
			t.Fatalf("prepare existing subscription before plan deletion: %v", err)
		}
		deleted, err := app.Services.Subscription.DeletePlan(t.Context(), admin, itoa(planID), 1)
		if err != nil || !deleted.Deleted {
			t.Fatalf("delete plan before purchase: result=%#v err=%v", deleted, err)
		}
		for _, buyer := range []struct {
			id  int64
			key string
		}{{newBuyerID, "deleted-plan-new-purchase"}, {renewBuyerID, "deleted-plan-renewal"}} {
			_, err = app.Services.Subscription.Purchase(t.Context(), platform.Actor{Role: "merchant", SubjectID: buyer.id, MerchantID: buyer.id}, subscription.PurchaseRequest{
				PlanID: itoa(planID), PlanRevision: 1, ExpectedSubscriptionRevision: 0,
			}, buyer.key)
			if !hasErrorCode(err, "PLAN_UNAVAILABLE") {
				t.Fatalf("purchase for buyer %d of soft-deleted plan error=%v, want PLAN_UNAVAILABLE", buyer.id, err)
			}
		}
	})

	t.Run("V23.B.r02/v03", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, _ := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "historical-plan", 1000, 30, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		admin := platform.Actor{Role: "admin", SubjectID: 1}
		if _, err := app.Services.Subscription.Grant(t.Context(), admin, merchantID, subscription.GrantRequest{
			PlanID: itoa(planID), ExpiresAt: db.Clock.Now().AddDate(0, 0, 30), Channels: []string{"alipay"}, ExpectedRevision: 0, Reason: "history remains available",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := app.Services.Subscription.DeletePlan(t.Context(), admin, itoa(planID), 1); err != nil {
			t.Fatalf("soft-delete source plan: %v", err)
		}
		var grants, deletedPlans int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM subscription_grants WHERE merchant_id=$1`, merchantID).Scan(&grants); err != nil {
			t.Fatal("read historical entitlement grants")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM plans WHERE id=$1 AND deleted_at IS NOT NULL`, planID).Scan(&deletedPlans); err != nil {
			t.Fatal("read soft-deleted plan history")
		}
		if grants != 1 || deletedPlans != 1 {
			t.Fatalf("historical records grants=%d deleted_plans=%d, want 1/1", grants, deletedPlans)
		}
		current, err := app.Services.Subscription.GetCurrent(t.Context(), platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID})
		if err != nil || current.Plan == nil || current.Plan.ID != itoa(planID) {
			t.Fatalf("historical current subscription after plan deletion = %#v, err=%v", current, err)
		}
		plans, err := app.Services.Subscription.ListPlans(t.Context(), true, admin)
		if err != nil {
			t.Fatal("read administrative plan history")
		}
		found := false
		for _, plan := range plans {
			if plan.ID == itoa(planID) && plan.Deleted {
				found = true
			}
		}
		if !found {
			t.Fatal("administrative plan history omitted the soft-deleted plan")
		}
	})

	t.Run("V25.B.r01/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		billingID, billingKey := db.SeedMerchant(t)
		buyerID, _ := db.SeedMerchant(t)
		limit := 2
		planID := db.SeedPlan(t, "concurrent-limit", 1500, 30, []string{"alipay"}, &limit)
		app := db.NewApplication(t)
		deviceToken := db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
		buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
		initial, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{PlanID: itoa(planID), PlanRevision: 1, ExpectedSubscriptionRevision: 0}, "acceptance-limit-paid")
		if err != nil {
			t.Fatalf("create first paid purchase for lifecycle limit: %v", err)
		}
		completeEntitlementPurchase(t, db, app, deviceToken, initial, "123e4567-e89b-12d3-a456-426614174330")
		current, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if err != nil || !current.Active {
			t.Fatalf("read paid purchase entitlement before concurrent reservation: %#v err=%v", current, err)
		}
		barrier := testsupport.NewBarrier(3)
		type outcome struct {
			buyerID int64
			result  subscription.PurchaseDTO
			err     error
		}
		outcomes := make(chan outcome, 3)
		var workers sync.WaitGroup
		for i := 0; i < 3; i++ {

			workers.Add(1)
			go func() {
				defer workers.Done()
				if err := barrier.Wait(t.Context()); err != nil {
					outcomes <- outcome{buyerID: buyerID, err: err}
					return
				}
				purchase, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{
					PlanID: itoa(planID), PlanRevision: 1, ExpectedSubscriptionRevision: current.Revision,
				}, "acceptance-limit-"+itoa(int64(i)))
				outcomes <- outcome{buyerID: buyerID, result: purchase, err: err}
			}()
		}
		workers.Wait()
		close(outcomes)
		var accepted, pending int
		for result := range outcomes {
			switch {
			case result.err == nil:
				accepted++
			case hasErrorCode(result.err, "PURCHASE_PENDING"):
				pending++
			default:
				t.Fatalf("concurrent limited purchase returned unexpected error for buyer %d: %v", result.buyerID, result.err)
			}
		}
		if accepted != 1 || pending != 2 {
			t.Fatalf("concurrent purchase outcomes accepted=%d pending=%d, want 1/2 for the same buyer", accepted, pending)
		}
		var paid, reserved, pendingRows int
		if err := db.Pool.QueryRow(context.Background(), `SELECT paid_count,reserved_count FROM plan_purchase_counters WHERE plan_id=$1 AND merchant_id=$2`, planID, buyerID).Scan(&paid, &reserved); err != nil || paid != 1 || reserved != 1 || paid+reserved > limit {
			t.Fatalf("concurrent purchase counter paid=%d reserved=%d, want 1/1 with total <= %d; err=%v", paid, reserved, limit, err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM purchases WHERE buyer_id=$1 AND plan_id=$2 AND state='pending'`, buyerID, planID).Scan(&pendingRows); err != nil || pendingRows != 1 {
			t.Fatalf("concurrent pending purchases for buyer=%d, want exactly one; err=%v", pendingRows, err)
		}
	})

	t.Run("V25.B.r01/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		billingID, billingKey := db.SeedMerchant(t)
		buyerID, _ := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "one-pending-per-buyer", 1000, 30, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
		actor := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
		request := subscription.PurchaseRequest{PlanID: itoa(planID), PlanRevision: 1, ExpectedSubscriptionRevision: 0}
		if _, err := app.Services.Subscription.Purchase(t.Context(), actor, request, "single-pending-01"); err != nil {
			t.Fatalf("first pending purchase failed: %v", err)
		}
		if _, err := app.Services.Subscription.Purchase(t.Context(), actor, request, "single-pending-02"); !hasErrorCode(err, "PURCHASE_PENDING") {
			t.Fatalf("second pending purchase error=%v, want PURCHASE_PENDING", err)
		}
	})
}
