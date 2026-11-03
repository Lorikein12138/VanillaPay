package portal

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns the SQL used by the portal's reporting and export paths.
type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return Repository{pool: pool} }

func (r Repository) StatsStatusRows(ctx context.Context, merchantID any, start, end time.Time) (pgx.Rows, error) {
	return r.pool.Query(ctx, `SELECT status,channel,count(*) FROM orders WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status<>'refunded' AND created_at >= $2 AND created_at < $3 GROUP BY status,channel`, merchantID, start, end)
}

func (r Repository) StatsChannelAmountRows(ctx context.Context, merchantID any, start, end time.Time) (pgx.Rows, error) {
	return r.pool.Query(ctx, `SELECT channel,COALESCE(sum(original_cents),0) FROM orders WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status='paid' AND paid_at >= $2 AND paid_at < $3 AND channel IS NOT NULL GROUP BY channel`, merchantID, start, end)
}

func (r Repository) StatsTotalAmount(ctx context.Context, merchantID any, start, end time.Time) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(sum(original_cents),0) FROM orders WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status='paid' AND paid_at >= $2 AND paid_at < $3`, merchantID, start, end).Scan(&total)
	return total, err
}

func (r Repository) StatsTotalOrders(ctx context.Context, merchantID any, start, end time.Time) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM orders WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status<>'refunded' AND created_at >= $2 AND created_at < $3`, merchantID, start, end).Scan(&total)
	return total, err
}

func (r Repository) StatsTrendCountRows(ctx context.Context, merchantID any, start, end time.Time, timezone string) (pgx.Rows, error) {
	return r.pool.Query(ctx, `SELECT to_char(created_at AT TIME ZONE $3,'YYYY-MM-DD'),status,count(*) FROM orders WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status<>'refunded' AND created_at >= $2 AND created_at < $4 GROUP BY 1,2`, merchantID, start, timezone, end)
}

func (r Repository) StatsTrendAmountRows(ctx context.Context, merchantID any, start, end time.Time, timezone string) (pgx.Rows, error) {
	return r.pool.Query(ctx, `SELECT to_char(paid_at AT TIME ZONE $3,'YYYY-MM-DD'),COALESCE(sum(original_cents),0) FROM orders WHERE ($1::bigint IS NULL OR merchant_id=$1) AND status='paid' AND paid_at >= $2 AND paid_at < $4 GROUP BY 1`, merchantID, start, timezone, end)
}

func (r Repository) AdminStats(ctx context.Context, start, end, now time.Time) (merchantCount, activeSubscriptions, subscriptionRevenue int64, err error) {
	if err = r.pool.QueryRow(ctx, `SELECT count(*) FROM merchants`).Scan(&merchantCount); err != nil {
		return
	}
	if err = r.pool.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE plan_id IS NOT NULL AND expires_at>$1`, now).Scan(&activeSubscriptions); err != nil {
		return
	}
	err = r.pool.QueryRow(ctx, `SELECT COALESCE(sum((plan_snapshot->>'price_cents')::bigint),0) FROM purchases WHERE state='paid' AND paid_at >= $1 AND paid_at < $2`, start, end).Scan(&subscriptionRevenue)
	return
}

func (r Repository) CountOrders(ctx context.Context, merchantID *int64, q OrderFilter, status, channel, query string) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM orders o WHERE ($1::bigint IS NULL OR o.merchant_id=$1) AND ($2::timestamptz IS NULL OR o.created_at >= $2) AND ($3::timestamptz IS NULL OR o.created_at < $3) AND ($4='' OR o.status=$4) AND ($5='' OR o.channel=$5) AND ($6='' OR o.trade_no=$6 OR o.out_trade_no=$6 OR o.name ILIKE '%'||$6||'%')`, merchantID, nullTimePtr(q.Start), nullTimePtr(q.End), status, channel, query).Scan(&total)
	return total, err
}

func (r Repository) OrderRows(ctx context.Context, merchantID *int64, q OrderFilter, status, channel, query string, cursorAt time.Time, cursorID int64, limit int) (pgx.Rows, error) {
	return r.pool.Query(ctx, `SELECT o.id,o.trade_no,o.out_trade_no,o.merchant_id,o.merchant_username_snapshot,o.purpose,o.protocol,o.channel,o.name,o.product_id,o.param,o.original_cents,o.actual_cents,o.status,o.expire_reason,o.is_manual,o.manual_reason,o.created_at,o.paid_at,o.expires_at,o.refunded_at,COALESCE((SELECT state FROM callback_events c WHERE c.order_id=o.id ORDER BY generation DESC LIMIT 1),CASE WHEN o.purpose='subscription' THEN 'internal_fulfillment' ELSE 'none' END),r.public_id FROM orders o LEFT JOIN receipts r ON r.order_id=o.id WHERE ($1::bigint IS NULL OR o.merchant_id=$1) AND ($2::timestamptz IS NULL OR o.created_at >= $2) AND ($3::timestamptz IS NULL OR o.created_at < $3) AND ($4='' OR o.status=$4) AND ($5='' OR o.channel=$5) AND ($6='' OR o.trade_no=$6 OR o.out_trade_no=$6 OR o.name ILIKE '%'||$6||'%') AND ($7::timestamptz IS NULL OR (o.created_at,o.id)<($7,$8)) ORDER BY o.created_at DESC,o.id DESC LIMIT $9`, merchantID, nullTimePtr(q.Start), nullTimePtr(q.End), status, channel, query, nullTime(cursorAt), cursorID, limit+1)
}

