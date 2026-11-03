package devices

import (
	"context"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_DeviceEligibilityInterleavings(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "ban-receipt-interleaving", 1000, 3650, []string{"alipay"}, nil)
	app := db.NewApplication(t)
	token := db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	order, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "ban-receipt-interleaving", Name: "interleaved", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.43", Actor: actor})
	if err != nil {
		t.Fatalf("create pending order before interleaving: %v", err)
	}
	checkout, err := app.Services.Payment.SelectChannel(t.Context(), order.TradeNo, "alipay")
	if err != nil || checkout.ActualCents == nil {
		t.Fatalf("select channel before interleaving: %#v err=%v", checkout, err)
	}
	identity, err := app.Services.Device.Authenticate(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	barrier := testsupport.NewBarrier(2)
	type outcome struct {
		name string
		err  error
	}
	results := make(chan outcome, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		if err := barrier.Wait(ctx); err != nil {
			results <- outcome{name: "ban", err: err}
			return
		}
		_, err := app.Services.Merchant.Ban(ctx, platform.Actor{Role: "admin", SubjectID: 1}, merchantID, "interleaving acceptance")
		results <- outcome{name: "ban", err: err}
	}()
	go func() {
		defer workers.Done()
		if err := barrier.Wait(ctx); err != nil {
			results <- outcome{name: "receipt", err: err}
			return
		}
		_, err := app.Services.Device.Ingest(ctx, identity, device.ReceiptRequest{
			EventID: "123e4567-e89b-12d3-a456-426614174420", Channel: "alipay", AmountCents: *checkout.ActualCents,
			OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: "ban-interleaving-provider",
		})
		results <- outcome{name: "receipt", err: err}
	}()
	workers.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			t.Fatalf("%s transaction failed or deadlocked: %v", result.name, result.err)
		}
	}
	var bannedAt *time.Time
	if err := db.Pool.QueryRow(t.Context(), `SELECT banned_at FROM merchants WHERE id=$1`, merchantID).Scan(&bannedAt); err != nil || bannedAt == nil {
		t.Fatalf("merchant ban did not commit after interleaving: banned_at=%v err=%v", bannedAt, err)
	}
	var status string
	if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE trade_no=$1`, order.TradeNo).Scan(&status); err != nil {
		t.Fatal("read final order state")
	}
	if status != "paid" && status != "expired" && status != "pending" {
		t.Fatalf("interleaving produced illegal order state %q", status)
	}
	var pending int
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND status='pending'`, merchantID).Scan(&pending); err != nil {
		t.Fatal("read pending orders after ban")
	}
	if pending != 0 {
		t.Fatalf("ban left %d pending order(s), want atomic expiry or a prior paid commit", pending)
	}
	t.Run("V45.B.r01/v01", func(t *testing.T) {
		if status != "paid" && status != "expired" {
			t.Fatalf("ban/receipt lock interleaving left unexpected order state %q", status)
		}
	})
	t.Run("V45.B.r01/v02", func(t *testing.T) {
		if err := ctx.Err(); err != nil {
			t.Fatalf("transaction lock interleaving exceeded bounded completion: %v", err)
		}
		if pending != 0 {
			t.Fatalf("ordered transactions left pending state=%d", pending)
		}
	})
	t.Logf("ban/receipt serialized with final order status=%s", status)
}
