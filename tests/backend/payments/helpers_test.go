package payments

import (
	"context"
	"errors"
	"testing"
	"time"

	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func seedPendingExternalOrder(t *testing.T, db *testsupport.Database, merchantID int64, trade, outTrade, notifyURL string) int64 {
	t.Helper()
	var id int64
	expires := db.Clock.Now().Add(time.Hour)
	err := db.Pool.QueryRow(context.Background(), `INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,original_cents,allowed_channels_snapshot,notify_url,signing_key_version,signing_key_cipher,merchant_username_snapshot,merchant_pid_snapshot,expires_at,status) SELECT $2,m.id,$3,'external','v1','integration order',100,ARRAY['alipay']::text[],$4,m.key_version,m.key_cipher,m.username,m.pid,$5,'pending' FROM merchants m WHERE m.id=$1 RETURNING id`, merchantID, trade, outTrade, notifyURL, expires).Scan(&id)
	if err != nil {
		t.Fatal("seed pending payment order")
	}
	return id
}

func errorCode(err error) string {
	var apiErr *platform.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return ""
}
