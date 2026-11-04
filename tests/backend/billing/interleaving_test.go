package billing

import (
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/app"
	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_PurchaseInterleavingsAndRefundChains(t *testing.T) {
	db := testsupport.NewDatabase(t)
	billingID, billingKey := db.SeedMerchant(t)
	buyerID, _ := db.SeedMerchant(t)
	planA := db.SeedPlan(t, "chain-original", 1200, 30, []string{"alipay"}, nil)
	planB := db.SeedPlan(t, "chain-switch", 2400, 60, []string{"alipay"}, nil)
	app := db.NewApplication(t)
	token := db.PrepareBillingMerchant(t, app, billingID, billingKey, planA)
	buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
	billingActor := platform.Actor{Role: "merchant", SubjectID: billingID, MerchantID: billingID}

	initial, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{PlanID: decimal(planA), PlanRevision: 1}, "chain-initial-purchase")
	if err != nil {
		t.Fatalf("create original subscription purchase: %v", err)
	}
	settleBillingPurchase(t, db, app, token, initial, "123e4567-e89b-12d3-a456-426614174501")
	current, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
	if err != nil || !current.Active || current.ExpiresAt == nil {
		t.Fatalf("initial entitlement after payment = %#v err=%v", current, err)
	}
	originalExpiry := *current.ExpiresAt
	renewal, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{
		PlanID: decimal(planA), PlanRevision: 1, ExpectedSubscriptionRevision: current.Revision,
	}, "chain-original-renewal")
	if err != nil || renewal.PurchaseKind != "renew" {
		t.Fatalf("create same-plan renewal = %#v err=%v", renewal, err)
	}
	settleBillingPurchase(t, db, app, token, renewal, "123e4567-e89b-12d3-a456-426614174503")
	refundBillingPurchase(t, db, app, billingActor, renewal, "chain-renewal-refund")
	t.Run("V27.B.r04/v01", func(t *testing.T) {
		updated, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if err != nil || updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(originalExpiry) {
			t.Fatalf("renewal refund expiry=%v, want original %v, err=%v", updated.ExpiresAt, originalExpiry, err)
		}
	})
	current, err = app.Services.Subscription.GetCurrent(t.Context(), buyer)
	if err != nil {
		t.Fatal("read current entitlement before confirmed switch")
	}

	// A confirmed switch starts a new chain and voids the original chain. Refunding
	// the switch can remove its own entitlement but must never revive the voided A chain.
	switchPurchase, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{
		PlanID: decimal(planB), PlanRevision: 1, ConfirmReplace: true, ExpectedSubscriptionRevision: current.Revision,
	}, "chain-confirmed-switch")
	if err != nil || switchPurchase.PurchaseKind != "switch" {
		t.Fatalf("create confirmed plan switch = %#v err=%v", switchPurchase, err)
	}
	settleBillingPurchase(t, db, app, token, switchPurchase, "123e4567-e89b-12d3-a456-426614174502")
	refundBillingPurchase(t, db, app, billingActor, switchPurchase, "chain-switch-refund")
	t.Run("V27.B.r04/v02", func(t *testing.T) {
		updated, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if err != nil || updated.Active || updated.Plan != nil {
			t.Fatalf("refunding switched chain restored an old entitlement: %#v err=%v", updated, err)
		}
		var activeOld int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM subscription_grants WHERE purchase_id=(SELECT id FROM purchases WHERE public_id=$1) AND voided_at IS NULL`, initial.ID).Scan(&activeOld); err != nil || activeOld != 0 {
			t.Fatalf("old chain grants active=%d err=%v", activeOld, err)
		}
	})

	// Install an administrator-controlled chain, then refund a purchase that
	// belongs to the already voided chain. Its refund must leave the manual chain intact.
	current, err = app.Services.Subscription.GetCurrent(t.Context(), buyer)
	if err != nil {
		t.Fatal("read revision after switch refund")
	}
	manualExpiry := db.Clock.Now().Add(45 * 24 * time.Hour).Truncate(time.Microsecond)
	manual, err := app.Services.Subscription.Grant(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, buyerID, subscription.GrantRequest{
		PlanID: decimal(planB), ExpiresAt: manualExpiry, Channels: []string{"alipay"}, ExpectedRevision: current.Revision, Reason: "manual chain after refunded switch",
	})
	if err != nil || manual.Source != "manual" || !manual.Active {
		t.Fatalf("install manual replacement chain = %#v err=%v", manual, err)
	}
	refundBillingPurchase(t, db, app, billingActor, initial, "chain-voided-original-refund")
	t.Run("V27.B.r04/v03", func(t *testing.T) {
		updated, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if err != nil || !updated.Active || updated.Source != "manual" || updated.Plan == nil || updated.Plan.ID != decimal(planB) || updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(manualExpiry) {
			t.Fatalf("refund of voided purchase replaced manual chain: %#v err=%v", updated, err)
		}
	})
	t.Run("V27.B.r04/v04", func(t *testing.T) {
		var activeOld int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM subscription_grants WHERE purchase_id=(SELECT id FROM purchases WHERE public_id=$1) AND voided_at IS NULL AND refunded_at IS NULL`, initial.ID).Scan(&activeOld); err != nil || activeOld != 0 {
			t.Fatalf("refund of the voided original purchase restored %d active original grants, want 0; err=%v", activeOld, err)
		}
		current, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if err != nil || current.Source != "manual" || current.ExpiresAt == nil || !current.ExpiresAt.Equal(manualExpiry) {
			t.Fatalf("refund of the voided purchase changed the active manual chain: %#v err=%v", current, err)
		}
	})
	t.Run("V27.B.r04/v05", func(t *testing.T) {
		current, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if err != nil || !current.Active || current.ExpiresAt == nil || !current.ExpiresAt.After(db.Clock.Now()) {
			t.Fatalf("refund chain left a negative/expired active duration: %#v err=%v", current, err)
		}
	})

}

