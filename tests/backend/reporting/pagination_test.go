package reporting

import (
	"context"
	"strconv"
	"testing"
	"time"

	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_OrderPaginationHasStableCursorAndEnforcesMerchantOwnership(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	otherID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	created := db.Clock.Now().Truncate(time.Microsecond)
	insertOrder(t, db, merchantID, "page-1", "first", "pending", "", created, nil, nil, 100)
	insertOrder(t, db, merchantID, "page-2", "second", "pending", "", created, nil, nil, 100)
	insertOrder(t, db, merchantID, "page-3", "third", "pending", "", created, nil, nil, 100)
	insertOrder(t, db, otherID, "other-page", "other", "pending", "", created, nil, nil, 999)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	first, err := application.Services.Portal.ListOrders(context.Background(), actor, portal.OrderFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Total != 3 || first.NextCursor == nil {
		t.Fatalf("first page = %#v", first)
	}
	second, err := application.Services.Portal.ListOrders(context.Background(), actor, portal.OrderFilter{Limit: 2, Cursor: *first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Total != 3 || second.NextCursor != nil {
		t.Fatalf("second page = %#v", second)
	}
	seen := map[string]bool{}
	for _, item := range append(first.Items, second.Items...) {
		if seen[item.TradeNo] || item.MerchantID != strconv.FormatInt(merchantID, 10) {
			t.Fatalf("cursor duplicated or leaked an order: %#v", item)
		}
		seen[item.TradeNo] = true
	}
	t.Run("V47.B.r03/v01", func(t *testing.T) {
		for _, item := range append(first.Items, second.Items...) {
			if item.MerchantID != strconv.FormatInt(merchantID, 10) || item.TradeNo == "other-page" || item.OriginalCents != 100 {
				t.Fatalf("merchant order page exposed another merchant's order/amount: %#v", item)
			}
		}
	})
	otherFilter := otherID
	forced, err := application.Services.Portal.ListOrders(context.Background(), actor, portal.OrderFilter{MerchantID: &otherFilter, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if forced.Total != 3 || len(forced.Items) != 3 {
		t.Fatalf("merchant_id filter escaped actor scope: %#v", forced)
	}
	t.Run("V47.B.r03/v02", func(t *testing.T) {
		for _, item := range forced.Items {
			if item.MerchantID != strconv.FormatInt(merchantID, 10) || item.TradeNo == "other-page" {
				t.Fatalf("merchant_id query parameter escaped actor scope: %#v", item)
			}
		}
	})
}
