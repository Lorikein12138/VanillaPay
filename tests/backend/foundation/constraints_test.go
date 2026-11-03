package foundation

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_FoundationConstraintsAndRiverOwnership(t *testing.T) {
	db := testsupport.NewDatabase(t)
	ctx := context.Background()

	var riverTable bool
	if err := db.Pool.QueryRow(ctx, `SELECT to_regclass('river_job') IS NOT NULL`).Scan(&riverTable); err != nil || !riverTable {
		t.Fatalf("River's PostgreSQL migration is not owned by the application database: present=%t err=%v", riverTable, err)
	}

	merchantA, _ := db.SeedMerchant(t)
	merchantB, _ := db.SeedMerchant(t)
	if _, err := db.Pool.Exec(ctx, `UPDATE merchants SET username='constraint_owner',username_normalized='constraint_owner',email='constraint-owner@example.test',email_normalized='constraint-owner@example.test',pid='1000000000000001' WHERE id=$1`, merchantA); err != nil {
		t.Fatal("prepare deterministic merchant uniqueness values")
	}
	var username, email, pid string
	if err := db.Pool.QueryRow(ctx, `SELECT username_normalized,email_normalized,pid FROM merchants WHERE id=$1`, merchantA).Scan(&username, &email, &pid); err != nil {
		t.Fatal("read merchant uniqueness fixture")
	}
	merchantUniqueBefore := func(t *testing.T) (string, string, string, int64) {
		t.Helper()
		var beforeUsername, beforeEmail, beforePID string
		var count int64
		if err := db.Pool.QueryRow(ctx, `SELECT username_normalized,email_normalized,pid FROM merchants WHERE id=$1`, merchantB).Scan(&beforeUsername, &beforeEmail, &beforePID); err != nil {
			t.Fatal("read second merchant before uniqueness check")
		}
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM merchants`).Scan(&count); err != nil {
			t.Fatal("count merchants before uniqueness check")
		}
		return beforeUsername, beforeEmail, beforePID, count
	}
	assertMerchantUniqueFailure := func(t *testing.T, id, statement, value, wantSQLState string) {
		t.Helper()
		beforeUsername, beforeEmail, beforePID, beforeCount := merchantUniqueBefore(t)
		_, err := db.Pool.Exec(ctx, statement, value, merchantB)
		assertConstraintSQLState(t, err, wantSQLState)
		var afterUsername, afterEmail, afterPID string
		var afterCount int64
		if err := db.Pool.QueryRow(ctx, `SELECT username_normalized,email_normalized,pid FROM merchants WHERE id=$1`, merchantB).Scan(&afterUsername, &afterEmail, &afterPID); err != nil {
			t.Fatalf("%s read second merchant after failed update: %v", id, err)
		}
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM merchants`).Scan(&afterCount); err != nil {
			t.Fatalf("%s count merchants after failed update: %v", id, err)
		}
		if afterUsername != beforeUsername || afterEmail != beforeEmail || afterPID != beforePID || afterCount != beforeCount {
			t.Fatalf("%s changed data after constraint rejection: before=(%q,%q,%q,%d) after=(%q,%q,%q,%d)", id, beforeUsername, beforeEmail, beforePID, beforeCount, afterUsername, afterEmail, afterPID, afterCount)
		}
	}
	t.Run("V03.B.r03/v01", func(t *testing.T) {
		assertMerchantUniqueFailure(t, "merchant_pid_unique", `UPDATE merchants SET pid=$1 WHERE id=$2`, pid, "23505")
	})
	t.Run("V03.B.r03/v05", func(t *testing.T) {
		assertMerchantUniqueFailure(t, "merchant_username_unique", `UPDATE merchants SET username_normalized=$1 WHERE id=$2`, username, "23505")
	})
	t.Run("V03.B.r03/v06", func(t *testing.T) {
		assertMerchantUniqueFailure(t, "merchant_email_unique", `UPDATE merchants SET email_normalized=$1 WHERE id=$2`, email, "23505")
	})

	insertOrder := func(trade, outTrade string, cents int64) error {
		_, err := db.Pool.Exec(ctx, `INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,original_cents,expires_at,status) VALUES($1,$2,$3,'external','v1','constraint fixture',$4,$5,'pending')`, trade, merchantA, outTrade, cents, db.Clock.Now().Add(time.Hour))
		return err
	}
	if err := insertOrder("constraint-order-valid", "constraint-out-valid", 100); err != nil {
		t.Fatalf("valid pending order fixture insert failed: %v", err)
	}
	t.Run("duplicate_out_trade_no", func(t *testing.T) {
		if err := insertOrder("constraint-order-duplicate", "constraint-out-valid", 100); err == nil {
			t.Error("duplicate merchant/out_trade_no was accepted")
		}
	})
	t.Run("V03.B.r03/v02", func(t *testing.T) {
		assertConstraintSQLState(t, insertOrder("constraint-order-negative", "constraint-out-negative", -1), "23514")
		var count int64
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM orders WHERE merchant_id=$1`, merchantA).Scan(&count); err != nil || count != 1 {
			t.Fatalf("merchant order count=%d err=%v, want 1 after invalid amount rejection", count, err)
		}
	})

	insertDevice := func(publicID, token string) error {
		hash := sha256.Sum256([]byte(token))
		_, err := db.Pool.Exec(ctx, `INSERT INTO devices(public_id,merchant_id,token_hash,name,last_seen_at,ever_online) VALUES($1,$2,$3,'constraint fixture',$4,true)`, publicID, merchantA, hash[:], db.Clock.Now())
		return err
	}
	if err := insertDevice("constraint-device-a", "constraint-token-a"); err != nil {
		t.Fatalf("valid active device fixture insert failed: %v", err)
	}
	t.Run("V03.B.r03/v03", func(t *testing.T) {
		assertConstraintSQLState(t, insertDevice("constraint-device-b", "constraint-token-b"), "23505")
		var activeDevices int64
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL`, merchantA).Scan(&activeDevices); err != nil || activeDevices != 1 {
			t.Fatalf("active device count=%d err=%v, want 1 after rejected insert", activeDevices, err)
		}
	})
	var deviceID int64
	if err := db.Pool.QueryRow(ctx, `SELECT id FROM devices WHERE public_id='constraint-device-a'`).Scan(&deviceID); err != nil {
		t.Fatal("read active device fixture")
	}

	insertReceipt := func(publicID, eventID string, orderID *int64) error {
		_, err := db.Pool.Exec(ctx, `INSERT INTO receipts(public_id,merchant_id,device_id,event_id,channel,amount_cents,occurred_at,match_state,unmatched_reason,order_id) VALUES($1,$2,$3,$4,'alipay',100,$5,$6,$7,$8)`, publicID, merchantA, deviceID, eventID, time.Now(), map[bool]string{true: "matched", false: "unmatched"}[orderID != nil], nullableUnmatchedReason(orderID), orderID)
		return err
	}
	eventID := "8a1cfcdd-84d8-41ec-9781-692e0d500001"
	if err := insertReceipt("constraint-receipt-valid", eventID, nil); err != nil {
		t.Fatalf("valid unmatched receipt fixture insert failed: %v", err)
	}
	t.Run("V03.B.r03/v04", func(t *testing.T) {
		assertConstraintSQLState(t, insertReceipt("constraint-receipt-duplicate", eventID, nil), "23505")
		var receiptCount int64
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM receipts WHERE merchant_id=$1 AND event_id=$2`, merchantA, eventID).Scan(&receiptCount); err != nil || receiptCount != 1 {
			t.Fatalf("receipt count=%d err=%v, want 1 after duplicate event rejection", receiptCount, err)
		}
	})
	t.Run("V03.B.r03/v07", func(t *testing.T) {
		missingOrder := int64(9223372036854775807)
		assertConstraintSQLState(t, insertReceipt("constraint-receipt-missing-order", "8a1cfcdd-84d8-41ec-9781-692e0d500002", &missingOrder), "23503")
		var receiptCount int64
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM receipts WHERE merchant_id=$1`, merchantA).Scan(&receiptCount); err != nil || receiptCount != 1 {
			t.Fatalf("merchant receipt count=%d err=%v, want 1 after FK rejection", receiptCount, err)
		}
	})
}

func assertConstraintSQLState(t *testing.T, err error, want string) string {
	t.Helper()
	if err == nil {
		t.Fatalf("PostgreSQL accepted input rejected by SQLSTATE %s", want)
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		t.Fatalf("constraint error %T does not expose PostgreSQL SQLSTATE: %v", err, err)
	}
	if postgresError.Code != want {
		t.Fatalf("PostgreSQL SQLSTATE=%s, want %s: %v", postgresError.Code, want, err)
	}
	return postgresError.Code
}

func nullableUnmatchedReason(orderID *int64) *string {
	if orderID != nil {
		return nil
	}
	reason := "no_match"
	return &reason
}