func TestDB_SubscriptionSupplementAndRefundTransitions(t *testing.T) {
	t.Run("V27.B.r03/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		billingID, billingKey := db.SeedMerchant(t)
		buyerID, _ := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "subscription-supplement", 1400, 30, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
		buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
		billing := platform.Actor{Role: "merchant", SubjectID: billingID, MerchantID: billingID}
		purchase, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{PlanID: decimal(planID), PlanRevision: 1}, "subscription-supplement-purchase")
		if err != nil || purchase.TradeNo == nil {
			t.Fatalf("create subscription purchase: %#v err=%v", purchase, err)
		}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), billing, *purchase.TradeNo, payment.SupplementRequest{Reason: "provider verified", Channel: "alipay"}, "subscription-supplement-001"); err != nil {
			t.Fatalf("supplement subscription order: %v", err)
		}
		var state string
		var grants int
		if err := db.Pool.QueryRow(t.Context(), `SELECT state FROM purchases WHERE public_id=$1`, purchase.ID).Scan(&state); err != nil {
			t.Fatal("read purchase after manual supplement")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM subscription_grants WHERE purchase_id=(SELECT id FROM purchases WHERE public_id=$1)`, purchase.ID).Scan(&grants); err != nil {
			t.Fatal("count grant after manual supplement")
		}
		current, currentErr := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if state != "paid" || grants != 1 || currentErr != nil || !current.Active || current.Source != "purchase" {
			t.Fatalf("manual supplement fulfillment state=%s grants=%d current=%#v err=%v", state, grants, current, currentErr)
		}
	})
	t.Run("V27.B.r03/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		billingID, billingKey := db.SeedMerchant(t)
		buyerID, _ := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "subscription-refund", 1400, 30, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		token := db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
		buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
		billing := platform.Actor{Role: "merchant", SubjectID: billingID, MerchantID: billingID}
		purchase, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{PlanID: decimal(planID), PlanRevision: 1}, "subscription-refund-purchase")
		if err != nil {
			t.Fatalf("create subscription purchase: %v", err)
		}
		settleBillingPurchase(t, db, app, token, purchase, "123e4567-e89b-12d3-a456-426614174531")
		refundBillingPurchase(t, db, app, billing, purchase, "subscription-refund-confirmed")
		current, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if err != nil || current.Active || current.Source != "none" {
			t.Fatalf("subscription after purchase refund=%#v err=%v", current, err)
		}
	})
}

func TestDB_DeletedPlanAndBanCancelPendingSubscriptionPurchases(t *testing.T) {
	t.Run("V27.B.r02/v01", func(t *testing.T) {
		// Plan deletion and buyer ban each cancel the pending purchase and its
		// associated internal billing order in their own isolated database.
		{
			db := testsupport.NewDatabase(t)
			billingID, billingKey := db.SeedMerchant(t)
			buyerID, _ := db.SeedMerchant(t)
			planID := db.SeedPlan(t, "pending-plan-delete", 1000, 30, []string{"alipay"}, nil)
			app := db.NewApplication(t)
			db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
			actor := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
			purchase, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: decimal(planID), PlanRevision: 1}, "pending-plan-delete-001")
			if err != nil || purchase.TradeNo == nil {
				t.Fatalf("create pending purchase before plan deletion: %#v err=%v", purchase, err)
			}
			if _, err := app.Services.Subscription.DeletePlan(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, decimal(planID), 1); err != nil {
				t.Fatalf("delete plan with pending purchase: %v", err)
			}
			var state, orderState string
			if err := db.Pool.QueryRow(t.Context(), `SELECT state FROM purchases WHERE public_id=$1`, purchase.ID).Scan(&state); err != nil {
				t.Fatal("read cancelled plan purchase")
			}
			if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE trade_no=$1`, *purchase.TradeNo).Scan(&orderState); err != nil {
				t.Fatal("read expired plan purchase order")
			}
			if state != "cancelled" || orderState != "expired" {
				t.Fatalf("plan deletion left purchase=%s order=%s, want cancelled/expired", state, orderState)
			}
		}
		{
			db := testsupport.NewDatabase(t)
			billingID, billingKey := db.SeedMerchant(t)
			buyerID, _ := db.SeedMerchant(t)
			planID := db.SeedPlan(t, "pending-ban", 1000, 30, []string{"alipay"}, nil)
			app := db.NewApplication(t)
			db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
			buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
			purchase, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{PlanID: decimal(planID), PlanRevision: 1}, "pending-ban-001")
			if err != nil || purchase.TradeNo == nil {
				t.Fatalf("create pending purchase before ban: %#v err=%v", purchase, err)
			}
			if _, err := app.Services.Merchant.Ban(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, buyerID, "V27 purchase cancellation"); err != nil {
				t.Fatalf("ban buyer with pending purchase: %v", err)
			}
			var state string
			if err := db.Pool.QueryRow(t.Context(), `SELECT state FROM purchases WHERE public_id=$1`, purchase.ID).Scan(&state); err != nil || state != "cancelled" {
				t.Fatalf("banned buyer purchase state=%s err=%v, want cancelled", state, err)
			}
		}
	})
	t.Run("V27.B.r02/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		billingID, billingKey := db.SeedMerchant(t)
		buyerID, _ := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "late-payment-after-ban", 1000, 30, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		db.PrepareBillingMerchant(t, app, billingID, billingKey, planID)
		buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
		purchase, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{PlanID: decimal(planID), PlanRevision: 1}, "late-payment-after-ban-001")
		if err != nil || purchase.TradeNo == nil {
			t.Fatalf("create pending purchase before ban: %#v err=%v", purchase, err)
		}
		if _, err := app.Services.Merchant.Ban(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, buyerID, "reject late receipt"); err != nil {
			t.Fatalf("ban buyer before late payment: %v", err)
		}
		var orderID int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT id FROM orders WHERE trade_no=$1`, *purchase.TradeNo).Scan(&orderID); err != nil {
			t.Fatal("find expired purchase order")
		}
		tx, err := db.Pool.Begin(t.Context())
		if err != nil {
			t.Fatal("begin late payment attempt")
		}
		_ = app.Services.Payment.MarkPaidTx(t.Context(), tx, orderID, payment.PaidCommand{Channel: "alipay"})
		_ = tx.Rollback(t.Context())
		var orderState, purchaseState string
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&orderState); err != nil {
			t.Fatal("read order after late payment attempt")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT state FROM purchases WHERE public_id=$1`, purchase.ID).Scan(&purchaseState); err != nil {
			t.Fatal("read purchase after late payment attempt")
		}
		if orderState == "paid" || purchaseState == "paid" {
			t.Fatalf("late payment reopened banned purchase order=%s purchase=%s", orderState, purchaseState)
		}
	})
}

