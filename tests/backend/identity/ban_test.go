package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"vanillapay/internal/auth"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_BannedMerchantCanAuthenticateWithinRestrictions(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	ctx := context.Background()
	bootstrapCode, err := fixture.App.Services.Auth.RotateBootstrapCode(ctx)
	if err != nil {
		t.Fatal("issue admin bootstrap code for ban contract")
	}
	admin := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", map[string]any{
		"initialization_code": bootstrapCode, "username": "ban_contract_admin", "password": fixturePassword, "password_confirmation": fixturePassword,
	}, "admin:ban-contract")
	if admin.Code != http.StatusCreated {
		t.Fatalf("bootstrap ban-contract admin status=%d body=%s", admin.Code, admin.Body.String())
	}
	if err := configureTOTPTestSiteverify(t, fixture, ctx); err != nil {
		t.Fatalf("configure local Siteverify for banned login: %v", err)
	}
	merchantID, _ := db.SeedMerchant(t)
	password := fixturePassword
	hash, err := auth.HashPassword(password, nil)
	if err != nil {
		t.Fatal("hash banned merchant fixture password")
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE merchants SET password_hash=$1 WHERE id=$2`, hash, merchantID); err != nil {
		t.Fatal("set banned merchant fixture password")
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE site_settings SET security=jsonb_set(security,'{ban_contact}','{"zh_CN":"help-zh@example.test","en":"help-en@example.test"}'::jsonb) WHERE singleton=true`); err != nil {
		t.Fatal("configure exact bilingual banned contact")
	}
	banTime := db.Clock.Now().Truncate(time.Microsecond)
	const reason = "contract policy violation"
	if _, err := fixture.App.Services.Merchant.Ban(ctx, platform.Actor{Role: "admin", SubjectID: 1}, merchantID, reason); err != nil {
		t.Fatalf("ban merchant for HTTP contract: %v", err)
	}
	var username string
	if err := db.Pool.QueryRow(ctx, `SELECT username FROM merchants WHERE id=$1`, merchantID).Scan(&username); err != nil {
		t.Fatal("read banned merchant username")
	}
	var loginData json.RawMessage
	t.Run("V13.B.r01/v01", func(t *testing.T) {
		response := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"identifier": username, "password": password, "turnstile_token": "ok:login",
		}, "merchant:banned-contract")
		loginData = assertIdentityResponse(t, response, http.StatusOK, "banned merchant login")
	})
	t.Run("V13.B.r01/v02", func(t *testing.T) {
		var data struct {
			User struct {
				Banned bool `json:"banned"`
			} `json:"user"`
			NextPath string `json:"next_path"`
		}
		if err := json.Unmarshal(loginData, &data); err != nil || !data.User.Banned || data.NextPath != "/banned" {
			t.Fatalf("banned merchant login session = %#v err=%v", data, err)
		}
	})
	t.Run("V13.B.r02/v01", func(t *testing.T) {
		for _, tc := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, "/api/v1/merchant/me", nil},
			{http.MethodGet, "/api/v1/merchant/banned", nil},
			{http.MethodPatch, "/api/v1/merchant/preferences", map[string]string{"locale": "en", "theme": "dark"}},
		} {
			response := fixture.Request(t, tc.method, tc.path, tc.body, "merchant:banned-contract")
			if response.Code != http.StatusOK {
				t.Errorf("banned merchant allowed route %s %s status=%d body=%s", tc.method, tc.path, response.Code, response.Body.String())
			}
		}
	})
	t.Run("V13.B.r02/v02", func(t *testing.T) {
		response := fixture.Request(t, http.MethodGet, "/api/v1/merchant/orders?limit=20", nil, "merchant:banned-contract")
		var envelope struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if response.Code != http.StatusForbidden || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.Error == nil || envelope.Error.Code != "ACCOUNT_BANNED" {
			t.Fatalf("banned merchant order access status=%d body=%s", response.Code, response.Body.String())
		}
		write := fixture.Request(t, http.MethodPut, "/api/v1/merchant/collection/settings", map[string]any{"order_ttl_seconds": nil}, "merchant:banned-contract")
		if write.Code != http.StatusForbidden {
			t.Fatalf("banned merchant collection write status=%d body=%s", write.Code, write.Body.String())
		}
	})
	t.Run("V13.B.r03/v01", func(t *testing.T) {
		var stored time.Time
		if err := db.Pool.QueryRow(ctx, `SELECT banned_at FROM merchants WHERE id=$1`, merchantID).Scan(&stored); err != nil || !stored.Equal(banTime) {
			t.Fatalf("ban timestamp=%s want occurrence time %s err=%v", stored, banTime, err)
		}
	})
	t.Run("V13.B.r03/v02", func(t *testing.T) {
		response := fixture.Request(t, http.MethodGet, "/api/v1/merchant/banned", nil, "merchant:banned-contract")
		var envelope struct {
			Data struct {
				Banned   bool              `json:"banned"`
				BannedAt *time.Time        `json:"banned_at"`
				Reason   string            `json:"reason"`
				Contact  map[string]string `json:"contact"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusOK || !envelope.Data.Banned || envelope.Data.BannedAt == nil || !envelope.Data.BannedAt.Equal(banTime) || envelope.Data.Reason != reason || envelope.Data.Contact["zh_CN"] != "help-zh@example.test" || envelope.Data.Contact["en"] != "help-en@example.test" {
			t.Fatalf("banned merchant DTO status=%d data=%#v err=%v", response.Code, envelope.Data, err)
		}
	})
}

func TestDB_BanExpiresPendingOrdersAndCancelsPendingPurchasesAtomically(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	billingID, _ := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "monthly", 1200, 30, []string{"alipay"}, nil)
	planSnapshot, _ := json.Marshal(map[string]any{"id": "1", "revision": 1, "name": map[string]string{"zh_CN": "monthly", "en": "monthly"}, "price_cents": 1200, "duration_days": 30, "channels": []string{"alipay"}})
	ctx := context.Background()
	var purchaseID, orderID int64
	expires := db.Clock.Now().Add(time.Hour)
	if err := db.Pool.QueryRow(ctx, `INSERT INTO purchases(public_id,buyer_id,billing_merchant_id,plan_id,plan_snapshot,purchase_kind,expected_subscription_revision,state,idempotency_key,request_digest,expires_at) VALUES('purchase-pending',$1,$1,$2,$3,'new',0,'pending','ban-test',decode(repeat('01',32),'hex'),$4) RETURNING id`, merchantID, planID, planSnapshot, expires).Scan(&purchaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO plan_purchase_counters(merchant_id,plan_id,paid_count,reserved_count) VALUES($1,$2,0,1)`, merchantID, planID); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,original_cents,expires_at,status) VALUES('ban-trade',$1,'ban-order','external','v1','service',1200,$2,'pending') RETURNING id`, merchantID, expires).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	var subscriptionOrderID int64
	if err := db.Pool.QueryRow(ctx, `INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,original_cents,buyer_id,purchase_id,expires_at,status) VALUES('ban-subscription-trade',$1,'ban-subscription-order','subscription','internal','monthly',1200,$2,$3,$4,'pending') RETURNING id`, billingID, merchantID, purchaseID, expires).Scan(&subscriptionOrderID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE purchases SET billing_merchant_id=$1,payment_trade_no='ban-subscription-trade' WHERE id=$2`, billingID, purchaseID); err != nil {
		t.Fatal(err)
	}
	var alreadyPaidCallbackOrderID int64
	if err := db.Pool.QueryRow(ctx, `INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,original_cents,expires_at,status,notify_url) VALUES('ban-paid-trade',$1,'ban-paid-order','external','v1','already paid',1200,$2,'pending','https://merchant.example.test/notify') RETURNING id`, merchantID, expires).Scan(&alreadyPaidCallbackOrderID); err != nil {
		t.Fatal("seed already-paid callback order")
	}
	application := db.NewApplication(t)
	paidTx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal("begin pre-ban payment transaction")
	}
	if err := application.Services.Payment.MarkPaidTx(ctx, paidTx, alreadyPaidCallbackOrderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
		_ = paidTx.Rollback(ctx)
		t.Fatalf("pay callback order before merchant ban: %v", err)
	}
	if err := paidTx.Commit(ctx); err != nil {
		t.Fatalf("commit callback payment before ban: %v", err)
	}
	result, err := application.Services.Merchant.Ban(ctx, platform.Actor{Role: "admin", SubjectID: 1}, merchantID, "policy violation")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("V45.B.r02/v01", func(t *testing.T) {
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		_, err := application.Services.Payment.Create(ctx, payment.CreateCommand{
			MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "ban-new-order-after-commit",
			Name: "banned new order", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.121", Actor: actor,
		})
		if !hasIdentityError(err, "ACCOUNT_BANNED") {
			t.Fatalf("new order after ban commit error=%v, want ACCOUNT_BANNED", err)
		}
		var created int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no='ban-new-order-after-commit'`, merchantID).Scan(&created); err != nil || created != 0 {
			t.Fatalf("banned create wrote %d orders err=%v", created, err)
		}
	})
	if result.ExpiredOrders != 1 {
		t.Fatalf("ban expired %d orders; want 1", result.ExpiredOrders)
	}
	var orderState, purchaseState, expireReason, subscriptionOrderState, subscriptionExpireReason string
	if err := db.Pool.QueryRow(ctx, `SELECT status,expire_reason FROM orders WHERE id=$1`, orderID).Scan(&orderState, &expireReason); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT state FROM purchases WHERE id=$1`, purchaseID).Scan(&purchaseState); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT status,expire_reason FROM orders WHERE id=$1`, subscriptionOrderID).Scan(&subscriptionOrderState, &subscriptionExpireReason); err != nil {
		t.Fatal(err)
	}
	t.Run("V13.B.r04/v01", func(t *testing.T) {
		if orderState != "expired" || expireReason != "banned" || purchaseState != "cancelled" || subscriptionOrderState != "expired" || subscriptionExpireReason != "cancelled" {
			t.Fatalf("ban result order=%s reason=%s purchase=%s subscription_order=%s subscription_reason=%s", orderState, expireReason, purchaseState, subscriptionOrderState, subscriptionExpireReason)
		}
	})
	t.Run("V13.B.r04/v02", func(t *testing.T) {
		var paidStatus string
		var callbackCount int
		var callbackState string
		if err := db.Pool.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1`, alreadyPaidCallbackOrderID).Scan(&paidStatus); err != nil {
			t.Fatal("read already-paid order after ban")
		}
		if err := db.Pool.QueryRow(ctx, `SELECT count(*),min(state) FROM callback_events WHERE order_id=$1`, alreadyPaidCallbackOrderID).Scan(&callbackCount, &callbackState); err != nil {
			t.Fatal("read callback queue after ban")
		}
		if paidStatus != "paid" || callbackCount != 1 || callbackState != "pending" {
			t.Fatalf("ban changed an already-paid callback: status=%s callback_events=%d state=%s", paidStatus, callbackCount, callbackState)
		}
	})
	t.Run("V13.B.r06/v01", func(t *testing.T) {
		db.Clock.Set(db.Clock.Now().AddDate(10, 0, 0))
		var bannedAt time.Time
		if err := db.Pool.QueryRow(ctx, `SELECT banned_at FROM merchants WHERE id=$1`, merchantID).Scan(&bannedAt); err != nil || bannedAt.IsZero() {
			t.Fatalf("ban state expired without an explicit unban: banned_at=%v err=%v", bannedAt, err)
		}
	})
	t.Run("V13.B.r05/v01", func(t *testing.T) {
		if _, err := application.Services.Merchant.Unban(ctx, platform.Actor{Role: "admin", SubjectID: 1}, merchantID); err != nil {
			t.Fatalf("unban merchant: %v", err)
		}
		var status, reason string
		if err := db.Pool.QueryRow(ctx, `SELECT status,expire_reason FROM orders WHERE id=$1`, orderID).Scan(&status, &reason); err != nil {
			t.Fatal("read expired order after unban")
		}
		if status != "expired" || reason != "banned" {
			t.Fatalf("unban revived pre-ban order: status=%s reason=%s", status, reason)
		}
	})
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = application.Services.Payment.MarkPaidTx(ctx, tx, orderID, payment.PaidCommand{Channel: "alipay"})
	_ = tx.Rollback(ctx)
	t.Run("V45.B.r02/v02", func(t *testing.T) {
		if err == nil {
			t.Fatalf("late payment for pre-ban pending order was not rejected: %v", err)
		}
		var status string
		if err := db.Pool.QueryRow(ctx, `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&status); err != nil || status != "expired" {
			t.Fatalf("late payment changed banned pending order status=%q err=%v", status, err)
		}
	})
}
