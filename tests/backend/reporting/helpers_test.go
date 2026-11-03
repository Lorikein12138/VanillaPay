package reporting

import (
	"context"
	"testing"
	"time"

	"vanillapay/tests/backend/testsupport"
)

func insertOrder(t *testing.T, db *testsupport.Database, merchantID int64, trade, name, status, channel string, createdAt time.Time, paidAt, refundedAt *time.Time, cents int64) int64 {
	t.Helper()
	var id int64
	var actual any
	if channel != "" {
		actual = cents
	}
	err := db.Pool.QueryRow(context.Background(), `INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,original_cents,actual_cents,channel,allowed_channels_snapshot,expires_at,status,paid_at,refunded_at,created_at,merchant_username_snapshot) VALUES($1,$2,$3,'external','v1',$4,$5,$6,NULLIF($7,'')::text,CASE WHEN $7='' THEN '{}'::text[] ELSE ARRAY[$7]::text[] END,$8,$9,$10,$11,$12,'reporting fixture') RETURNING id`, trade, merchantID, "out-"+trade, name, cents, actual, channel, db.Clock.Now().Add(24*time.Hour), status, paidAt, refundedAt, createdAt).Scan(&id)
	if err != nil {
		t.Fatalf("insert reporting order %s: %v", trade, err)
	}
	return id
}

func datePtr(value time.Time) *time.Time { return &value }