func TestDB_PurchaseAndManualGrantInterleaveThroughDatabaseLocks(t *testing.T) {
	t.Run("V27.B.r01/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		billingID, billingKey := db.SeedMerchant(t)
		buyerID, _ := db.SeedMerchant(t)
		planA := db.SeedPlan(t, "interleaving-renew", 1200, 30, []string{"alipay"}, nil)
		planB := db.SeedPlan(t, "interleaving-manual", 1900, 45, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		token := db.PrepareBillingMerchant(t, app, billingID, billingKey, planA)
		buyer := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}
		base, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{PlanID: decimal(planA), PlanRevision: 1}, "interleave-base-purchase")
		if err != nil {
			t.Fatalf("create initial entitlement purchase: %v", err)
		}
		settleBillingPurchase(t, db, app, token, base, "123e4567-e89b-12d3-a456-426614174541")
		current, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if err != nil || !current.Active {
			t.Fatalf("read initial entitlement: %#v err=%v", current, err)
		}
		pending, err := app.Services.Subscription.Purchase(t.Context(), buyer, subscription.PurchaseRequest{PlanID: decimal(planA), PlanRevision: 1, ExpectedSubscriptionRevision: current.Revision}, "interleave-pending-renewal")
		if err != nil {
			t.Fatalf("create pending renewal before interleave: %v", err)
		}
		checkout, err := app.Services.Payment.SelectChannel(t.Context(), *pending.TradeNo, "alipay")
		if err != nil || checkout.ActualCents == nil {
			t.Fatalf("select channel before interleave: %#v err=%v", checkout, err)
		}
		identity, err := app.Services.Device.Authenticate(t.Context(), token)
		if err != nil {
			t.Fatal("authenticate billing device before interleave")
		}
		gateTx, err := db.Pool.Begin(t.Context())
		if err != nil {
			t.Fatal("begin deterministic interleaving lock")
		}
		var lockedID int64
		if err := gateTx.QueryRow(t.Context(), `SELECT id FROM merchants WHERE id=$1 FOR UPDATE`, buyerID).Scan(&lockedID); err != nil {
			_ = gateTx.Rollback(t.Context())
			t.Fatal("lock buyer serialization row")
		}
		barrier := testsupport.NewBarrier(3)
		type outcome struct {
			name    string
			err     error
			receipt device.ReceiptResult
		}
		outcomes := make(chan outcome, 2)
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			if err := barrier.Wait(t.Context()); err != nil {
				outcomes <- outcome{name: "receipt", err: err}
				return
			}
			receipt, err := app.Services.Device.Ingest(t.Context(), identity, device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174542", Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: "interleave-provider-01"})
			outcomes <- outcome{name: "receipt", receipt: receipt, err: err}
		}()
		go func() {
			defer workers.Done()
			if err := barrier.Wait(t.Context()); err != nil {
				outcomes <- outcome{name: "manual grant", err: err}
				return
			}
			_, err := app.Services.Subscription.Grant(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, buyerID, subscription.GrantRequest{PlanID: decimal(planB), ExpiresAt: db.Clock.Now().Add(45 * 24 * time.Hour), Channels: []string{"alipay"}, ExpectedRevision: current.Revision, Reason: "concurrent renewal and manual replacement"})
			outcomes <- outcome{name: "manual grant", err: err}
		}()
		if err := barrier.Wait(t.Context()); err != nil {
			_ = gateTx.Rollback(t.Context())
			t.Fatal("release concurrent operations")
		}
		if err := gateTx.Commit(t.Context()); err != nil {
			t.Fatal("release buyer serialization lock")
		}
		workers.Wait()
		close(outcomes)
		var receiptResult device.ReceiptResult
		manualApplied := false
		for result := range outcomes {
			if result.name == "receipt" {
				receiptResult = result.receipt
			}
			if result.name == "manual grant" && result.err == nil {
				manualApplied = true
			}
			if result.err != nil && (result.name != "manual grant" || !hasBillingCode(result.err, "VERSION_CONFLICT")) {
				t.Fatalf("%s interleave failed: %v", result.name, result.err)
			}
		}
		var purchaseState string
		if err := db.Pool.QueryRow(t.Context(), `SELECT state FROM purchases WHERE public_id=$1`, pending.ID).Scan(&purchaseState); err != nil {
			t.Fatalf("read pending renewal after serial interleave: %v", err)
		}
		updated, err := app.Services.Subscription.GetCurrent(t.Context(), buyer)
		if err != nil || !updated.Active || updated.ExpiresAt == nil || (updated.Plan == nil || (updated.Plan.ID != decimal(planA) && updated.Plan.ID != decimal(planB))) {
			t.Fatalf("interleaved purchase/grant left invalid current entitlement=%#v err=%v", updated, err)
		}
		switch {
		case purchaseState == "paid" && receiptResult.MatchState == "matched" && !manualApplied && updated.Plan.ID == decimal(planA):
		case purchaseState == "cancelled" && receiptResult.MatchState == "unmatched" && manualApplied && updated.Plan.ID == decimal(planB):
		default:
			t.Fatalf("interleaved purchase/grant was not a legal serialization: purchase=%q receipt=%#v manual_applied=%t entitlement=%#v", purchaseState, receiptResult, manualApplied, updated)
		}
	})
}

