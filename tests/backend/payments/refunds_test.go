package payments

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_RefundRequestAndManualResolutionAreFullAmountAndCancelCallbacks(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	orderID := seedPendingExternalOrder(t, db, merchantID, "refund-trade", "refund-out", "https://merchant.example/notify")
	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := application.Services.Payment.MarkPaidTx(context.Background(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	digest := sha256.Sum256([]byte("refund content"))
	request, err := application.Services.Payment.RequestRefund(context.Background(), payment.RefundCommand{MerchantID: merchantID, TradeNo: "refund-trade", OutRefundNo: "refund-1", AmountCents: 100, Reason: "customer request", RequestDigest: digest[:], Actor: actor})
	if err != nil || request.Status != "pending" {
		t.Fatalf("refund request = %#v, %v", request, err)
	}
	t.Run("V37.B.r01/v02", func(t *testing.T) {
		var orderStatus string
		var notifications, mailJobs int
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&orderStatus); err != nil {
			t.Fatal("read order while refund request is pending")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM notifications WHERE merchant_id=$1 AND kind='refund_request'`, merchantID).Scan(&notifications); err != nil {
			t.Fatal("count refund request notification")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_deliveries WHERE dedupe_key LIKE 'refund:%'`).Scan(&mailJobs); err != nil {
			t.Fatal("count refund request mail jobs")
		}
		if orderStatus != "paid" || notifications != 1 || mailJobs != 1 {
			t.Fatalf("pending refund side effects order=%s notifications=%d mail_jobs=%d", orderStatus, notifications, mailJobs)
		}
	})
	t.Run("V37.B.r01/v01", func(t *testing.T) {
		if request.Status != "pending" || request.AmountCents != 100 {
			t.Fatalf("full refund request response=%#v, want committed pending request for 100 cents", request)
		}
	})
	t.Run("V37.B.r02/v01", func(t *testing.T) {
		repeated, err := application.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: merchantID, TradeNo: "refund-trade", OutRefundNo: "refund-1", AmountCents: 100, Reason: "customer request", RequestDigest: digest[:], Actor: actor})
		if err != nil || repeated.ID != request.ID || repeated.Status != "pending" {
			t.Fatalf("identical pending refund request=%#v err=%v, want same durable request", repeated, err)
		}
		var notifications int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM notifications WHERE merchant_id=$1 AND kind='refund_request'`, merchantID).Scan(&notifications); err != nil || notifications != 1 {
			t.Fatalf("idempotent refund request created %d notifications err=%v", notifications, err)
		}
	})
	t.Run("V37.B.r02/v02", func(t *testing.T) {
		partialDigest := sha256.Sum256([]byte("partial refund request"))
		if _, err := application.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: merchantID, TradeNo: "refund-trade", OutRefundNo: "refund-partial", AmountCents: 99, Reason: "partial", RequestDigest: partialDigest[:], Actor: actor}); errorCode(err) != "PARTIAL_REFUND_UNSUPPORTED" {
			t.Fatalf("partial refund error=%v, want PARTIAL_REFUND_UNSUPPORTED", err)
		}
	})
	t.Run("V37.B.r02/v03", func(t *testing.T) {
		otherID, _ := db.SeedMerchant(t)
		otherActor := platform.Actor{Role: "merchant", SubjectID: otherID, MerchantID: otherID}
		otherDigest := sha256.Sum256([]byte("cross merchant refund"))
		if _, err := application.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: otherID, TradeNo: "refund-trade", OutRefundNo: "refund-foreign", AmountCents: 100, Reason: "foreign", RequestDigest: otherDigest[:], Actor: otherActor}); errorCode(err) != "NOT_FOUND" {
			t.Fatalf("cross-merchant refund request error=%v, want NOT_FOUND", err)
		}
	})
	t.Run("V37.B.r02/v04", func(t *testing.T) {
		changedDigest := sha256.Sum256([]byte("different content"))
		if _, err := application.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: merchantID, TradeNo: "refund-trade", OutRefundNo: "refund-1", AmountCents: 100, Reason: "changed content", RequestDigest: changedDigest[:], Actor: actor}); errorCode(err) != "REFUND_PENDING" {
			t.Fatalf("same refund number with changed pending content error=%v, want a 409 pending conflict", err)
		}
	})
	if err := application.Services.Payment.ResolveRefund(context.Background(), actor, request.ID, payment.RefundResolution{Action: "mark_refunded", Reason: "settlement confirmed", Confirm: true}); err != nil {
		t.Fatal(err)
	}
	var status, eventState string
	if err := db.Pool.QueryRow(context.Background(), `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT state FROM callback_events WHERE order_id=$1 ORDER BY generation DESC LIMIT 1`, orderID).Scan(&eventState); err != nil {
		t.Fatal(err)
	}
	if status != "refunded" || eventState != "cancelled" {
		t.Fatalf("refund result order=%s callback=%s", status, eventState)
	}
	t.Run("V37.B.r03/v02", func(t *testing.T) {
		var state string
		var refundedAt *time.Time
		if err := db.Pool.QueryRow(t.Context(), `SELECT status,refunded_at FROM orders WHERE id=$1`, orderID).Scan(&state, &refundedAt); err != nil || state != "refunded" || refundedAt == nil || refundedAt.IsZero() {
			t.Fatalf("completed refund status=%q refunded_at=%v err=%v", state, refundedAt, err)
		}
	})
	t.Run("V37.B.r03/v03", func(t *testing.T) {
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE id=$1 AND refunded_at IS NOT NULL`, orderID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("refund timestamp persisted rows=%d err=%v", count, err)
		}
	})
	t.Run("V37.B.r03/v04", func(t *testing.T) {
		if eventState != "cancelled" {
			t.Fatalf("uncompleted callback was not cancelled on full refund: %s", eventState)
		}
	})
}

