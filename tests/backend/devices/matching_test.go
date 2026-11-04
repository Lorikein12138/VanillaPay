package devices

import (
	"context"
	"testing"
	"time"

	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_DeviceReceiptMatchesAndPaysOneRealOrderExactlyOnce(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "merchant entitlement", 1000, 3650, []string{"alipay"}, nil)
	application := db.NewApplication(t)
	token := db.PrepareBillingMerchant(t, application, merchantID, merchantKey, planID)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	created, err := application.Services.Payment.Create(context.Background(), payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "device-order-1", Name: "matching order", OriginalCents: 100, NotifyURL: "https://merchant.example/notify", ClientIP: "198.51.100.20", SourceIP: "198.51.100.21", Actor: actor})
	if err != nil {
		t.Fatal(err)
	}
	checkout, err := application.Services.Payment.SelectChannel(context.Background(), created.TradeNo, "alipay")
	if err != nil || checkout.ActualCents == nil {
		t.Fatalf("select channel = %#v, %v", checkout, err)
	}
	identity, err := application.Services.Device.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	request := deviceReceipt("123e4567-e89b-12d3-a456-426614174000", *checkout.ActualCents, db.Clock.Now().Add(time.Second))
	first, err := application.Services.Device.Ingest(context.Background(), identity, request)
	if err != nil || first.MatchState != "matched" || first.TradeNo == nil || *first.TradeNo != created.TradeNo {
		t.Fatalf("first receipt = %#v, %v", first, err)
	}
	second, err := application.Services.Device.Ingest(context.Background(), identity, request)
	if err != nil || !second.Duplicate || second.ReceiptID != first.ReceiptID {
		t.Fatalf("receipt replay = %#v, %v", second, err)
	}
	order, err := application.Services.Payment.Query(context.Background(), actor, payment.OrderSelector{TradeNo: created.TradeNo, MerchantID: merchantID})
	if err != nil || order.Status != "paid" {
		t.Fatalf("matched order = %#v, %v", order, err)
	}
	var manual bool
	if err := db.Pool.QueryRow(context.Background(), `SELECT is_manual FROM orders WHERE trade_no=$1`, created.TradeNo).Scan(&manual); err != nil || manual {
		t.Fatalf("matched order is_manual = %t, %v", manual, err)
	}
	var receipts int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM receipts WHERE event_id=$1::uuid`, request.EventID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 {
		t.Fatalf("persisted receipt count = %d", receipts)
	}
	t.Run("V42.B.r02/v01", func(t *testing.T) {
		if first.ReceiptID == "" || second.ReceiptID != first.ReceiptID {
			t.Fatalf("identical event replay receipt IDs first=%q replay=%q", first.ReceiptID, second.ReceiptID)
		}
	})
	t.Run("V42.B.r02/v02", func(t *testing.T) {
		if !second.Duplicate {
			t.Fatal("same event and content replay was not marked duplicate")
		}
	})
	t.Run("V42.B.r02/v03", func(t *testing.T) {
		var callbackEvents int
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM callback_events WHERE order_id=(SELECT id FROM orders WHERE trade_no=$1)`, created.TradeNo).Scan(&callbackEvents); err != nil || callbackEvents != 1 || receipts != 1 {
			t.Fatalf("duplicate event caused receipt/callback count=%d/%d err=%v, want 1/1", receipts, callbackEvents, err)
		}
	})
	t.Run("V43.B.r01/v01", func(t *testing.T) {
		if first.MatchState != "matched" || first.TradeNo == nil || *first.TradeNo != created.TradeNo || request.AmountCents != *checkout.ActualCents || request.Channel != *checkout.Channel {
			t.Fatalf("eligible exact-amount receipt match=%#v request=%#v checkout=%#v", first, request, checkout)
		}
	})
	t.Run("V43.B.r01/v02", func(t *testing.T) {
		if order.Status != "paid" || manual {
			t.Fatalf("automatic device match produced status=%q is_manual=%t", order.Status, manual)
		}
	})
	t.Run("V44.B.r02/v01", func(t *testing.T) {
		if order.Status != "paid" {
			t.Fatalf("matching receipt did not atomically mark order paid: %q", order.Status)
		}
	})
	t.Run("V44.B.r02/v02", func(t *testing.T) {
		var state string
		if err := db.Pool.QueryRow(context.Background(), `SELECT match_state FROM receipts WHERE event_id=$1::uuid`, request.EventID).Scan(&state); err != nil || state != "matched" {
			t.Fatalf("committed receipt match_state=%q err=%v", state, err)
		}
	})
	t.Run("V44.B.r02/v03", func(t *testing.T) {
		var callbacks int
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM callback_events WHERE order_id=(SELECT id FROM orders WHERE trade_no=$1)`, created.TradeNo).Scan(&callbacks); err != nil || callbacks != 1 {
			t.Fatalf("receipt payment callback generations=%d err=%v, want one durable generation", callbacks, err)
		}
	})
	t.Run("V42.B.r04/v01", func(t *testing.T) {
		conflicting := request
		conflicting.EventID = "123e4567-e89b-12d3-a456-426614174099"
		if _, err := application.Services.Device.Ingest(context.Background(), identity, conflicting); !hasCode(err, "EVENT_CONFLICT") {
			t.Fatalf("same provider trade number with a different event error=%v, want EVENT_CONFLICT", err)
		}
		var receipts, paid int
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM receipts WHERE merchant_id=$1 AND provider_trade_no='provider-trade-1'`, merchantID).Scan(&receipts); err != nil {
			t.Fatal(err)
		}
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM orders WHERE trade_no=$1 AND status='paid'`, created.TradeNo).Scan(&paid); err != nil || receipts != 1 || paid != 1 {
			t.Fatalf("provider duplicate changed business counts receipts=%d paid=%d err=%v", receipts, paid, err)
		}
	})
	t.Run("V42.B.r03/v02", func(t *testing.T) {
		if err := application.Services.Device.Revoke(context.Background(), actor); err != nil {
			t.Fatalf("revoke prior device before device replacement: %v", err)
		}
		binding, err := application.Services.Device.NewBinding(context.Background(), actor, false)
		if err != nil {
			t.Fatalf("issue replacement device binding: %v", err)
		}
		replacement, err := application.Services.Device.Redeem(context.Background(), device.RedeemRequest{Code: binding.Code, DeviceName: "replacement billing device", AppVersion: "2.0"})
		if err != nil {
			t.Fatalf("redeem replacement device: %v", err)
		}
		replacementIdentity, err := application.Services.Device.Authenticate(context.Background(), replacement.DeviceToken)
		if err != nil {
			t.Fatalf("authenticate replacement device: %v", err)
		}
		replay, err := application.Services.Device.Ingest(context.Background(), replacementIdentity, request)
		if err != nil || !replay.Duplicate || replay.ReceiptID != first.ReceiptID {
			t.Fatalf("same merchant event replay from replacement device=%#v err=%v", replay, err)
		}
	})
}

func TestDB_ReceiptCandidateBoundaryAndUnmatchedEvidence(t *testing.T) {
	t.Run("V43.B.r02/v01", func(t *testing.T) {
		for _, scenario := range []string{"other_merchant", "wrong_channel", "wrong_amount", "before_channel_lock", "future_over_30_seconds", "received_after_expiry", "occurred_after_expiry"} {
			t.Run(scenario, func(t *testing.T) {
				db := testsupport.NewDatabase(t)
				merchantID, merchantKey := db.SeedMerchant(t)
				planID := db.SeedPlan(t, "candidate boundary "+scenario, 1000, 3650, []string{"alipay"}, nil)
				app := db.NewApplication(t)
				deviceToken := db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
				identity, err := app.Services.Device.Authenticate(t.Context(), deviceToken)
				if err != nil {
					t.Fatal(err)
				}
				orderMerchantID := merchantID
				if scenario == "other_merchant" {
					otherMerchantID, otherKey := db.SeedMerchant(t)
					orderMerchantID = otherMerchantID
					otherPlan := db.SeedPlan(t, "other merchant plan", 1000, 3650, []string{"alipay"}, nil)
					db.PrepareBillingMerchant(t, app, orderMerchantID, otherKey, otherPlan)
				}
				actor := platform.Actor{Role: "merchant", SubjectID: orderMerchantID, MerchantID: orderMerchantID}
				trade := "candidate-" + scenario
				order, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{MerchantID: orderMerchantID, Protocol: "v1", Purpose: "external", OutTradeNo: trade, Name: "candidate boundary", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.141", Actor: actor})
				if err != nil {
					t.Fatalf("create candidate order: %v", err)
				}
				checkout, err := app.Services.Payment.SelectChannel(t.Context(), order.TradeNo, "alipay")
				if err != nil || checkout.ActualCents == nil {
					t.Fatalf("select candidate channel: %#v err=%v", checkout, err)
				}
				if scenario == "other_merchant" {
					receipt := device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174501", Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: db.Clock.Now().Add(time.Second), ProviderTradeNo: "candidate-provider-other-merchant"}
					result, err := app.Services.Device.Ingest(t.Context(), identity, receipt)
					if err != nil || result.MatchState != "unmatched" || result.TradeNo != nil {
						t.Fatalf("M1 receipt matched M2 order: result=%#v err=%v", result, err)
					}
					var status string
					if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE trade_no=$1`, order.TradeNo).Scan(&status); err != nil || status != "pending" {
						t.Fatalf("cross-merchant receipt changed M2 order status=%q err=%v", status, err)
					}
					return
				}
				channel := "alipay"
				occurred := db.Clock.Now().Add(time.Second)
				amount := *checkout.ActualCents
				switch scenario {
				case "wrong_channel":
					channel = "wxpay"
				case "wrong_amount":
					amount++
				case "before_channel_lock":
					occurred = db.Clock.Now().Add(-time.Second)
				case "future_over_30_seconds":
					occurred = db.Clock.Now().Add(31 * time.Second)
				case "received_after_expiry":
					db.Clock.Set(order.ExpiresAt)
					occurred = order.ExpiresAt.Add(-time.Second)
				case "occurred_after_expiry":
					occurred = order.ExpiresAt
				}
				receipt := device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-4266141745" + scenarioEventSuffix(scenario), Channel: channel, AmountCents: amount, OccurredAt: occurred, ProviderTradeNo: "candidate-provider-" + scenario, RawText: "restricted-original"}
				result, err := app.Services.Device.Ingest(t.Context(), identity, receipt)
				if err != nil || result.MatchState != "unmatched" || result.TradeNo != nil {
					t.Fatalf("ineligible receipt was matched: %#v err=%v", result, err)
				}
				var state string
				if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE trade_no=$1`, order.TradeNo).Scan(&state); err != nil || state != "pending" {
					t.Fatalf("ineligible receipt changed order state=%q err=%v", state, err)
				}
			})
		}
	})

	t.Run("V43.B.r03/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, merchantKey := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "unmatched evidence", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		deviceToken := db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
		identity, err := app.Services.Device.Authenticate(t.Context(), deviceToken)
		if err != nil {
			t.Fatal(err)
		}
		noOrder, err := app.Services.Device.Ingest(t.Context(), identity, device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174510", Channel: "alipay", AmountCents: 991, OccurredAt: db.Clock.Now(), ProviderTradeNo: "unmatched-no-order", RawText: "original text remains encrypted at rest"})
		if err != nil || noOrder.MatchState != "unmatched" || noOrder.UnmatchedReason == nil || *noOrder.UnmatchedReason != "no_matching_order" {
			t.Fatalf("unmatched no-order receipt=%#v err=%v", noOrder, err)
		}
		expiredOrder, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "unmatched-expired-order", Name: "expired candidate", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.142", Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}})
		if err != nil {
			t.Fatal("create expiring candidate order")
		}
		checkout, err := app.Services.Payment.SelectChannel(t.Context(), expiredOrder.TradeNo, "alipay")
		if err != nil || checkout.ActualCents == nil {
			t.Fatalf("select expiring candidate channel: %#v err=%v", checkout, err)
		}
		db.Clock.Set(expiredOrder.ExpiresAt)
		late, err := app.Services.Device.Ingest(t.Context(), identity, device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174511", Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: expiredOrder.ExpiresAt, ProviderTradeNo: "unmatched-expired", RawText: "late full original"})
		if err != nil || late.MatchState != "unmatched" || late.UnmatchedReason == nil || *late.UnmatchedReason != "expired" {
			t.Fatalf("expired receipt=%#v err=%v", late, err)
		}
		if _, err := app.Services.Merchant.Ban(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, merchantID, "receipt policy fixture"); err != nil {
			t.Fatal("ban merchant for receipt reason fixture")
		}
		banned, err := app.Services.Device.Ingest(t.Context(), identity, device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174512", Channel: "alipay", AmountCents: 992, OccurredAt: db.Clock.Now(), ProviderTradeNo: "unmatched-banned", RawText: "banned full original"})
		if err != nil || banned.MatchState != "unmatched" || banned.UnmatchedReason == nil || *banned.UnmatchedReason != "merchant_banned" {
			t.Fatalf("banned receipt=%#v err=%v", banned, err)
		}
		for _, item := range []struct{ id, raw string }{{noOrder.ReceiptID, "original text remains encrypted at rest"}, {late.ReceiptID, "late full original"}, {banned.ReceiptID, "banned full original"}} {
			detail, err := app.Services.Device.OpenReceipt(t.Context(), platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}, item.id)
			if err != nil || detail["raw_text"] != item.raw {
				t.Fatalf("receipt detail lost complete original evidence id=%s data=%#v err=%v", item.id, detail, err)
			}
		}
	})

	t.Run("V43.B.r04/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, merchantKey := db.SeedMerchant(t)
		planID := db.SeedPlan(t, "receipt list filters", 1000, 3650, []string{"alipay"}, nil)
		app := db.NewApplication(t)
		token := db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
		identity, err := app.Services.Device.Authenticate(t.Context(), token)
		if err != nil {
			t.Fatal(err)
		}
		order, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "receipt-list-match", Name: "list filter", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.143", Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}})
		if err != nil {
			t.Fatal(err)
		}
		checkout, err := app.Services.Payment.SelectChannel(t.Context(), order.TradeNo, "alipay")
		if err != nil || checkout.ActualCents == nil {
			t.Fatalf("select receipt list channel: %#v err=%v", checkout, err)
		}
		matched, err := app.Services.Device.Ingest(t.Context(), identity, device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174520", Channel: "alipay", AmountCents: *checkout.ActualCents, OccurredAt: db.Clock.Now(), ProviderTradeNo: "receipt-list-matched"})
		if err != nil || matched.MatchState != "matched" {
			t.Fatalf("create matched list item: %#v err=%v", matched, err)
		}
		unmatched, err := app.Services.Device.Ingest(t.Context(), identity, device.ReceiptRequest{EventID: "123e4567-e89b-12d3-a456-426614174521", Channel: "alipay", AmountCents: 990, OccurredAt: db.Clock.Now(), ProviderTradeNo: "receipt-list-unmatched"})
		if err != nil || unmatched.MatchState != "unmatched" {
			t.Fatalf("create unmatched list item: %#v err=%v", unmatched, err)
		}
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		matchedPage, err := app.Services.Portal.ListReceipts(t.Context(), actor, portal.ReceiptFilter{MatchState: "matched", Limit: 20})
		if err != nil || matchedPage.Total != 1 || len(matchedPage.Items) != 1 || matchedPage.Items[0].ID != matched.ReceiptID || matchedPage.Items[0].MatchState != "matched" {
			t.Fatalf("matched receipt filter page=%#v err=%v", matchedPage, err)
		}
		unmatchedPage, err := app.Services.Portal.ListReceipts(t.Context(), actor, portal.ReceiptFilter{MatchState: "unmatched", Limit: 20})
		if err != nil || unmatchedPage.Total != 1 || len(unmatchedPage.Items) != 1 || unmatchedPage.Items[0].ID != unmatched.ReceiptID || unmatchedPage.Items[0].MatchState != "unmatched" {
			t.Fatalf("unmatched receipt filter page=%#v err=%v", unmatchedPage, err)
		}
	})
}

func scenarioEventSuffix(scenario string) string {
	// These seven fixed suffixes keep each event UUID valid and independent.
	switch scenario {
	case "other_merchant":
		return "01"
	case "wrong_channel":
		return "02"
	case "wrong_amount":
		return "03"
	case "before_channel_lock":
		return "04"
	case "future_over_30_seconds":
		return "05"
	case "received_after_expiry":
		return "06"
	default:
		return "07"
	}
}

func deviceReceipt(id string, amount int64, occurredAt time.Time) device.ReceiptRequest {
	return device.ReceiptRequest{EventID: id, Channel: "alipay", AmountCents: amount, OccurredAt: occurredAt, ProviderTradeNo: "provider-trade-1", PayerHint: "payer", RawText: "receipt payload"}
}
