package payments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"vanillapay/internal/auth"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_SupplementExpectedAmount(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	plan := db.SeedPlan(t, "supplement amounts", 1000, 3650, []string{"alipay", "wxpay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, plan)
	const password = "Local-supplement-test-123!"
	hash, err := auth.HashPassword(password, nil)
	if err != nil {
		t.Fatal(err)
	}
	var email string
	if err := db.Pool.QueryRow(t.Context(), `UPDATE merchants SET password_hash=$1 WHERE id=$2 RETURNING email_normalized`, hash, merchantID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if response := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", auth.LoginRequest{Identifier: email, Password: password}, "merchant"); response.Code != http.StatusOK {
		t.Fatalf("fixture login: status=%d", response.Code)
	}

	submit := func(t *testing.T, trade, key, body string, status int, code string) {
		t.Helper()
		headers := http.Header{"Content-Type": {"application/json"}, "Idempotency-Key": {key}}
		response := fixture.RequestWithHeaders(t, http.MethodPost, "/api/v1/merchant/orders/"+trade+"/supplement", strings.NewReader(body), "merchant", headers)
		var result struct {
			Error *platform.Error `json:"error"`
		}
		err := json.Unmarshal(response.Body.Bytes(), &result)
		actualCode := ""
		if result.Error != nil {
			actualCode = result.Error.Code
		}
		if err != nil || response.Code != status || actualCode != code {
			t.Fatalf("supplement response: status=%d error=%q decode=%v", response.Code, actualCode, err)
		}
	}

	createReservationOrder(t, db, fixture.App, merchantID, "occupied-supplement-amount", 100, "alipay", nil)
	order := createReservationOrder(t, db, fixture.App, merchantID, "floating-supplement-amount", 100, "", nil)
	if order.ActualCents != nil {
		t.Fatal("unselected order already has an actual amount")
	}
	locked, err := fixture.App.Services.Payment.SelectChannel(t.Context(), order.TradeNo, "alipay")
	if err != nil || locked.ActualCents == nil || *locked.ActualCents == order.OriginalCents {
		t.Fatalf("select channel with floating amount: %+v, %v", locked.ActualCents, err)
	}

	t.Run("validate supplied amount", func(t *testing.T) {
		for _, value := range []string{"0", "-1", "1000000000", "1.5", `"100"`} {
			body := `{"reason":"verified","channel":"alipay","expected_amount_cents":` + value + `}`
			submit(t, order.TradeNo, "supplement-validation", body, http.StatusBadRequest, "VALIDATION_ERROR")
		}
	})
	t.Run("reject changed amount and replay confirmed request", func(t *testing.T) {
		const key = "supplement-confirmed-amount"
		body := func(amount int64) string {
			return fmt.Sprintf(`{"reason":"verified","channel":"alipay","expected_amount_cents":%d}`, amount)
		}
		submit(t, order.TradeNo, key, body(order.OriginalCents), http.StatusConflict, "ORDER_AMOUNT_CHANGED")
		var status string
		var manual bool
		var events int
		if err := db.Pool.QueryRow(t.Context(), `
			SELECT status,is_manual,(SELECT count(*) FROM callback_events WHERE order_id=orders.id)
			FROM orders WHERE trade_no=$1
		`, order.TradeNo).Scan(&status, &manual, &events); err != nil || status != "pending" || manual || events != 0 {
			t.Fatalf("rejected supplement: status=%s manual=%t events=%d err=%v", status, manual, events, err)
		}
		submit(t, order.TradeNo, key, body(*locked.ActualCents), http.StatusOK, "")
		submit(t, order.TradeNo, key, body(*locked.ActualCents), http.StatusOK, "")
		submit(t, order.TradeNo, key, body(order.OriginalCents), http.StatusConflict, "IDEMPOTENCY_CONFLICT")
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM callback_events JOIN orders ON orders.id=callback_events.order_id WHERE trade_no=$1`, order.TradeNo).Scan(&events); err != nil || events != 1 {
			t.Fatalf("confirmed supplement callback count=%d err=%v", events, err)
		}
	})
	t.Run("preserve legacy digest and null replay", func(t *testing.T) {
		const key = "supplement-legacy-amount"
		legacy := createReservationOrder(t, db, fixture.App, merchantID, "legacy-supplement-amount", 100, "", nil)
		submit(t, legacy.TradeNo, key, `{"reason":"verified","channel":"alipay"}`, http.StatusOK, "")
		want := sha256.Sum256([]byte(legacy.TradeNo + "\x00verified\x00alipay"))
		var digest []byte
		if err := db.Pool.QueryRow(t.Context(), `SELECT request_digest FROM idempotency_records WHERE route='merchant.orders.supplement' AND key=$1`, key).Scan(&digest); err != nil || !bytes.Equal(digest, want[:]) {
			t.Fatalf("legacy digest changed: %v", err)
		}
		submit(t, legacy.TradeNo, key, `{"reason":"verified","channel":"alipay","expected_amount_cents":null}`, http.StatusOK, "")
	})
}

func TestDB_ManualSupplementMarksPaidAndPersistsCallbackInOneTransaction(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	orderID := seedPendingExternalOrder(t, db, merchantID, "supplement-trade", "supplement-out", "https://merchant.example/notify")
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	request := payment.SupplementRequest{Reason: "verified offline receipt", Channel: "alipay"}
	if err := application.Services.Payment.SupplementIdempotent(context.Background(), actor, "supplement-trade", request, "supplement-request-001"); err != nil {
		t.Fatal(err)
	}
	if err := application.Services.Payment.SupplementIdempotent(context.Background(), actor, "supplement-trade", request, "supplement-request-001"); err != nil {
		t.Fatalf("idempotent supplement replay: %v", err)
	}
	var status string
	var manual bool
	var reason string
	if err := db.Pool.QueryRow(context.Background(), `SELECT status,is_manual,manual_reason FROM orders WHERE id=$1`, orderID).Scan(&status, &manual, &reason); err != nil {
		t.Fatal(err)
	}
	if status != "paid" || !manual || reason != request.Reason {
		t.Fatalf("supplemented order status=%s manual=%t reason=%q", status, manual, reason)
	}
	var events int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM callback_events WHERE order_id=$1 AND generation=1 AND state='pending'`, orderID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("manual supplement created %d callback events", events)
	}
}