func TestDB_RefundRejectedCanBeResubmitted(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	tradeNo := "refund-rejected-resubmit"
	orderID := seedPendingExternalOrder(t, db, merchantID, tradeNo, "refund-rejected-out", "https://merchant.example.test/notify")
	tx, err := db.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Services.Payment.MarkPaidTx(t.Context(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("pay order for rejected refund: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	digest1 := sha256.Sum256([]byte("first refund request"))
	first, err := app.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: merchantID, TradeNo: tradeNo, OutRefundNo: "refund-rejected-1", AmountCents: 100, Reason: "first attempt", RequestDigest: digest1[:], Actor: actor})
	if err != nil {
		t.Fatalf("create initial refund request: %v", err)
	}
	if err := app.Services.Payment.ResolveRefund(t.Context(), actor, first.ID, payment.RefundResolution{Action: "reject", Reason: "insufficient evidence"}); err != nil {
		t.Fatalf("reject initial refund request: %v", err)
	}
	digest2 := sha256.Sum256([]byte("second refund request"))
	second, err := app.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: merchantID, TradeNo: tradeNo, OutRefundNo: "refund-rejected-2", AmountCents: 100, Reason: "resubmitted", RequestDigest: digest2[:], Actor: actor})
	if err != nil || second.ID == first.ID || second.Status != "pending" {
		t.Fatalf("resubmitted refund = %#v, %v", second, err)
	}
	t.Run("V37.B.r03/v01", func(t *testing.T) {
		var orderStatus, firstState, secondState string
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&orderStatus); err != nil {
			t.Fatal("read order after rejected refund and resubmission")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM refunds WHERE public_id=$1`, first.ID).Scan(&firstState); err != nil {
			t.Fatal("read rejected refund")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM refunds WHERE public_id=$1`, second.ID).Scan(&secondState); err != nil {
			t.Fatal("read resubmitted refund")
		}
		if orderStatus != "paid" || firstState != "rejected" || secondState != "pending" {
			t.Fatalf("refund resubmission states order=%s first=%s second=%s", orderStatus, firstState, secondState)
		}
	})
}

func TestHTTP_AdminMutationRoutesAbsent(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	code, err := fixture.App.Services.Auth.RotateBootstrapCode(t.Context())
	if err != nil {
		t.Fatal("prepare one-use administrator bootstrap code")
	}
	bootstrap := fixture.Request(t, "POST", "/api/v1/console/bootstrap", map[string]any{
		"initialization_code": code, "username": "refund_admin", "password": fixturePassword, "password_confirmation": fixturePassword,
	}, "admin:refund-admin")
	if bootstrap.Code != 201 {
		t.Fatalf("bootstrap admin session status=%d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	merchantID, _ := db.SeedMerchant(t)
	orderID := seedPendingExternalOrder(t, db, merchantID, "admin-readonly-trade", "admin-readonly-out", "https://merchant.example.test/notify")
	paths := []string{
		"/api/v1/console/orders/admin-readonly-trade/supplement",
		"/api/v1/console/orders/admin-readonly-trade/refund",
		"/api/v1/console/orders/admin-readonly-trade/callback-replay",
	}
	for i, path := range paths {
		method := "POST"
		response := fixture.Request(t, method, path, map[string]any{}, "admin:refund-admin")
		if response.Code != 404 && response.Code != 405 {
			t.Fatalf("admin order mutation %s %s exposed status=%d body=%s", method, path, response.Code, response.Body.String())
		}
		if i == 0 {
			t.Run("V37.B.r04/v01", func(t *testing.T) {
				for _, mutationPath := range paths {
					result := fixture.Request(t, "POST", mutationPath, map[string]any{}, "admin:refund-admin")
					if result.Code != 404 && result.Code != 405 {
						t.Fatalf("admin mutation route %s status=%d body=%s, want 404/405", mutationPath, result.Code, result.Body.String())
					}
				}
				var unchanged string
				if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id=$1`, orderID).Scan(&unchanged); err != nil || unchanged != "pending" {
					t.Fatalf("read-only administrator routes changed order state=%q err=%v", unchanged, err)
				}
			})
			break
		}
	}
}
