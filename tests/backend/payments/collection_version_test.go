package payments

import (
	"fmt"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_ChannelSelectionSnapshotsOwningMerchantCollectionVersion(t *testing.T) {
	db := testsupport.NewDatabase(t)
	application := db.NewApplication(t)
	planID := db.SeedPlan(t, "collection version", 1000, 3650, []string{"alipay"}, nil)
	merchants := make([]int64, 2)
	for index := range merchants {
		merchantID, key := db.SeedMerchant(t)
		merchants[index] = merchantID
		db.PrepareBillingMerchant(t, application, merchantID, key, planID)
		if _, err := db.Pool.Exec(t.Context(), `UPDATE merchant_collection_settings SET version=$1 WHERE merchant_id=$2`, 17+index*12, merchantID); err != nil {
			t.Fatal(err)
		}
	}
	// Make order IDs differ from both merchant IDs: using the order ID in the
	// collection-settings subquery must never accidentally select the owner.
	seedPendingExternalOrder(t, db, merchants[0], "version-offset-1", "version-offset-1", "https://merchant.example/notify")
	seedPendingExternalOrder(t, db, merchants[0], "version-offset-2", "version-offset-2", "https://merchant.example/notify")
	for _, merchantID := range merchants {
		t.Run(fmt.Sprintf("merchant_%d", merchantID), func(t *testing.T) {
			created, err := application.Services.Payment.Create(t.Context(), payment.CreateCommand{
				MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "collection-version", Name: "collection snapshot", OriginalCents: 250,
				NotifyURL: "https://merchant.example/notify", SourceIP: "192.0.2.33",
				Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := application.Services.Payment.SelectChannel(t.Context(), created.TradeNo, "alipay"); err != nil {
				t.Fatal(err)
			}
			var orderID, ownerID, want int64
			var got *int64
			if err := db.Pool.QueryRow(t.Context(), `SELECT o.id,o.merchant_id,o.collection_version,c.version FROM orders o JOIN merchant_collection_settings c ON c.merchant_id=o.merchant_id WHERE o.trade_no=$1`, created.TradeNo).Scan(&orderID, &ownerID, &got, &want); err != nil {
				t.Fatal(err)
			}
			if orderID == ownerID || ownerID != merchantID || got == nil || *got != want {
				t.Fatalf("order=%d owner=%d merchant=%d collection_version=%v want=%d", orderID, ownerID, merchantID, got, want)
			}
		})
	}
}
