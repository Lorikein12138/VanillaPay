package payments

import (
	"strings"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_TestPaymentSkipsProtocolRiskAndReplacesPendingTestOrder(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	planID := db.SeedPlan(t, "test payment plan", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	ctx := t.Context()
	// Protocol-only risk rules would block this order: a block word matching
	// the fixed name and a notify-domain allowlist.
	if _, err := db.Pool.Exec(ctx, `UPDATE site_settings SET risk=risk || '{"name_block_words":["测试"]}'::jsonb WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO merchant_notify_domains(merchant_id,enabled,entries) VALUES($1,true,'{merchant.example.test}')`, merchantID); err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID, IP: "198.51.100.7"}
	service := fixture.App.Services.Payment

	if _, err := service.Create(ctx, payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "blocked-normal", Name: "测试商品", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.10", Actor: actor}); errorCode(err) != "NAME_BLOCKED" {
		t.Fatalf("normal order with blocked name err=%v, want NAME_BLOCKED", err)
	}

	first, err := service.CreateTestPayment(ctx, actor, 100, "alipay")
	if err != nil {
		t.Fatalf("create first test payment: %v", err)
	}
	type row struct {
		outTrade, name, status string
		reason                 *string
		isTest, hasNotify      bool
		releaseSet             *bool
	}
	read := func(trade string) row {
		t.Helper()
		var r row
		if err := db.Pool.QueryRow(ctx, `SELECT o.out_trade_no,o.name,o.status,o.expire_reason,o.is_test,o.notify_url IS NOT NULL,
			(SELECT a.release_after IS NOT NULL FROM amount_reservations a WHERE a.order_id=o.id) FROM orders o WHERE o.trade_no=$1`, trade).
			Scan(&r.outTrade, &r.name, &r.status, &r.reason, &r.isTest, &r.hasNotify, &r.releaseSet); err != nil {
			t.Fatalf("read order %s: %v", trade, err)
		}
		return r
	}
	got := read(first.TradeNo)
	if !got.isTest || got.hasNotify || got.status != "pending" || got.name != "测试支付" || !strings.HasPrefix(got.outTrade, "TEST") || got.releaseSet == nil || *got.releaseSet {
		t.Fatalf("first test order = %+v", got)
	}

	if _, err := service.CreateTestPayment(ctx, actor, 100, "wxpay"); errorCode(err) != "CHANNEL_NOT_ALLOWED" {
		t.Fatalf("failed replacement err=%v, want CHANNEL_NOT_ALLOWED", err)
	}
	if got = read(first.TradeNo); got.status != "pending" || got.releaseSet == nil || *got.releaseSet {
		t.Fatalf("failed replacement changed the first test order: %+v", got)
	}

	second, err := service.CreateTestPayment(ctx, actor, 200, "")
	if err != nil {
		t.Fatalf("create second test payment: %v", err)
	}
	got = read(first.TradeNo)
	if got.status != "expired" || got.reason == nil || *got.reason != "cancelled" || got.releaseSet == nil || !*got.releaseSet {
		t.Fatalf("previous test order was not closed: %+v", got)
	}
	if got = read(second.TradeNo); got.status != "pending" || !got.isTest {
		t.Fatalf("second test order = %+v", got)
	}
	var closeAudits int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='order.close' AND target_id=$1`, first.TradeNo).Scan(&closeAudits); err != nil || closeAudits != 1 {
		t.Fatalf("close audits=%d err=%v", closeAudits, err)
	}

	t.Run("normal pending orders are not closed by a test payment", func(t *testing.T) {
		normal, err := service.Create(ctx, payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "normal-pending", Name: "会员", OriginalCents: 300, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.10", Actor: actor})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreateTestPayment(ctx, actor, 100, ""); err != nil {
			t.Fatal(err)
		}
		if got := read(normal.TradeNo); got.status != "pending" || got.isTest {
			t.Fatalf("normal order changed: %+v", got)
		}
	})

	t.Run("validation and actor checks", func(t *testing.T) {
		for _, cents := range []int64{0, payment.MaxAmountCents + 1} {
			if _, err := service.CreateTestPayment(ctx, actor, cents, ""); errorCode(err) != "VALIDATION_ERROR" {
				t.Fatalf("amount %d err=%v", cents, err)
			}
		}
		if _, err := service.CreateTestPayment(ctx, actor, 100, "wxpay"); errorCode(err) != "CHANNEL_NOT_ALLOWED" {
			t.Fatalf("channel outside plan err=%v", err)
		}
		if _, err := service.CreateTestPayment(ctx, platform.Actor{Role: "admin", SubjectID: 1}, 100, ""); errorCode(err) != "UNAUTHENTICATED" {
			t.Fatalf("admin actor err=%v", err)
		}
	})

	t.Run("paid test order schedules no callback and is flagged in the portal", func(t *testing.T) {
		latest, err := service.CreateTestPayment(ctx, actor, 100, "alipay")
		if err != nil {
			t.Fatal(err)
		}
		orderID, err := service.OrderID(ctx, latest.TradeNo)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.MarkPaidTx(ctx, tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var events int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM callback_events WHERE order_id=$1`, orderID).Scan(&events); err != nil || events != 0 {
			t.Fatalf("test order callback events=%d err=%v", events, err)
		}
		page, err := fixture.App.Services.Portal.ListOrders(ctx, actor, portal.OrderFilter{Query: latest.TradeNo, Limit: 10})
		if err != nil || len(page.Items) != 1 || !page.Items[0].IsTest || page.Items[0].CallbackStatus != "none" {
			t.Fatalf("portal list = %+v err=%v", page.Items, err)
		}
		if _, err := service.ReplayCallback(ctx, actor, latest.TradeNo, "test-payment-replay"); errorCode(err) != "CALLBACK_UNAVAILABLE" {
			t.Fatalf("replay on test order err=%v", err)
		}
	})
}