func (r Repository) CountReceipts(ctx context.Context, merchantID int64, q ReceiptFilter) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM receipts WHERE merchant_id=$1 AND ($2::timestamptz IS NULL OR received_at >= $2) AND ($3::timestamptz IS NULL OR received_at < $3) AND ($4='' OR channel=$4) AND ($5='' OR match_state=$5)`, merchantID, nullTimePtr(q.Start), nullTimePtr(q.End), q.Channel, q.MatchState).Scan(&total)
	return total, err
}

func (r Repository) ReceiptRows(ctx context.Context, merchantID int64, q ReceiptFilter, cursorAt time.Time, cursorID int64, limit int) (pgx.Rows, error) {
	return r.pool.Query(ctx, `SELECT r.id,r.public_id,r.event_id,r.device_id,r.channel,r.amount_cents,r.occurred_at,r.received_at,r.provider_trade_no,r.payer_hint,r.match_state,r.unmatched_reason,o.trade_no FROM receipts r LEFT JOIN orders o ON o.id=r.order_id WHERE r.merchant_id=$1 AND ($2::timestamptz IS NULL OR r.received_at >= $2) AND ($3::timestamptz IS NULL OR r.received_at < $3) AND ($4='' OR r.channel=$4) AND ($5='' OR r.match_state=$5) AND ($6::timestamptz IS NULL OR (r.received_at,r.id)<($6,$7)) ORDER BY r.received_at DESC,r.id DESC LIMIT $8`, merchantID, nullTimePtr(q.Start), nullTimePtr(q.End), q.Channel, q.MatchState, nullTime(cursorAt), cursorID, limit+1)
}

func (r Repository) CountAudits(ctx context.Context, merchantID *int64, q AuditFilter) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE ($1::bigint IS NULL OR merchant_id=$1) AND ($2='' OR action=$2) AND ($3='' OR result=$3) AND ($4='' OR actor_role=$4) AND ($5::timestamptz IS NULL OR created_at >= $5) AND ($6::timestamptz IS NULL OR created_at < $6)`, merchantID, q.Action, q.Result, q.ActorRole, nullTimePtr(q.Start), nullTimePtr(q.End)).Scan(&total)
	return total, err
}

func (r Repository) AuditRows(ctx context.Context, merchantID *int64, q AuditFilter, cursorAt time.Time, cursorID int64, limit int) (pgx.Rows, error) {
	return r.pool.Query(ctx, `SELECT id,actor_role,actor_id,merchant_id,action,target_type,target_id,result,host(ip),created_at,request_id,safe_metadata FROM audit_logs WHERE ($1::bigint IS NULL OR merchant_id=$1) AND ($2='' OR action=$2) AND ($3='' OR result=$3) AND ($4='' OR actor_role=$4) AND ($5::timestamptz IS NULL OR created_at >= $5) AND ($6::timestamptz IS NULL OR created_at < $6) AND ($7::timestamptz IS NULL OR (created_at,id)<($7,$8)) ORDER BY created_at DESC,id DESC LIMIT $9`, merchantID, q.Action, q.Result, q.ActorRole, nullTimePtr(q.Start), nullTimePtr(q.End), nullTime(cursorAt), cursorID, limit+1)
}

func (r Repository) BeginOrderExport(ctx context.Context) (pgx.Tx, error) {
	return r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
}

func (r Repository) ExportOrderRows(ctx context.Context, tx pgx.Tx, merchantID *int64, q OrderFilter) (pgx.Rows, error) {
	return tx.Query(ctx, `SELECT o.trade_no,o.out_trade_no,o.merchant_id,o.merchant_username_snapshot,o.channel,o.name,o.product_id,o.original_cents,o.actual_cents,o.status,o.is_manual,o.created_at,o.paid_at,o.expires_at,o.refunded_at,COALESCE((SELECT state FROM callback_events c WHERE c.order_id=o.id ORDER BY generation DESC LIMIT 1),CASE WHEN o.purpose='subscription' THEN 'internal_fulfillment' ELSE 'none' END) FROM orders o WHERE ($1::bigint IS NULL OR o.merchant_id=$1) AND ($2::timestamptz IS NULL OR o.created_at >= $2) AND ($3::timestamptz IS NULL OR o.created_at < $3) AND ($4='' OR o.status=$4) AND ($5='' OR o.channel=$5) AND ($6='' OR o.trade_no=$6 OR o.out_trade_no=$6 OR o.name ILIKE '%'||$6||'%') ORDER BY o.created_at DESC,o.id DESC`, merchantID, nullTimePtr(q.Start), nullTimePtr(q.End), q.Status, q.Channel, strings.TrimSpace(q.Query))
}
