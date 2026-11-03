-- name: ListVisiblePlans :many
SELECT id,revision,name_zh,name_en,price_cents,duration_days,channels,purchase_limit,deleted_at,created_at,updated_at FROM plans WHERE deleted_at IS NULL ORDER BY price_cents,id;

-- name: GetPlan :one
SELECT id,revision,name_zh,name_en,price_cents,duration_days,channels,purchase_limit,deleted_at,created_at,updated_at FROM plans WHERE id=$1;

-- name: UpsertPurchaseCounter :one
INSERT INTO plan_purchase_counters(merchant_id,plan_id) VALUES($1,$2) ON CONFLICT(merchant_id,plan_id) DO UPDATE SET merchant_id=EXCLUDED.merchant_id RETURNING merchant_id,plan_id,paid_count,reserved_count;

-- name: GetSubscription :one
SELECT merchant_id,plan_id,snapshot,expires_at,revision,active_chain_id,source,updated_at FROM subscriptions WHERE merchant_id=$1;

-- name: GetPendingPurchase :one
SELECT id,public_id,buyer_id,billing_merchant_id,plan_id,plan_snapshot,purchase_kind,expected_subscription_revision,state,payment_trade_no,fulfillment_at,paid_at,expires_at,created_at FROM purchases WHERE buyer_id=$1 AND state='pending';

-- name: InsertPurchase :one
INSERT INTO purchases(public_id,buyer_id,billing_merchant_id,plan_id,plan_snapshot,purchase_kind,expected_subscription_revision,state,idempotency_key,request_digest,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10) RETURNING id,public_id,state,expires_at;

-- name: ListPurchaseGrants :many
SELECT id,purchase_id,merchant_id,chain_id,kind,duration,effective_at,fixed_expires_at,snapshot,voided_at,refunded_at FROM subscription_grants WHERE merchant_id=$1 ORDER BY effective_at,id;

-- name: CountAdminPurchases :one
SELECT count(*) FROM purchases p
WHERE ($1::timestamptz IS NULL OR p.created_at >= $1) AND ($2::timestamptz IS NULL OR p.created_at < $2)
  AND ($3::bigint IS NULL OR p.buyer_id=$3) AND ($4::bigint IS NULL OR p.plan_id=$4) AND ($5='' OR p.state=$5);

-- name: ListAdminPurchases :many
SELECT p.id,p.public_id,p.plan_snapshot,p.purchase_kind,p.state,p.payment_trade_no,p.created_at,p.paid_at,p.expires_at,p.fulfillment_at,p.buyer_id,p.billing_merchant_id
FROM purchases p
WHERE ($1::timestamptz IS NULL OR p.created_at >= $1) AND ($2::timestamptz IS NULL OR p.created_at < $2)
  AND ($3::bigint IS NULL OR p.buyer_id=$3) AND ($4::bigint IS NULL OR p.plan_id=$4) AND ($5='' OR p.state=$5)
  AND ($6::timestamptz IS NULL OR (p.created_at,p.id)<($6,$7))
ORDER BY p.created_at DESC,p.id DESC LIMIT $8;
