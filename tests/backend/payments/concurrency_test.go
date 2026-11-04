package payments

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/app"
	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_ConcurrentAmountReservationsAreUniqueAndBounded(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "reservation merchant", 1000, 3650, []string{"alipay"}, nil)
	application := db.NewApplication(t)
	db.PrepareBillingMerchant(t, application, merchantID, merchantKey, planID)
	const workers = 20
	var group sync.WaitGroup
	group.Add(workers)
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer group.Done()
			_, err := application.Services.Payment.Create(context.Background(), payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: fmt.Sprintf("parallel-%02d", i), Name: "parallel order", OriginalCents: 100, NotifyURL: "https://merchant.example/notify", ClientIP: "198.51.100.31", SourceIP: "192.0.2.31", Channel: "alipay", Actor: platform.Actor{Role: "merchant", MerchantID: merchantID}})
			results <- err
		}(i)
	}
	group.Wait()
	close(results)
	var successes, exhausted int
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if errorCode(err) == "AMOUNT_CAPACITY_EXHAUSTED" {
			exhausted++
			continue
		}
		t.Errorf("unexpected concurrent create error: %v", err)
	}
	if successes != 11 || exhausted != workers-successes {
		t.Fatalf("concurrent allocation successes=%d exhausted=%d", successes, exhausted)
	}
	var distinct int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(DISTINCT actual_cents) FROM orders WHERE merchant_id=$1 AND out_trade_no LIKE 'parallel-%' AND status='pending'`, merchantID).Scan(&distinct); err != nil {
		t.Fatal(err)
	}
	if distinct != successes {
		t.Fatalf("distinct actual amounts=%d, successful reservations=%d", distinct, successes)
	}
	t.Run("V31.B.r01/v01", func(t *testing.T) {
		if workers != 20 {
			t.Fatalf("same merchant/channel input orders=%d, want 20", workers)
		}
	})
	t.Run("V31.B.r01/v02", func(t *testing.T) {
		if successes != 11 || exhausted != 9 {
			t.Fatalf("limit-ten allocation successes=%d capacity errors=%d, want 11/9", successes, exhausted)
		}
	})
	t.Run("V31.B.r01/v03", func(t *testing.T) {
		var minAmount, maxAmount int64
		if err := db.Pool.QueryRow(context.Background(), `SELECT min(actual_cents),max(actual_cents) FROM orders WHERE merchant_id=$1 AND out_trade_no LIKE 'parallel-%'`, merchantID).Scan(&minAmount, &maxAmount); err != nil || minAmount != 90 || maxAmount != 100 || distinct != 11 {
			t.Fatalf("allocated range min=%d max=%d distinct=%d err=%v, want 90..100/11", minAmount, maxAmount, distinct, err)
		}
	})
	t.Run("V31.B.r01/v04", func(t *testing.T) {
		if successes+exhausted != workers || exhausted != 9 {
			t.Fatalf("capacity result partition success=%d error=%d input=%d", successes, exhausted, workers)
		}
	})
}

func TestDB_AmountReservationsRespectOriginalsAndLifecycle(t *testing.T) {
	t.Run("V31.B.r02/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, key := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "reservation original amount", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, merchantID, key, planID)
		for i := 0; i < 10; i++ {
			createReservationOrder(t, db, app, merchantID, fmt.Sprintf("base-%02d", i), 100, "alipay", nil)
		}
		order := createReservationOrder(t, db, app, merchantID, "different-original-overlap", 95, "alipay", nil)
		if order.ActualCents == nil || *order.ActualCents != 90 {
			t.Fatalf("different original collided with an existing actual amount: %#v", order)
		}
		var duplicates int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*)-count(DISTINCT actual_cents) FROM orders WHERE merchant_id=$1 AND (out_trade_no LIKE 'base-%' OR out_trade_no='different-original-overlap')`, merchantID).Scan(&duplicates); err != nil || duplicates != 0 {
			t.Fatalf("cross-original reservations duplicate count=%d err=%v", duplicates, err)
		}
	})
	t.Run("V31.B.r03/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, key := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "reservation upward amount", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, merchantID, key, planID)
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		limit, direction := 3, "up"
		if _, err := app.Services.Settings.SaveCollection(t.Context(), actor, settings.SaveCollectionRequest{FloatLimitCents: &limit, FloatDirection: &direction}); err != nil {
			t.Fatalf("configure upward reservation bounds: %v", err)
		}
		first := createReservationOrder(t, db, app, merchantID, "up-first", 100, "alipay", nil)
		second := createReservationOrder(t, db, app, merchantID, "up-second", 100, "alipay", nil)
		third := createReservationOrder(t, db, app, merchantID, "up-third", 100, "alipay", nil)
		if first.ActualCents == nil || second.ActualCents == nil || third.ActualCents == nil || *first.ActualCents != 100 || *second.ActualCents != 101 || *third.ActualCents != 102 {
			t.Fatalf("upward allocations=%v/%v/%v", first.ActualCents, second.ActualCents, third.ActualCents)
		}
	})
	t.Run("V31.B.r03/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, key := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "reservation nonzero amount", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, merchantID, key, planID)
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		limit, direction := 10, "down"
		if _, err := app.Services.Settings.SaveCollection(t.Context(), actor, settings.SaveCollectionRequest{FloatLimitCents: &limit, FloatDirection: &direction}); err != nil {
			t.Fatalf("configure downward reservation bounds: %v", err)
		}
		order := createReservationOrder(t, db, app, merchantID, "one-cent-original", 1, "alipay", nil)
		if order.ActualCents == nil || *order.ActualCents != 1 {
			t.Fatalf("one-cent order was allocated as %#v", order.ActualCents)
		}
	})
	t.Run("V31.B.r03/v03", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, key := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "reservation global bounds", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, merchantID, key, planID)
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		limit, direction := 10, "up"
		if _, err := app.Services.Settings.SaveCollection(t.Context(), actor, settings.SaveCollectionRequest{FloatLimitCents: &limit, FloatDirection: &direction}); err != nil {
			t.Fatalf("configure upper-bound reservation: %v", err)
		}
		order := createReservationOrder(t, db, app, merchantID, "global-upper-bound", 9999999, "alipay", nil)
		if order.ActualCents == nil || *order.ActualCents > payment.MaxAmountCents || *order.ActualCents > 10000000 {
			t.Fatalf("allocation crossed global amount bound: %#v", order.ActualCents)
		}
	})
	t.Run("V31.B.r04/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		firstID, firstKey := db.SeedMerchant(t)
		secondID, secondKey := db.SeedMerchant(t)
		firstPlan := db.SeedPlan(t, "first reservation merchant", 1000, 3650, []string{"alipay", "wxpay"}, nil)
		secondPlan := db.SeedPlan(t, "second reservation merchant", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, firstID, firstKey, firstPlan)
		db.PrepareBillingMerchant(t, app, secondID, secondKey, secondPlan)
		first := createReservationOrder(t, db, app, firstID, "merchant-one-same-amount", 100, "alipay", nil)
		second := createReservationOrder(t, db, app, secondID, "merchant-two-same-amount", 100, "alipay", nil)
		if first.ActualCents == nil || second.ActualCents == nil || *first.ActualCents != 100 || *second.ActualCents != 100 {
			t.Fatalf("separate merchant reservation amounts=%v/%v", first.ActualCents, second.ActualCents)
		}
		otherChannel := createReservationOrder(t, db, app, firstID, "same-merchant-other-channel", 100, "wxpay", nil)
		if otherChannel.ActualCents == nil || *otherChannel.ActualCents != 100 {
			t.Fatalf("reservation amount was shared across channels: %#v", otherChannel.ActualCents)
		}
	})
	t.Run("V31.B.r05/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, key := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "paid reservation release", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, merchantID, key, planID)
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		first := createReservationOrder(t, db, app, merchantID, "paid-reservation", 100, "alipay", nil)
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, first.TradeNo, payment.SupplementRequest{Reason: "verified", Channel: "alipay"}, "paid-reservation-key"); err != nil {
			t.Fatalf("mark reservation order paid: %v", err)
		}
		second := createReservationOrder(t, db, app, merchantID, "after-paid-release", 100, "alipay", nil)
		if second.ActualCents == nil || *second.ActualCents != 100 {
			t.Fatalf("paid amount slot was not released: %#v", second.ActualCents)
		}
	})
	t.Run("V31.B.r05/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, key := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "expired reservation isolation", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		deviceToken := db.PrepareBillingMerchant(t, app, merchantID, key, planID)
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		limit, direction := 0, "down"
		if _, err := app.Services.Settings.SaveCollection(t.Context(), actor, settings.SaveCollectionRequest{FloatLimitCents: &limit, FloatDirection: &direction}); err != nil {
			t.Fatalf("configure a single available actual amount: %v", err)
		}
		ttl := 60
		first := createReservationOrder(t, db, app, merchantID, "expired-slot-hold", 100, "alipay", &ttl)
		db.Clock.Set(first.ExpiresAt)
		if expired, err := app.Services.Payment.ExpireBatch(t.Context(), 10); err != nil || expired != 1 {
			t.Fatalf("expire amount reservation order at its deadline: count=%d err=%v", expired, err)
		}
		if _, err := app.Services.Payment.Create(t.Context(), reservationCommand(merchantID, "blocked-before-slot-release", 100, "alipay", nil)); errorCode(err) != "AMOUNT_CAPACITY_EXHAUSTED" {
			t.Fatalf("expired reservation released before 24 hours: %v", err)
		}
		db.Clock.Set(first.ExpiresAt.Add(24 * time.Hour))
		deviceIdentity, err := app.Services.Device.Authenticate(t.Context(), deviceToken)
		if err != nil {
			t.Fatalf("authenticate billing fixture device after advancing the clock: %v", err)
		}
		if _, err := app.Services.Device.Heartbeat(t.Context(), deviceIdentity, device.HeartbeatRequest{AppVersion: "1.0"}); err != nil {
			t.Fatalf("restore billing fixture device online after advancing the clock: %v", err)
		}
		after := createReservationOrder(t, db, app, merchantID, "available-after-slot-release", 100, "alipay", nil)
		if after.ActualCents == nil || *after.ActualCents != 100 {
			t.Fatalf("expired reservation remained after 24 hours: %#v", after.ActualCents)
		}
	})
	t.Run("V31.B.r05/v03", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, key := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "cross-service amount reservations", 1000, 3650, []string{"alipay"}, nil)
		firstApp := db.NewApplication(t)
		secondApp := db.NewApplication(t)
		db.PrepareBillingMerchant(t, firstApp, merchantID, key, planID)
		const workers = 20
		barrier := testsupport.NewBarrier(workers)
		type outcome struct{ err error }
		outcomes := make(chan outcome, workers)
		for i := 0; i < workers; i++ {

			go func() {
				if err := barrier.Wait(t.Context()); err != nil {
					outcomes <- outcome{err: err}
					return
				}
				service := firstApp.Services.Payment
				if i%2 == 1 {
					service = secondApp.Services.Payment
				}
				_, err := service.Create(t.Context(), reservationCommand(merchantID, fmt.Sprintf("cross-service-%02d", i), 100, "alipay", nil))
				outcomes <- outcome{err: err}
			}()
		}
		var created, exhausted int
		for i := 0; i < workers; i++ {
			result := <-outcomes
			switch {
			case result.err == nil:
				created++
			case errorCode(result.err) == "AMOUNT_CAPACITY_EXHAUSTED":
				exhausted++
			default:
				t.Fatalf("cross-service reservation error: %v", result.err)
			}
		}
		if created != 11 || exhausted != 9 {
			t.Fatalf("cross-service reservation outcomes created=%d exhausted=%d", created, exhausted)
		}
		var distinct int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(DISTINCT actual_cents) FROM orders WHERE merchant_id=$1 AND out_trade_no LIKE 'cross-service-%'`, merchantID).Scan(&distinct); err != nil || distinct != 11 {
			t.Fatalf("cross-service actual amounts=%d err=%v, want 11 unique", distinct, err)
		}
	})
}

func reservationCommand(merchantID int64, out string, cents int64, channel string, ttl *int) payment.CreateCommand {
	return payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: out, Name: out, OriginalCents: cents, NotifyURL: "https://merchant.example/notify", SourceIP: "192.0.2.31", Channel: channel, ExpireSeconds: ttl, Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}}
}

func createReservationOrder(t *testing.T, db *testsupport.Database, app *app.App, merchantID int64, out string, cents int64, channel string, ttl *int) payment.OrderDTO {
	t.Helper()
	order, err := app.Services.Payment.Create(t.Context(), reservationCommand(merchantID, out, cents, channel, ttl))
	if err != nil {
		t.Fatalf("create amount reservation %s: %v", out, err)
	}
	return order
}
