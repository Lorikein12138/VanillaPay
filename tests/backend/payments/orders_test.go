package payments

import (
	"context"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

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
