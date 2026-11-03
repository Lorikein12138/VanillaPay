package subscriptions

import (
	"testing"
	"time"

	"vanillapay/internal/app"
	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_SubscriptionEntitlementTransitions(t *testing.T) {
	db := testsupport.NewDatabase(t)
	billingID, billingKey := db.SeedMerchant(t)
	buyerID, _ := db.SeedMerchant(t)
	firstPlan := db.SeedPlan(t, "entitlement-original", 1000, 30, []string{"alipay"}, nil)
	secondPlan := db.SeedPlan(t, "entitlement-replacement", 1800, 60, []string{"wxpay"}, nil)
	billingPlan := db.SeedPlan(t, "entitlement-billing", 1000, 3650, []string{"alipay", "wxpay"}, nil)
	app := db.NewApplication(t)
	deviceToken := db.PrepareBillingMerchant(t, app, billingID, billingKey, billingPlan)
	actor := platform.Actor{Role: "merchant", SubjectID: buyerID, MerchantID: buyerID}

	t.Run("V24.B.r01/v01", func(t *testing.T) {
		current, err := app.Services.Subscription.GetCurrent(t.Context(), actor)
		if err != nil || current.Active || current.Source != "none" {
			t.Fatalf("unsubscribed merchant entitlement = %#v, err=%v", current, err)
		}
		_, err = app.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: buyerID, Protocol: "v1", Purpose: "external", OutTradeNo: "no-subscription-order",
			Name: "blocked", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify",
			ReturnURL: "https://merchant.example.test/return", SourceIP: "192.0.2.25", Actor: actor,
		})
		if !hasErrorCode(err, "SUBSCRIPTION_INACTIVE") {
			t.Fatalf("external order without an active subscription error=%v, want SUBSCRIPTION_INACTIVE", err)
		}
	})

	first, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(firstPlan), PlanRevision: 1}, "entitlement-initial-001")
	if err != nil || first.TradeNo == nil || first.State != "pending" {
		t.Fatalf("initial subscription purchase = %#v, err=%v", first, err)
	}
	completeEntitlementPurchase(t, db, app, deviceToken, first, "123e4567-e89b-12d3-a456-426614174301")

	current, err := app.Services.Subscription.GetCurrent(t.Context(), actor)
	if err != nil || !current.Active || current.ExpiresAt == nil || current.Plan == nil || current.Plan.ID != itoa(firstPlan) {
		t.Fatalf("fulfilled initial entitlement = %#v, err=%v", current, err)
	}
	firstExpiry := *current.ExpiresAt
	renewal, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(firstPlan), PlanRevision: 1, ExpectedSubscriptionRevision: current.Revision}, "entitlement-renew-001")
	if err != nil || renewal.PurchaseKind != "renew" {
		t.Fatalf("same-plan renewal = %#v, err=%v", renewal, err)
	}
	completeEntitlementPurchase(t, db, app, deviceToken, renewal, "123e4567-e89b-12d3-a456-426614174302")
	t.Run("V24.B.r02/v01", func(t *testing.T) {
		updated, err := app.Services.Subscription.GetCurrent(t.Context(), actor)
		if err != nil || updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(firstExpiry.Add(30*24*time.Hour)) {
			t.Fatalf("renewal expiry=%v, want previous expiry+30d (%v), err=%v", updated.ExpiresAt, firstExpiry.Add(30*24*time.Hour), err)
		}
	})

	// Expire the entitlement using the fixture clock, then start a new fixed-duration chain.
	db.Clock.Set(firstExpiry.Add(31 * 24 * time.Hour))
	billingDevice, err := app.Services.Device.Authenticate(t.Context(), deviceToken)
	if err != nil {
		t.Fatalf("authenticate billing device before expired-chain purchase: %v", err)
	}
	if _, err := app.Services.Device.Heartbeat(t.Context(), billingDevice, device.HeartbeatRequest{AppVersion: "billing-test"}); err != nil {
		t.Fatalf("refresh billing-device online status before expired-chain purchase: %v", err)
	}
	current, err = app.Services.Subscription.GetCurrent(t.Context(), actor)
	if err != nil || current.Active {
		t.Fatalf("expired entitlement remained active: %#v, err=%v", current, err)
	}
	expiredPurchase, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(secondPlan), PlanRevision: 1, ExpectedSubscriptionRevision: current.Revision, ConfirmReplace: true}, "entitlement-expired-new-001")
	if err != nil || expiredPurchase.TradeNo == nil {
		t.Fatalf("purchase after expiration = %#v, err=%v", expiredPurchase, err)
	}
	completeEntitlementPurchase(t, db, app, deviceToken, expiredPurchase, "123e4567-e89b-12d3-a456-426614174303")
	t.Run("V24.B.r02/v02", func(t *testing.T) {
		var paidAt time.Time
		if err := db.Pool.QueryRow(t.Context(), `SELECT paid_at FROM purchases WHERE public_id=$1`, expiredPurchase.ID).Scan(&paidAt); err != nil {
			t.Fatal("read new chain paid_at")
		}
		updated, err := app.Services.Subscription.GetCurrent(t.Context(), actor)
		if err != nil || updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(paidAt.Add(60*24*time.Hour)) {
			t.Fatalf("new expiry=%v, want paid_at+60d (%v), err=%v", updated.ExpiresAt, paidAt.Add(60*24*time.Hour), err)
		}
	})

	// A different package cannot replace a still-active chain without explicit confirmation.
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	grantExpiry := db.Clock.Now().Add(40 * 24 * time.Hour)
	grant, err := app.Services.Subscription.Grant(t.Context(), admin, buyerID, subscription.GrantRequest{
		PlanID: itoa(firstPlan), ExpiresAt: grantExpiry, Channels: []string{"alipay"}, ExpectedRevision: current.Revision + 1, Reason: "switch fixture",
	})
	if err != nil {
		t.Fatalf("prepare active entitlement for switch: %v", err)
	}
	_, err = app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(secondPlan), PlanRevision: 1, ExpectedSubscriptionRevision: grant.Revision}, "entitlement-switch-no-confirm")
	t.Run("V24.B.r03/v01", func(t *testing.T) {
		if !hasErrorCode(err, "VERSION_CONFLICT") {
			t.Fatalf("unconfirmed plan switch error=%v, want VERSION_CONFLICT", err)
		}
	})
	switchPurchase, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{PlanID: itoa(secondPlan), PlanRevision: 1, ExpectedSubscriptionRevision: grant.Revision, ConfirmReplace: true}, "entitlement-switch-confirmed")
	if err != nil || switchPurchase.PurchaseKind != "switch" {
		t.Fatalf("confirmed plan switch = %#v, err=%v", switchPurchase, err)
	}
	completeEntitlementPurchase(t, db, app, deviceToken, switchPurchase, "123e4567-e89b-12d3-a456-426614174304")
	t.Run("V24.B.r03/v02", func(t *testing.T) {
		updated, err := app.Services.Subscription.GetCurrent(t.Context(), actor)
		if err != nil || updated.Plan == nil || updated.Plan.ID != itoa(secondPlan) || updated.Source != "purchase" {
			t.Fatalf("confirmed switch entitlement = %#v, err=%v", updated, err)
		}
		var oldPlanInActiveChain int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM subscription_grants g JOIN purchases p ON p.id=g.purchase_id WHERE g.merchant_id=$1 AND g.chain_id=(SELECT active_chain_id FROM subscriptions WHERE merchant_id=$1) AND p.plan_id=$2`, buyerID, firstPlan).Scan(&oldPlanInActiveChain); err != nil || oldPlanInActiveChain != 0 {
			t.Fatalf("old plan grants in the active entitlement chain=%d, err=%v", oldPlanInActiveChain, err)
		}
	})

	current, err = app.Services.Subscription.GetCurrent(t.Context(), actor)
	if err != nil || !current.Active {
		t.Fatalf("prepare active entitlement before pending-order expiry scenario: %#v err=%v", current, err)
	}
	pending, err := app.Services.Subscription.Purchase(t.Context(), actor, subscription.PurchaseRequest{
		PlanID: itoa(secondPlan), PlanRevision: 1, ExpectedSubscriptionRevision: current.Revision,
	}, "pending-order-after-expiry")
	if err != nil || pending.TradeNo == nil {
		t.Fatalf("create subscription order before entitlement expires: %#v err=%v", pending, err)
	}
	locked, err := app.Services.Payment.SelectChannel(t.Context(), *pending.TradeNo, "wxpay")
	if err != nil || locked.Channel == nil || *locked.Channel != "wxpay" || locked.ActualCents == nil {
		t.Fatalf("lock pending subscription order channel snapshot: %#v err=%v", locked, err)
	}
	if _, err := db.Pool.Exec(t.Context(), `UPDATE subscriptions SET expires_at=$1 WHERE merchant_id=$2`, db.Clock.Now().Add(-time.Second), buyerID); err != nil {
		t.Fatal("expire buyer entitlement after channel selection")
	}
	t.Run("V24.B.r04/v02", func(t *testing.T) {
		_, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: buyerID, Protocol: "v1", Purpose: "external", OutTradeNo: "new-order-after-entitlement-expiry",
			Name: "blocked after expiry", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify",
			SourceIP: "192.0.2.26", Actor: actor,
		})
		if !hasErrorCode(err, "SUBSCRIPTION_INACTIVE") {
			t.Fatalf("new external order after entitlement expiry error=%v, want SUBSCRIPTION_INACTIVE", err)
		}
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no='new-order-after-entitlement-expiry'`, buyerID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("expired buyer got %d new orders, err=%v", count, err)
		}
	})
	t.Run("V24.B.r04/v01", func(t *testing.T) {
		if _, err := app.Services.Payment.SelectChannel(t.Context(), *pending.TradeNo, "wxpay"); err != nil {
			t.Fatalf("selected pending order lost its channel snapshot after entitlement expiry: %v", err)
		}
		completeEntitlementPurchase(t, db, app, deviceToken, pending, "123e4567-e89b-12d3-a456-426614174405")
		var state string
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE trade_no=$1`, *pending.TradeNo).Scan(&state); err != nil || state != "paid" {
			t.Fatalf("pre-existing selected order status=%q err=%v, want paid", state, err)
		}
	})
}

func completeEntitlementPurchase(t *testing.T, db *testsupport.Database, app *app.App, deviceToken string, purchase subscription.PurchaseDTO, eventID string) {
	t.Helper()
	if purchase.TradeNo == nil || len(purchase.PlanSnapshot.Channels) == 0 {
		t.Fatalf("purchase fixture lacks a trade number or payment channel: %#v", purchase)
	}
	channel := purchase.PlanSnapshot.Channels[0]
	checkout, err := app.Services.Payment.SelectChannel(t.Context(), *purchase.TradeNo, channel)
	if err != nil || checkout.ActualCents == nil {
		t.Fatalf("lock subscription order payment snapshot: %#v err=%v", checkout, err)
	}
	identity, err := app.Services.Device.Authenticate(t.Context(), deviceToken)
	if err != nil {
		t.Fatalf("authenticate billing device: %v", err)
	}
	result, err := app.Services.Device.Ingest(t.Context(), identity, device.ReceiptRequest{
		EventID: eventID, Channel: channel, AmountCents: *checkout.ActualCents,
		OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: "provider-" + eventID,
	})
	if err != nil || result.MatchState != "matched" {
		t.Fatalf("settle subscription purchase with billing device: %#v err=%v", result, err)
	}
}
