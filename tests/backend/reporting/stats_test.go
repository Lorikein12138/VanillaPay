package reporting

import (
	"context"
	"crypto/sha256"
	"strconv"
	"testing"
	"time"

	"vanillapay/internal/app"
	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_StatsSeparatesHalfOpenOrderCountsFromPaidAmounts(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	otherID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	start := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	insertOrder(t, db, merchantID, "inside-paid", "paid", "paid", "alipay", start, datePtr(start.Add(time.Hour)), nil, 100)
	insertOrder(t, db, merchantID, "inside-pending", "pending", "pending", "", start.Add(time.Hour), nil, nil, 200)
	insertOrder(t, db, merchantID, "at-end", "excluded", "expired", "wxpay", end, nil, nil, 300)
	insertOrder(t, db, merchantID, "refunded", "excluded refund", "refunded", "qqpay", start.Add(2*time.Hour), datePtr(start.Add(2*time.Hour)), datePtr(start.Add(3*time.Hour)), 400)
	insertOrder(t, db, merchantID, "created-before-paid-inside", "paid amount", "paid", "wxpay", start.Add(-time.Hour), datePtr(start.Add(4*time.Hour)), nil, 500)
	insertOrder(t, db, merchantID, "created-inside-paid-at-end", "created count", "paid", "qqpay", start.Add(5*time.Hour), datePtr(end), nil, 600)
	insertOrder(t, db, otherID, "other-merchant", "other", "paid", "alipay", start, datePtr(start.Add(time.Hour)), nil, 700)

	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	stats, err := application.Services.Portal.Stats(context.Background(), actor, portal.StatsQuery{Start: start, End: end, Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalOrderCount != 3 || stats.StatusCounts["paid"] != 2 || stats.StatusCounts["pending"] != 1 || stats.StatusCounts["expired"] != 0 {
		t.Fatalf("created-at counts = total %d, status %#v", stats.TotalOrderCount, stats.StatusCounts)
	}
	if stats.TotalAmountCents != 600 || stats.Channels["alipay"].AmountCents != 100 || stats.Channels["wxpay"].AmountCents != 500 || stats.Channels["qqpay"].AmountCents != 0 {
		t.Fatalf("paid-at amounts = total %d channels %#v", stats.TotalAmountCents, stats.Channels)
	}
	if stats.UnassignedCount != 1 || len(stats.Trend) != 1 || stats.Trend[0].Date != "2026-10-30" {
		t.Fatalf("unassigned/trend = %d/%#v", stats.UnassignedCount, stats.Trend)
	}
	if stats.AmountBasis != "paid_at" || stats.OrderBasis != "created_at" {
		t.Fatalf("statistics basis = %s/%s", stats.AmountBasis, stats.OrderBasis)
	}
	t.Run("V46.B.r01/v01", func(t *testing.T) {
		if stats.TotalOrderCount != 3 || stats.StatusCounts["paid"] != 2 || stats.StatusCounts["pending"] != 1 {
			t.Fatalf("fixed-range status totals = total %d statuses %#v", stats.TotalOrderCount, stats.StatusCounts)
		}
	})
	t.Run("V46.B.r01/v02", func(t *testing.T) {
		if stats.Channels["alipay"].AmountCents != 100 || stats.Channels["wxpay"].AmountCents != 500 || stats.Channels["qqpay"].AmountCents != 0 || stats.UnassignedCount != 1 {
			t.Fatalf("channel and unassigned totals = %#v unassigned=%d", stats.Channels, stats.UnassignedCount)
		}
	})
	t.Run("V46.B.r01/v03", func(t *testing.T) {
		if stats.TotalAmountCents != 600 || stats.Channels["wxpay"].AmountCents != 500 {
			t.Fatalf("paid-at totals = %d channels=%#v", stats.TotalAmountCents, stats.Channels)
		}
	})
	t.Run("V46.B.r01/v04", func(t *testing.T) {
		if stats.TotalOrderCount != 3 || stats.StatusCounts["paid"] != 2 {
			t.Fatalf("created-at order counts = %d statuses=%#v", stats.TotalOrderCount, stats.StatusCounts)
		}
	})
	t.Run("V46.B.r01/v05", func(t *testing.T) {
		if stats.TotalAmountCents != 600 || stats.TotalOrderCount != 3 {
			t.Fatalf("refunded order leaked into statistics: amount=%d count=%d", stats.TotalAmountCents, stats.TotalOrderCount)
		}
	})
}

func TestDB_AdminStatsShowsPlatformCountsAndExcludesRefundedSubscriptionRevenue(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	start := db.Clock.Now().Add(-time.Hour)
	end := db.Clock.Now().Add(time.Hour)
	insertOrder(t, db, merchantID, "admin-count", "paid", "paid", "alipay", db.Clock.Now(), datePtr(db.Clock.Now()), nil, 321)
	stats, err := application.Services.Portal.Stats(context.Background(), platform.Actor{Role: "admin", SubjectID: 1}, portal.StatsQuery{Start: start, End: end, Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if stats.MerchantCount == nil || *stats.MerchantCount != 1 || stats.TotalAmountCents != 321 || stats.SubscriptionRevenueCents == nil || *stats.SubscriptionRevenueCents != 0 {
		t.Fatalf("admin statistics = %#v", stats)
	}
}

func TestDB_AdminStatsCountsCurrentSubscriptions(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "admin stats active subscription", 1250, 30, []string{"alipay"}, nil)
	app := db.NewApplication(t)
	if _, err := app.Services.Subscription.Grant(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, merchantID, subscription.GrantRequest{
		PlanID: strconv.FormatInt(planID, 10), ExpiresAt: db.Clock.Now().Add(30 * 24 * time.Hour), Channels: []string{"alipay"}, ExpectedRevision: 0, Reason: "statistics fixture",
	}); err != nil {
		t.Fatalf("grant an active merchant subscription: %v", err)
	}
	start, end := db.Clock.Now().Add(-time.Hour), db.Clock.Now().Add(time.Hour)
	stats, err := app.Services.Portal.Stats(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, portal.StatsQuery{Start: start, End: end, Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("V47.B.r01/v01", func(t *testing.T) {
		if stats.MerchantCount == nil || *stats.MerchantCount != 1 {
			t.Fatalf("admin merchant count=%v, want one seeded merchant", stats.MerchantCount)
		}
	})
	t.Run("V47.B.r01/v02", func(t *testing.T) {
		if stats.ActiveSubscriptionCount == nil || *stats.ActiveSubscriptionCount != 1 {
			t.Fatalf("current active subscription count=%v, want 1", stats.ActiveSubscriptionCount)
		}
	})
}

func TestDB_ManualPaidOrderAppearsInMerchantStats(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "manual stats billing", 1000, 3650, []string{"alipay"}, nil)
	app := db.NewApplication(t)
	db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
	created := db.Clock.Now().UTC().Truncate(time.Microsecond)
	order, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{
		MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "manual-stats-order", Name: "manual payment",
		OriginalCents: 125, NotifyURL: "https://merchant.example.test/notify", Channel: "alipay", SourceIP: "192.0.2.80",
		Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID},
	})
	if err != nil {
		t.Fatalf("create real manual supplement order: %v", err)
	}
	if err := app.Services.Payment.SupplementIdempotent(t.Context(), platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}, order.TradeNo, payment.SupplementRequest{Reason: "offline verified", Channel: "alipay"}, "manual-stats-supplement"); err != nil {
		t.Fatalf("settle the order through the manual supplement service: %v", err)
	}
	stats, err := app.Services.Portal.Stats(t.Context(), platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}, portal.StatsQuery{Start: created.Add(-time.Minute), End: created.Add(time.Minute), Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("V46.B.r01/v06", func(t *testing.T) {
		if stats.TotalAmountCents != 125 || stats.StatusCounts["paid"] != 1 || stats.Channels["alipay"].PaidCount != 1 {
			t.Fatalf("manual paid order statistics = amount %d status %#v channel %#v", stats.TotalAmountCents, stats.StatusCounts, stats.Channels["alipay"])
		}
	})
}

func TestDB_AdminSubscriptionRevenueAndPlatformPaymentsAreSeparate(t *testing.T) {
	db := testsupport.NewDatabase(t)
	billingID, billingKey := db.SeedMerchant(t)
	firstBuyer, _ := db.SeedMerchant(t)
	refundedBuyer, _ := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "admin revenue contract", 1000, 30, []string{"alipay"}, nil)
	app := db.NewApplication(t)
	billingDeviceToken := db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
	firstPurchase := completeReportingPurchase(t, db, app, billingDeviceToken, firstBuyer, planID, "admin-revenue-paid")
	refundedPurchase := completeReportingPurchase(t, db, app, billingDeviceToken, refundedBuyer, planID, "admin-revenue-refunded")
	refundActor := platform.Actor{Role: "merchant", SubjectID: billingID, MerchantID: billingID}
	digest := sha256.Sum256([]byte("admin revenue refund"))
	refund, err := app.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{
		MerchantID: billingID, TradeNo: *refundedPurchase.TradeNo, OutRefundNo: "admin-revenue-refund",
		AmountCents: refundedPurchase.PriceCents, Reason: "confirmed refund", RequestDigest: digest[:], Actor: refundActor,
	})
	if err != nil {
		t.Fatalf("request subscription purchase refund: %v", err)
	}
	if err := app.Services.Payment.ResolveRefund(t.Context(), refundActor, refund.ID, payment.RefundResolution{Action: "mark_refunded", Reason: "provider confirms refund", Confirm: true}); err != nil {
		t.Fatalf("mark subscription purchase refunded: %v", err)
	}
	refundedCurrent, err := app.Services.Subscription.GetCurrent(t.Context(), platform.Actor{Role: "merchant", SubjectID: refundedBuyer, MerchantID: refundedBuyer})
	if err != nil {
		t.Fatalf("read subscription revision after refund: %v", err)
	}
	if _, err := app.Services.Subscription.Grant(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, refundedBuyer, subscription.GrantRequest{
		PlanID: strconv.FormatInt(planID, 10), ExpiresAt: db.Clock.Now().Add(30 * 24 * time.Hour), Channels: []string{"alipay"}, ExpectedRevision: refundedCurrent.Revision, Reason: "manual grant is not revenue",
	}); err != nil {
		t.Fatalf("add manual entitlement: %v", err)
	}
	ordinary, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{
		MerchantID: billingID, Protocol: "v1", Purpose: "external", OutTradeNo: "admin-revenue-ordinary", Name: "ordinary MB payment",
		OriginalCents: 333, NotifyURL: "https://merchant.example.test/notify", Channel: "alipay", SourceIP: "192.0.2.81",
		Actor: platform.Actor{Role: "merchant", SubjectID: billingID, MerchantID: billingID},
	})
	if err != nil {
		t.Fatalf("create ordinary MB payment: %v", err)
	}
	if err := app.Services.Payment.SupplementIdempotent(t.Context(), platform.Actor{Role: "merchant", SubjectID: billingID, MerchantID: billingID}, ordinary.TradeNo, payment.SupplementRequest{Reason: "verified ordinary receipt", Channel: "alipay"}, "admin-revenue-ordinary-key"); err != nil {
		t.Fatalf("settle ordinary MB payment: %v", err)
	}
	start, end := db.Clock.Now().Add(-time.Hour), db.Clock.Now().Add(time.Hour)
	stats, err := app.Services.Portal.Stats(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, portal.StatsQuery{Start: start, End: end, Timezone: "UTC"})
	if err != nil {
		t.Fatalf("read platform statistics: %v", err)
	}
	t.Run("V47.B.r02/v01", func(t *testing.T) {
		if stats.SubscriptionRevenueCents == nil || *stats.SubscriptionRevenueCents != 1000 {
			t.Fatalf("successful purchase subscription revenue=%v, want 1000 cents", stats.SubscriptionRevenueCents)
		}
	})
	t.Run("V47.B.r02/v02", func(t *testing.T) {
		if stats.SubscriptionRevenueCents == nil || *stats.SubscriptionRevenueCents != firstPurchase.PriceCents {
			t.Fatalf("refunded purchase/manual grant affected subscription revenue=%v, expected only paid purchase %d", stats.SubscriptionRevenueCents, firstPurchase.PriceCents)
		}
	})
	t.Run("V47.B.r02/v03", func(t *testing.T) {
		if stats.TotalAmountCents != 1000+333 {
			t.Fatalf("platform paid order total=%d, want subscription order once plus ordinary MB order once", stats.TotalAmountCents)
		}
	})
	t.Run("V47.B.r02/v04", func(t *testing.T) {
		if stats.TotalAmountCents != 1333 || stats.SubscriptionRevenueCents == nil || *stats.SubscriptionRevenueCents != 1000 {
			t.Fatalf("subscription income was added twice: paid orders=%d subscription revenue=%v", stats.TotalAmountCents, stats.SubscriptionRevenueCents)
		}
	})
}

