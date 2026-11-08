package subscriptions

import (
	"context"
	"errors"
	"math"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_PlanRevisionConflictAndSoftDeletePreserveHistory(t *testing.T) {
	db := testsupport.NewDatabase(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	plan, err := application.Services.Subscription.SavePlan(ctx, admin, subscription.PlanCommand{Name: subscription.LocalizedName{ZhCN: "基础", En: "Basic"}, PriceCents: 1000, DurationDays: 30, Channels: []string{"alipay"}})
	if err != nil {
		t.Fatal(err)
	}
	update := subscription.PlanCommand{ID: plan.ID, Revision: plan.Revision, Name: subscription.LocalizedName{ZhCN: "基础更新", En: "Basic updated"}, PriceCents: 1200, DurationDays: 30, Channels: []string{"alipay"}}
	updated, err := application.Services.Subscription.SavePlan(ctx, admin, update)
	if err != nil || updated.Revision != plan.Revision+1 {
		t.Fatalf("plan update = %#v, %v", updated, err)
	}
	if _, err := application.Services.Subscription.SavePlan(ctx, admin, update); !hasErrorCode(err, "VERSION_CONFLICT") {
		t.Fatalf("stale plan update error = %v", err)
	}
	deleted, err := application.Services.Subscription.DeletePlan(ctx, admin, plan.ID, updated.Revision)
	if err != nil || !deleted.Deleted {
		t.Fatalf("plan delete = %#v, %v", deleted, err)
	}
	if _, err := application.Services.Subscription.SavePlan(ctx, admin, update); err == nil {
		t.Fatal("soft-deleted plan was mutable")
	}
}

func TestDB_PlanPurchaseLimitFitsPostgresInteger(t *testing.T) {
	service := testsupport.NewDatabase(t).NewApplication(t).Services.Subscription
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	command := subscription.PlanCommand{Name: subscription.LocalizedName{ZhCN: "限购"}, PriceCents: 100, DurationDays: 30,
		Channels: []string{"alipay"}, PurchaseLimit: new(math.MaxInt32)}
	plan, err := service.SavePlan(t.Context(), admin, command)
	if err != nil || plan.PurchaseLimit == nil || *plan.PurchaseLimit != math.MaxInt32 {
		t.Fatalf("max purchase limit: %#v, %v", plan.PurchaseLimit, err)
	}
	command.PurchaseLimit = new(math.MaxInt32 + 1)
	for _, id := range []string{"", plan.ID} {
		command.ID, command.Revision = id, plan.Revision
		_, err := service.SavePlan(t.Context(), admin, command)
		var problem *platform.Error
		if !errors.As(err, &problem) || problem.Code != "VALIDATION_ERROR" || problem.HTTPStatus != 400 || problem.Fields["purchase_limit"] == "" {
			t.Fatalf("overflow plan %q: %#v", id, err)
		}
	}
}

func hasErrorCode(err error, code string) bool {
	var apiErr *platform.Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}
