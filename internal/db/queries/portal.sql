-- name: InsertAnnouncement :one
INSERT INTO announcements(public_id,title,body,popup) VALUES($1,$2,$3,$4) RETURNING id,public_id,revision,title,body,popup,published_at,deleted_at;

-- name: ListAnnouncements :many
SELECT id,public_id,revision,title,body,popup,published_at,deleted_at FROM announcements WHERE deleted_at IS NULL ORDER BY published_at DESC,id DESC LIMIT $1;

-- name: InsertNotification :one
INSERT INTO notifications(public_id,merchant_id,kind,source_id,dedupe_key,safe_payload) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(dedupe_key) DO UPDATE SET dedupe_key=EXCLUDED.dedupe_key RETURNING id,public_id,kind,source_id,safe_payload,read_at,created_at;

-- name: ListMerchantNotifications :many
SELECT id,public_id,kind,source_id,safe_payload,read_at,created_at FROM notifications WHERE merchant_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2;

-- name: ListMerchantOrders :many
SELECT id,trade_no,out_trade_no,purpose,protocol,name,product_id,param,original_cents,actual_cents,channel,status,expire_reason,is_manual,manual_reason,created_at,paid_at,expires_at,refunded_at FROM orders WHERE merchant_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2;

-- name: GetMerchantStatsOrders :many
SELECT status,channel,original_cents,created_at,paid_at FROM orders WHERE merchant_id=$1 AND created_at >= $2 AND created_at < $3 ORDER BY created_at,id;

-- name: GetAllPaidSubscriptionPurchases :many
SELECT buyer_id,plan_snapshot,paid_at FROM purchases WHERE state IN ('paid','refunded') AND paid_at >= $1 AND paid_at < $2 ORDER BY paid_at,id;

-- name: PortalStatsStatusRows :many
SELECT status,channel,count(*) FROM orders
WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status<>'refunded' AND created_at >= $2 AND created_at < $3
GROUP BY status,channel;

-- name: PortalStatsChannelAmountRows :many
SELECT channel,COALESCE(sum(original_cents),0) FROM orders
WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status='paid' AND paid_at >= $2 AND paid_at < $3 AND channel IS NOT NULL
GROUP BY channel;

-- name: PortalStatsTotalAmount :one
SELECT COALESCE(sum(original_cents),0) FROM orders
WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status='paid' AND paid_at >= $2 AND paid_at < $3;

-- name: PortalStatsTotalOrders :one
SELECT count(*) FROM orders
WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status<>'refunded' AND created_at >= $2 AND created_at < $3;

-- name: PortalStatsTrendCounts :many
SELECT to_char(created_at AT TIME ZONE $3,'YYYY-MM-DD'),status,count(*) FROM orders
WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status<>'refunded' AND created_at >= $2 AND created_at < $4
GROUP BY 1,2;

-- name: PortalStatsTrendAmounts :many
SELECT to_char(paid_at AT TIME ZONE $3,'YYYY-MM-DD'),COALESCE(sum(original_cents),0) FROM orders
WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status='paid' AND paid_at >= $2 AND paid_at < $4
GROUP BY 1;

-- name: PortalCountOrders :one
SELECT count(*) FROM orders o
WHERE ($1::bigint IS NULL OR o.merchant_id=$1)
  AND ($2::timestamptz IS NULL OR o.created_at >= $2) AND ($3::timestamptz IS NULL OR o.created_at < $3)
  AND ($4='' OR o.status=$4) AND ($5='' OR o.channel=$5)
  AND ($6='' OR o.trade_no=$6 OR o.out_trade_no=$6 OR o.name ILIKE '%'||$6||'%');

-- name: PortalListOrders :many
SELECT o.id,o.trade_no,o.out_trade_no,o.merchant_id,o.merchant_username_snapshot,o.purpose,o.protocol,o.channel,o.name,o.product_id,o.param,o.original_cents,o.actual_cents,o.status,o.expire_reason,o.is_manual,o.manual_reason,o.created_at,o.paid_at,o.expires_at,o.refunded_at,
       COALESCE((SELECT state FROM callback_events c WHERE c.order_id=o.id ORDER BY generation DESC LIMIT 1),CASE WHEN o.purpose='subscription' THEN 'internal_fulfillment' ELSE 'none' END),r.public_id