func completeReportingPurchase(t *testing.T, db *testsupport.Database, app *app.App, deviceToken string, buyerID, planID int64, outTradePrefix string) subscription.PurchaseDTO {
	t.Helper()
	actor := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
	purchase, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: strconv.FormatInt(planID, 10), PlanRevision: 1}, outTradePrefix)
	if err != nil || purchase.TradeNo == nil {
		t.Fatalf("create subscription purchase: %#v err=%v", purchase, err)
	}
	checkout, err := app.Services.Payment.SelectChannel(t.Context(), *purchase.TradeNo, "alipay")
	if err != nil || checkout.ActualCents == nil {
		t.Fatalf("select subscription purchase payment: %#v err=%v", checkout, err)
	}
	identity, err := app.Services.Device.Authenticate(t.Context(), deviceToken)
	if err != nil {
		t.Fatalf("authenticate billing device: %v", err)
	}
	result, err := app.Services.Device.Ingest(t.Context(), identity, device.ReceiptRequest{
		EventID: "123e4567-e89b-12d3-a456-426614174" + map[string]string{"admin-revenue-paid": "501", "admin-revenue-refunded": "502"}[outTradePrefix],
		Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: outTradePrefix + "-provider",
	})
	if err != nil || result.MatchState != "matched" {
		t.Fatalf("settle subscription purchase through billing device: %#v err=%v", result, err)
	}
	return purchase
}
