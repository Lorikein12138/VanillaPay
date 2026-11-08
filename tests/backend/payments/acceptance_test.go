package payments

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_PaymentRemainingAcceptance(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}

	t.Run("V33.B.r01/v01", func(t *testing.T) {
		orderID := seedPendingExternalOrder(t, db, merchantID, "acceptance-supplement-pending", "acceptance-supplement-pending-out", "https://merchant.example.test/notify")
		request := payment.SupplementRequest{Reason: "verified offline settlement", Channel: "alipay"}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "acceptance-supplement-pending", request, "acceptance-supplement-key-01"); err != nil {
			t.Fatalf("supplement pending order: %v", err)
		}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "acceptance-supplement-pending", request, "acceptance-supplement-key-01"); err != nil {
			t.Fatalf("replay identical supplement: %v", err)
		}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "acceptance-supplement-pending", payment.SupplementRequest{Reason: "different settlement", Channel: "alipay"}, "acceptance-supplement-key-01"); errorCode(err) != "IDEMPOTENCY_CONFLICT" {
			t.Fatalf("same idempotency key with different content error=%v, want IDEMPOTENCY_CONFLICT", err)
		}
		var status string
		var manual bool
		var reason string
		if err := db.Pool.QueryRow(t.Context(), `SELECT status,is_manual,manual_reason FROM orders WHERE id=$1`, orderID).Scan(&status, &manual, &reason); err != nil {
			t.Fatal("read manually supplemented order")
		}
		if status != "paid" || !manual || reason != request.Reason {
			t.Fatalf("manual supplement state status=%s manual=%t reason=%q", status, manual, reason)
		}
		var count int
		var role string
		var auditMerchant int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*),min(actor_role),min(merchant_id) FROM audit_logs WHERE action='order.paid' AND target_id='acceptance-supplement-pending'`).Scan(&count, &role, &auditMerchant); err != nil {
			t.Fatal("read manually paid audit")
		}
		if count != 1 || role != "merchant" || auditMerchant != merchantID {
			t.Fatalf("manual paid audit count=%d role=%q merchant=%d", count, role, auditMerchant)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM callback_events WHERE order_id=$1 AND state='pending'`, orderID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("manual paid callback count=%d err=%v", count, err)
		}
		t.Run("V33.B.r01/v02", func(t *testing.T) {
			var actorID int64
			if err := db.Pool.QueryRow(t.Context(), `SELECT manual_actor_id FROM orders WHERE id=$1`, orderID).Scan(&actorID); err != nil || actorID != merchantID || !manual || reason != request.Reason {
				t.Fatalf("manual order actor=%d manual=%t reason=%q err=%v", actorID, manual, reason, err)
			}
			if count, role, auditMerchant := readPaidAudit(t, db, "acceptance-supplement-pending"); count != 1 || role != "merchant" || auditMerchant != merchantID {
				t.Fatalf("manual order audit count=%d role=%q merchant=%d", count, role, auditMerchant)
			}
		})
		t.Run("V33.B.r01/v03", func(t *testing.T) {
			if count != 1 {
				t.Fatalf("manual supplement queued %d callback generations, want one", count)
			}
		})
		t.Run("V33.B.r04/v01", func(t *testing.T) {
			detail, err := app.Services.Payment.Query(t.Context(), actor, payment.OrderSelector{TradeNo: "acceptance-supplement-pending", MerchantID: merchantID})
			if err != nil || detail.Status != "paid" || detail.ActualCents == nil || *detail.ActualCents != 100 || detail.Channel == nil || *detail.Channel != "alipay" || detail.CallbackStatus != "pending" {
				t.Fatalf("supplemented order detail=%#v err=%v", detail, err)
			}
		})
	})

	t.Run("V33.B.r02/v01", func(t *testing.T) {
		orderID := seedPendingExternalOrder(t, db, merchantID, "acceptance-supplement-unselected", "acceptance-supplement-unselected-out", "https://merchant.example.test/notify")
		if _, err := db.Pool.Exec(t.Context(), `UPDATE orders SET status='expired',expire_reason='timeout' WHERE id=$1`, orderID); err != nil {
			t.Fatal("expire unselected fixture order")
		}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "acceptance-supplement-unselected", payment.SupplementRequest{Reason: "verified but channel omitted"}, "acceptance-supplement-unselected-key"); errorCode(err) != "CHANNEL_NOT_ALLOWED" {
			t.Fatalf("unselected order without explicit channel error=%v, want CHANNEL_NOT_ALLOWED", err)
		}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "acceptance-supplement-unselected", payment.SupplementRequest{Reason: "verified expired order", Channel: "alipay"}, "acceptance-supplement-expired-key"); err != nil {
			t.Fatalf("supplement expired order after explicit channel: %v", err)
		}
		var status, channel string
		if err := db.Pool.QueryRow(t.Context(), `SELECT status,channel FROM orders WHERE id=$1`, orderID).Scan(&status, &channel); err != nil || status != "paid" || channel != "alipay" {
			t.Fatalf("expired order supplement state status=%s channel=%s err=%v", status, channel, err)
		}
	})

	t.Run("V33.B.r03/v01", func(t *testing.T) {
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "does-not-exist", payment.SupplementRequest{Reason: "missing"}, "acceptance-missing-order-key"); errorCode(err) != "NOT_FOUND" {
			t.Fatalf("missing order supplement error=%v, want NOT_FOUND", err)
		}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), platform.Actor{Role: "merchant", SubjectID: merchantID + 1, MerchantID: merchantID + 1}, "acceptance-supplement-pending", payment.SupplementRequest{Reason: "foreign"}, "acceptance-foreign-order-key"); errorCode(err) != "NOT_FOUND" {
			t.Fatalf("cross-merchant supplement error=%v, want NOT_FOUND", err)
		}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "acceptance-supplement-pending", payment.SupplementRequest{Reason: "already paid"}, "acceptance-paid-order-key"); errorCode(err) != "ORDER_NOT_PAYABLE" {
			t.Fatalf("already paid supplement error=%v, want ORDER_NOT_PAYABLE", err)
		}
	})
	t.Run("V33.B.r03/v03", func(t *testing.T) {
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, "acceptance-supplement-pending", payment.SupplementRequest{Reason: "second attempt", Channel: "alipay"}, "acceptance-paid-order-key-02"); errorCode(err) != "ORDER_NOT_PAYABLE" {
			t.Fatalf("paid order repeated supplement error=%v, want ORDER_NOT_PAYABLE", err)
		}
	})

}