func settleBillingPurchase(t *testing.T, db *testsupport.Database, app *app.App, token string, purchase subscription.PurchaseDTO, eventID string) {
	t.Helper()
	if purchase.TradeNo == nil || len(purchase.PlanSnapshot.Channels) == 0 {
		t.Fatalf("purchase lacks a trade or payment channel: %#v", purchase)
	}
	channel := purchase.PlanSnapshot.Channels[0]
	checkout, err := app.Services.Payment.SelectChannel(t.Context(), *purchase.TradeNo, channel)
	if err != nil || checkout.ActualCents == nil {
		t.Fatalf("select billing purchase channel: %#v err=%v", checkout, err)
	}
	identity, err := app.Services.Device.Authenticate(t.Context(), token)
	if err != nil {
		t.Fatalf("authenticate billing device: %v", err)
	}
	result, err := app.Services.Device.Ingest(t.Context(), identity, device.ReceiptRequest{
		EventID: eventID, Channel: channel, AmountCents: *checkout.ActualCents,
		OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: "chain-provider-" + eventID,
	})
	if err != nil || result.MatchState != "matched" {
		t.Fatalf("settle subscription purchase: %#v err=%v", result, err)
	}
}

func refundBillingPurchase(t *testing.T, db *testsupport.Database, app *app.App, actor platform.Actor, purchase subscription.PurchaseDTO, outRefundNo string) {
	t.Helper()
	if purchase.TradeNo == nil {
		t.Fatalf("cannot refund purchase with no trade: %#v", purchase)
	}
	var amount int64
	if err := db.Pool.QueryRow(t.Context(), `SELECT original_cents FROM orders WHERE trade_no=$1`, *purchase.TradeNo).Scan(&amount); err != nil {
		t.Fatalf("read purchase order amount: %v", err)
	}
	digest := sha256.Sum256([]byte(outRefundNo))
	request, err := app.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{
		MerchantID: actor.MerchantID, TradeNo: *purchase.TradeNo, OutRefundNo: outRefundNo,
		AmountCents: amount, Reason: "chain acceptance refund", RequestDigest: digest[:], Actor: actor,
	})
	if err != nil {
		t.Fatalf("request full billing purchase refund: %v", err)
	}
	if err := app.Services.Payment.ResolveRefund(t.Context(), actor, request.ID, payment.RefundResolution{Action: "mark_refunded", Reason: "confirmed provider refund", Confirm: true}); err != nil {
		t.Fatalf("resolve billing purchase refund: %v", err)
	}
	var state string
	if err := db.Pool.QueryRow(t.Context(), `SELECT state FROM purchases WHERE public_id=$1`, purchase.ID).Scan(&state); err != nil || state != "refunded" {
		t.Fatalf("refunded purchase state=%q err=%v", state, err)
	}
}
