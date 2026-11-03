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

func TestDB_PaidReceiptAndFulfillmentSurviveServiceReconstruction(t *testing.T) {
	db := testsupport.NewDatabase(t)
	billingID, billingKey := db.SeedMerchant(t)
	buyerID, _ := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "recovery plan", 2200, 45, []string{"alipay"}, nil)
	firstApp := db.NewApplication(t)
	deviceToken := db.PrepareBillingMerchant(t, firstApp, billingID, billingKey, planID)
	buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
	purchase, err := firstApp.Services.Subscription.Purchase(context.Background(), buyer, subscription.PurchaseRequest{PlanID: strconv.FormatInt(planID, 10), PlanRevision: 1, ExpectedSubscriptionRevision: 0}, "recovery-purchase-001")
	if err != nil || purchase.TradeNo == nil {
		t.Fatalf("purchase = %#v, %v", purchase, err)
	}
	checkout, err := firstApp.Services.Payment.SelectChannel(context.Background(), *purchase.TradeNo, "alipay")
	if err != nil || checkout.ActualCents == nil {
		t.Fatalf("checkout = %#v, %v", checkout, err)
	}
	identity, err := firstApp.Services.Device.Authenticate(context.Background(), deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	receipt := device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174002", Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: db.Clock.Now().Add(time.Second)}
	if result, err := firstApp.Services.Device.Ingest(context.Background(), identity, receipt); err != nil || result.MatchState != "matched" {
		t.Fatalf("first service receipt = %#v, %v", result, err)
	}
	if err := firstApp.Close(); err != nil {
		t.Fatal(err)
	}
	secondApp := db.NewApplication(t)
	identity, err = secondApp.Services.Device.Authenticate(context.Background(), deviceToken)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("V26.B.r03/v01", func(t *testing.T) {
		replayed, err := secondApp.Services.Device.Ingest(t.Context(), identity, receipt)
		if err != nil || !replayed.Duplicate {
			t.Fatalf("receipt replay after reconstruction = %#v, %v", replayed, err)
		}
		var orderID int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT id FROM orders WHERE trade_no=$1 AND status='paid'`, *purchase.TradeNo).Scan(&orderID); err != nil {
			t.Fatalf("read paid subscription order ID: %v", err)
		}
		var riverJobs, paid, grants, fulfillmentRecords int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE args->>'order_id'=$1`, strconv.FormatInt(orderID, 10)).Scan(&riverJobs); err != nil {
			t.Fatalf("read River jobs enqueued for subscription payment: %v", err)
		}
		if riverJobs != 0 {
			t.Fatalf("synchronous subscription fulfillment unexpectedly enqueued %d order jobs", riverJobs)
		}
		initialRiverJobs := riverJobs
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE trade_no=$1 AND status='paid'`, *purchase.TradeNo).Scan(&paid); err != nil {
			t.Fatal(err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM subscription_grants WHERE purchase_id=(SELECT id FROM purchases WHERE public_id=$1)`, purchase.ID).Scan(&grants); err != nil {
			t.Fatal(err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM callback_events WHERE order_id=$1 AND state='internal_fulfillment'`, orderID).Scan(&fulfillmentRecords); err != nil {
			t.Fatalf("read synchronous fulfillment record: %v", err)
		}
		var expiresAt *time.Time
		var revision int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT expires_at,revision FROM subscriptions WHERE merchant_id=$1`, buyerID).Scan(&expiresAt, &revision); err != nil {
			t.Fatalf("read active subscription snapshot: %v", err)
		}
		var revenue int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT COALESCE(sum(actual_cents),0) FROM orders WHERE merchant_id=$1 AND status='paid'`, billingID).Scan(&revenue); err != nil {
			t.Fatalf("read billing merchant revenue: %v", err)
		}
		if paid != 1 || grants != 1 || fulfillmentRecords != 1 || expiresAt == nil || revision < 1 || revenue != *checkout.ActualCents {
			t.Fatalf("restart recovery baseline paid=%d grants=%d fulfillment=%d expires_at=%v revision=%d revenue=%d", paid, grants, fulfillmentRecords, expiresAt, revision, revenue)
		}

		// Subscription fulfillment is synchronous and this transaction enqueued
		// no order-specific River job. Start the production registry and wait for
		// its periodic expiry/offline workers to each complete two scans.
		workerCtx, stopWorkers := context.WithCancel(context.Background())
		if err := secondApp.Services.Jobs.Start(workerCtx); err != nil {
			stopWorkers()
			t.Fatalf("start reconstructed River workers: %v", err)
		}
		workersRunning := true
		defer func() {
			if workersRunning {
				stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := secondApp.Services.Jobs.Stop(stopCtx); err != nil {
					t.Errorf("stop reconstructed River workers: %v", err)
				}
				stopWorkers()
			}
		}()
		deadline := time.Now().Add(40 * time.Second)
		var expiryRuns, offlineRuns int
		for {
			if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE kind='vanillapay_expiry_scan' AND state='completed'),count(*) FILTER (WHERE kind='vanillapay_offline_scan' AND state='completed') FROM river_job`).Scan(&expiryRuns, &offlineRuns); err != nil {
				t.Fatalf("count completed expiry/offline River scans: %v", err)
			}
			if expiryRuns >= 2 && offlineRuns >= 2 {
				stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := secondApp.Services.Jobs.Stop(stopCtx); err != nil {
					cancel()
					t.Fatalf("stop workers after two recovery scans: %v", err)
				}
				cancel()
				stopWorkers()
				workersRunning = false
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("River scan workers did not each complete two periodic scans: expiry=%d offline=%d", expiryRuns, offlineRuns)
			}
			select {
			case <-t.Context().Done():
				t.Fatal("context canceled while waiting for repeated River scan workers")
			case <-time.After(100 * time.Millisecond):
			}
		}
		var afterPaid, afterGrants, afterFulfillment int
		var afterExpiresAt *time.Time
		var afterRevision int64
		var afterRevenue int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE trade_no=$1 AND status='paid'`, *purchase.TradeNo).Scan(&afterPaid); err != nil {
			t.Fatal(err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM subscription_grants WHERE purchase_id=(SELECT id FROM purchases WHERE public_id=$1)`, purchase.ID).Scan(&afterGrants); err != nil {
			t.Fatal(err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM callback_events WHERE order_id=$1 AND state='internal_fulfillment'`, orderID).Scan(&afterFulfillment); err != nil {
			t.Fatal(err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT expires_at,revision FROM subscriptions WHERE merchant_id=$1`, buyerID).Scan(&afterExpiresAt, &afterRevision); err != nil {
			t.Fatal(err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT COALESCE(sum(actual_cents),0) FROM orders WHERE merchant_id=$1 AND status='paid'`, billingID).Scan(&afterRevenue); err != nil {
			t.Fatal(err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE args->>'order_id'=$1`, strconv.FormatInt(orderID, 10)).Scan(&riverJobs); err != nil {
			t.Fatalf("read River jobs after repeated scans: %v", err)
		}
		if expiryRuns != 2 || offlineRuns != 2 || afterPaid != 1 || afterGrants != 1 || afterFulfillment != 1 || afterExpiresAt == nil || !afterExpiresAt.Equal(*expiresAt) || afterRevision != revision || afterRevenue != revenue || riverJobs != initialRiverJobs {
			t.Fatalf("repeat scans changed fulfillment: paid=%d grants=%d fulfillment=%d expires_at=%v revision=%d revenue=%d", afterPaid, afterGrants, afterFulfillment, afterExpiresAt, afterRevision, afterRevenue)
		}
	})
}