func TestDB_PaymentAmountTTLAndOrderSnapshots(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "amount acceptance plan", 1000, 3650, []string{"alipay"}, nil)
	application := db.NewApplication(t)
	db.PrepareBillingMerchant(t, application, merchantID, merchantKey, planID)
	var pid string
	if err := db.Pool.QueryRow(t.Context(), `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&pid); err != nil {
		t.Fatal("read protocol merchant PID")
	}
	router := paymentProtocolRouter(t, application.Services.Payment)
	submit := func(outTradeNo, money, productID, name string, ttl string) *httptest.ResponseRecorder {
		t.Helper()
		values := url.Values{
			"pid": {pid}, "out_trade_no": {outTradeNo}, "notify_url": {"https://merchant.example/notify"},
			"return_url": {"https://merchant.example/return?campaign=round5"}, "name": {name}, "money": {money},
			"product_id": {productID}, "type": {"alipay"}, "clientip": {"198.51.100.15"},
		}
		if ttl != "" {
			values.Set("expire_seconds", ttl)
		}
		values.Set("sign", mustV1ContractSignature(values, merchantKey))
		request := httptest.NewRequest(http.MethodGet, "/submit.php?"+values.Encode(), nil)
		request.RemoteAddr = "192.0.2.15:31000"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	runInvalidAmount := func(t *testing.T, suffix, money string) {
		t.Helper()
		out := "invalid-amount-" + suffix
		response := submit(out, money, "", "invalid amount", "")
		var payload struct {
			Code  int    `json:"code"`
			Error string `json:"error_code"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &payload) != nil || payload.Code != -1 || payload.Error != "VALIDATION_ERROR" {
			t.Fatalf("invalid money %q response=%d %s", money, response.Code, response.Body.String())
		}
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no=$2`, merchantID, out).Scan(&count); err != nil || count != 0 {
			t.Fatalf("invalid money %q left %d orders err=%v", money, count, err)
		}
	}
	t.Run("V30.B.r01/v01", func(t *testing.T) { runInvalidAmount(t, "01", "0") })
	t.Run("V30.B.r01/v02", func(t *testing.T) { runInvalidAmount(t, "02", "-1") })
	t.Run("V30.B.r01/v03", func(t *testing.T) { runInvalidAmount(t, "03", "1.234") })
	t.Run("V30.B.r01/v04", func(t *testing.T) { runInvalidAmount(t, "04", "1e2") })
	t.Run("V30.B.r01/v05", func(t *testing.T) { runInvalidAmount(t, "05", "10000000.00") })
	t.Run("V30.B.r02/v01", func(t *testing.T) {
		response := submit("amount-1-23", "1.23", "product-123", "exact cents", "")
		if response.Code != http.StatusFound {
			t.Fatalf("valid V1 amount status=%d body=%s", response.Code, response.Body.String())
		}
		var cents int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT original_cents FROM orders WHERE merchant_id=$1 AND out_trade_no='amount-1-23'`, merchantID).Scan(&cents); err != nil || cents != 123 {
			t.Fatalf("1.23 stored as %d cents err=%v", cents, err)
		}
	})
	t.Run("V30.B.r03/v01", func(t *testing.T) {
		merchant := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		merchantTTL := 600
		if _, err := application.Services.Settings.SaveCollection(t.Context(), merchant, settings.SaveCollectionRequest{ExpectedVersion: 1, OrderTTLSeconds: &merchantTTL}); err != nil {
			t.Fatalf("set merchant TTL: %v", err)
		}
		call := submit("ttl-call-overrides-merchant", "1.01", "", "call TTL", "900")
		if call.Code != http.StatusFound {
			t.Fatalf("call TTL order status=%d body=%s", call.Code, call.Body.String())
		}
		var callCreated, callExpiry time.Time
		if err := db.Pool.QueryRow(t.Context(), `SELECT created_at,expires_at FROM orders WHERE merchant_id=$1 AND out_trade_no='ttl-call-overrides-merchant'`, merchantID).Scan(&callCreated, &callExpiry); err != nil || callExpiry.Sub(callCreated) != 900*time.Second {
			t.Fatalf("call TTL expiry delta=%s err=%v, want 900s", callExpiry.Sub(callCreated), err)
		}
		merchantDefault := submit("ttl-merchant-overrides-global", "1.02", "", "merchant TTL", "")
		if merchantDefault.Code != http.StatusFound {
			t.Fatalf("merchant TTL order status=%d body=%s", merchantDefault.Code, merchantDefault.Body.String())
		}
		var merchantCreated, merchantExpiry time.Time
		if err := db.Pool.QueryRow(t.Context(), `SELECT created_at,expires_at FROM orders WHERE merchant_id=$1 AND out_trade_no='ttl-merchant-overrides-global'`, merchantID).Scan(&merchantCreated, &merchantExpiry); err != nil || merchantExpiry.Sub(merchantCreated) != 600*time.Second {
			t.Fatalf("merchant TTL expiry delta=%s err=%v, want 600s", merchantExpiry.Sub(merchantCreated), err)
		}
	})
	t.Run("V30.B.r03/v02", func(t *testing.T) {
		var globalTTL int
		if err := db.Pool.QueryRow(t.Context(), `SELECT (payment->>'default_ttl_seconds')::int FROM site_settings WHERE singleton=true`).Scan(&globalTTL); err != nil || globalTTL != 300 {
			t.Fatalf("global default TTL=%d err=%v, want 300", globalTTL, err)
		}
	})
	t.Run("V30.B.r04/v01", func(t *testing.T) {
		tooLong := 1801
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		current, err := application.Services.Settings.GetCollection(t.Context(), actor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := application.Services.Settings.SaveCollection(t.Context(), actor, settings.SaveCollectionRequest{ExpectedVersion: current.Version, OrderTTLSeconds: &tooLong}); errorCode(err) != "TTL_OUT_OF_RANGE" {
			t.Fatalf("merchant setting above max error=%v, want TTL_OUT_OF_RANGE", err)
		}
		response := submit("ttl-above-max", "1.03", "", "too long", "1801")
		var payload map[string]any
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &payload) != nil || payload["error_code"] != "TTL_OUT_OF_RANGE" {
			t.Fatalf("order TTL above max response=%d %s", response.Code, response.Body.String())
		}
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no='ttl-above-max'`, merchantID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("above-max TTL left %d orders err=%v", count, err)
		}
	})
	t.Run("V30.B.r05/v01", func(t *testing.T) {
		response := submit("snapshot-product", "1.04", "product-special-1", "name snapshot", "")
		if response.Code != http.StatusFound {
			t.Fatalf("snapshot V1 order status=%d body=%s", response.Code, response.Body.String())
		}
		var storedProduct, merchantPID string
		var storedMerchant int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT product_id,merchant_id,merchant_pid_snapshot FROM orders WHERE merchant_id=$1 AND out_trade_no='snapshot-product'`, merchantID).Scan(&storedProduct, &storedMerchant, &merchantPID); err != nil || storedProduct != "product-special-1" || storedMerchant != merchantID || merchantPID != pid {
			t.Fatalf("order snapshots product=%q merchant=%d pid=%q err=%v", storedProduct, storedMerchant, merchantPID, err)
		}
	})
	t.Run("V30.B.r05/v02", func(t *testing.T) {
		longName := strings.Repeat("商", 60)
		response := submit("snapshot-long-name", "1.05", "utf8-product", longName, "")
		if response.Code != http.StatusFound {
			t.Fatalf("long name V1 order status=%d body=%s", response.Code, response.Body.String())
		}
		var stored string
		if err := db.Pool.QueryRow(t.Context(), `SELECT name FROM orders WHERE merchant_id=$1 AND out_trade_no='snapshot-long-name'`, merchantID).Scan(&stored); err != nil || len([]byte(stored)) > 127 || !strings.HasPrefix(longName, stored) || !utf8.ValidString(stored) {
			t.Fatalf("truncated UTF-8 name=%q bytes=%d err=%v", stored, len([]byte(stored)), err)
		}
	})
}

