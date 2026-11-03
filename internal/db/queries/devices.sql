-- name: GetActiveDevice :one
SELECT id,public_id,merchant_id,token_hash,name,app_version,last_seen_at,ever_online,offline_episode,offline_notified_episode,revoked_at FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL;

-- name: GetDeviceByTokenHash :one
SELECT id,merchant_id,public_id,token_hash FROM devices WHERE token_hash=$1 AND revoked_at IS NULL;

-- name: InsertBinding :one
INSERT INTO device_bindings(public_id,code_hash,merchant_id,replace_confirmed,expires_at) VALUES($1,$2,$3,$4,$5) RETURNING id,public_id,expires_at;

-- name: ConsumeBinding :execrows
UPDATE device_bindings SET consumed_at=now() WHERE code_hash=$1 AND expires_at>now() AND consumed_at IS NULL;

-- name: InsertReceipt :one
INSERT INTO receipts(public_id,merchant_id,device_id,event_id,channel,amount_cents,occurred_at,provider_trade_no,payer_hint,raw_text_cipher,raw_text_hash,match_state,unmatched_reason,order_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id,public_id,received_at,match_state,unmatched_reason,order_id;

-- name: ListOfflineDevices :many
SELECT id,merchant_id,last_seen_at,offline_episode,offline_notified_episode FROM devices WHERE revoked_at IS NULL AND ever_online=true AND last_seen_at < now()-interval '90 seconds' AND offline_notified_episode<offline_episode ORDER BY last_seen_at LIMIT $1 FOR UPDATE SKIP LOCKED;
