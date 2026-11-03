package subscriptions

import (
	"context"
	"errors"
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

func hasErrorCode(err error, code string) bool {
	var apiErr *platform.Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}
