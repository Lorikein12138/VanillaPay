-- name: GetOrderByTradeNo :one
SELECT id,trade_no,merchant_id,out_trade_no,purpose,protocol,name,product_id,param,original_cents,actual_cents,channel,status,expires_at,channel_locked_at,paid_at,refunded_at,is_manual,manual_reason,created_at FROM orders WHERE trade_no=$1;

-- name: GetOrderIDByTradeNo :one
SELECT id FROM orders WHERE trade_no=$1;

-- name: GetOrderSigningKeyCipher :one
SELECT signing_key_cipher FROM orders WHERE trade_no=$1;

-- name: InsertOrder :one
INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,product_id,param,original_cents,allowed_channels_snapshot,notify_url,return_url,expires_at,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'pending') RETURNING id,trade_no,status,expires_at;

-- name: LockMerchantOrder :one
SELECT id,trade_no,merchant_id,status,original_cents,actual_cents,channel,expires_at,channel_locked_at FROM orders WHERE merchant_id=$1 AND trade_no=$2 FOR UPDATE;

-- name: MarkOrderPaid :execrows
UPDATE orders SET status='paid',paid_at=$2,updated_at=now() WHERE id=$1 AND status='pending' AND expires_at>$2;

-- name: ExpireOrders :execrows
UPDATE orders SET status='expired',expire_reason='timeout',updated_at=now() WHERE status='pending' AND expires_at<=now() AND id IN (SELECT id FROM orders WHERE status='pending' AND expires_at<=now() ORDER BY expires_at,id FOR UPDATE SKIP LOCKED LIMIT $1);

-- name: GetPendingOrdersForAmount :many
SELECT id,trade_no,merchant_id,original_cents,actual_cents,channel,expires_at,channel_locked_at,created_at FROM orders WHERE merchant_id=$1 AND channel=$2 AND actual_cents=$3 AND status='pending' ORDER BY id FOR UPDATE;

-- name: ListCallbackAttempts :many
SELECT a.id,a.public_id,a.event_id,e.generation,a.attempt_no,a.started_at,a.finished_at,a.http_status,a.duration_ms,a.response_excerpt,a.error_code,a.next_attempt_at,a.url_redacted FROM callback_attempts a JOIN callback_events e ON e.id=a.event_id WHERE e.order_id=$1 ORDER BY a.id;