FROM orders o LEFT JOIN receipts r ON r.order_id=o.id
WHERE ($1::bigint IS NULL OR o.merchant_id=$1)
  AND ($2::timestamptz IS NULL OR o.created_at >= $2) AND ($3::timestamptz IS NULL OR o.created_at < $3)
  AND ($4='' OR o.status=$4) AND ($5='' OR o.channel=$5)
  AND ($6='' OR o.trade_no=$6 OR o.out_trade_no=$6 OR o.name ILIKE '%'||$6||'%')
  AND ($7::timestamptz IS NULL OR (o.created_at,o.id)<($7,$8))
ORDER BY o.created_at DESC,o.id DESC LIMIT $9;

-- name: PortalCountReceipts :one
SELECT count(*) FROM receipts WHERE merchant_id=$1
  AND ($2::timestamptz IS NULL OR received_at >= $2) AND ($3::timestamptz IS NULL OR received_at < $3)
  AND ($4='' OR channel=$4) AND ($5='' OR match_state=$5);

-- name: PortalListReceipts :many
SELECT r.id,r.public_id,r.event_id,r.device_id,r.channel,r.amount_cents,r.occurred_at,r.received_at,r.provider_trade_no,r.payer_hint,r.match_state,r.unmatched_reason,o.trade_no
FROM receipts r LEFT JOIN orders o ON o.id=r.order_id
WHERE r.merchant_id=$1 AND ($2::timestamptz IS NULL OR r.received_at >= $2) AND ($3::timestamptz IS NULL OR r.received_at < $3)
  AND ($4='' OR r.channel=$4) AND ($5='' OR r.match_state=$5)
  AND ($6::timestamptz IS NULL OR (r.received_at,r.id)<($6,$7))
ORDER BY r.received_at DESC,r.id DESC LIMIT $8;

-- name: PortalCountAudits :one
SELECT count(*) FROM audit_logs WHERE ($1::bigint IS NULL OR merchant_id=$1)
  AND ($2='' OR action=$2) AND ($3='' OR result=$3) AND ($4='' OR actor_role=$4)
  AND ($5::timestamptz IS NULL OR created_at >= $5) AND ($6::timestamptz IS NULL OR created_at < $6);

-- name: PortalListAudits :many
SELECT id,actor_role,actor_id,merchant_id,action,target_type,target_id,result,host(ip),created_at,request_id,safe_metadata
FROM audit_logs WHERE ($1::bigint IS NULL OR merchant_id=$1) AND ($2='' OR action=$2) AND ($3='' OR result=$3) AND ($4='' OR actor_role=$4)
  AND ($5::timestamptz IS NULL OR created_at >= $5) AND ($6::timestamptz IS NULL OR created_at < $6)
  AND ($7::timestamptz IS NULL OR (created_at,id)<($7,$8))
ORDER BY created_at DESC,id DESC LIMIT $9;

-- name: PortalExportOrders :many
SELECT o.trade_no,o.out_trade_no,o.merchant_id,o.merchant_username_snapshot,o.channel,o.name,o.product_id,o.original_cents,o.actual_cents,o.status,o.is_manual,o.created_at,o.paid_at,o.expires_at,o.refunded_at,
       COALESCE((SELECT state FROM callback_events c WHERE c.order_id=o.id ORDER BY generation DESC LIMIT 1),CASE WHEN o.purpose='subscription' THEN 'internal_fulfillment' ELSE 'none' END)
FROM orders o WHERE ($1::bigint IS NULL OR o.merchant_id=$1)
  AND ($2::timestamptz IS NULL OR o.created_at >= $2) AND ($3::timestamptz IS NULL OR o.created_at < $3)
  AND ($4='' OR o.status=$4) AND ($5='' OR o.channel=$5)
  AND ($6='' OR o.trade_no=$6 OR o.out_trade_no=$6 OR o.name ILIKE '%'||$6||'%')
ORDER BY o.created_at DESC,o.id DESC;