func readPaidAudit(t *testing.T, db *testsupport.Database, trade string) (int, string, int64) {
	t.Helper()
	var count int
	var role string
	var merchantID int64
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*),min(actor_role),min(merchant_id) FROM audit_logs WHERE action='order.paid' AND target_id=$1`, trade).Scan(&count, &role, &merchantID); err != nil {
		t.Fatalf("query paid audit for %s: %v", trade, err)
	}
	return count, role, merchantID
}

func TestDB_SupplementOwnershipAndBanRules(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantOne, _ := db.SeedMerchant(t)
	merchantTwo, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	one := platform.Actor{Role: "merchant", SubjectID: merchantOne, MerchantID: merchantOne}
	two := platform.Actor{Role: "merchant", SubjectID: merchantTwo, MerchantID: merchantTwo}
	trade := "supplement-owned-by-one"
	orderID := seedPendingExternalOrder(t, db, merchantOne, trade, "supplement-owned-out", "https://merchant.example/notify")
	t.Run("V33.B.r03/v02", func(t *testing.T) {
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), two, trade, payment.SupplementRequest{Reason: "cross-merchant", Channel: "alipay"}, "supplement-cross-merchant-key"); errorCode(err) != "NOT_FOUND" {
			t.Fatalf("M2 supplementing M1 order error=%v, want NOT_FOUND", err)
		}
		if _, err := db.Pool.Exec(t.Context(), `UPDATE merchants SET banned_at=$1,ban_reason='fixture' WHERE id=$2`, db.Clock.Now(), merchantOne); err != nil {
			t.Fatalf("ban M1 fixture: %v", err)
		}
		if err := app.Services.Payment.SupplementIdempotent(t.Context(), one, trade, payment.SupplementRequest{Reason: "banned merchant", Channel: "alipay"}, "supplement-banned-key"); errorCode(err) != "ACCOUNT_BANNED" {
			t.Fatalf("banned M1 supplement error=%v, want ACCOUNT_BANNED", err)
		}
		var status string
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&status); err != nil || status != "pending" {
			t.Fatalf("forbidden supplement changed order status=%s err=%v", status, err)
		}
	})
}
