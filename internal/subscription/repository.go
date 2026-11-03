package subscription

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository centralizes read queries used by the subscription service.
type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return Repository{pool: pool} }

func (r Repository) CountPurchases(ctx context.Context, q PurchaseFilter) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM purchases p WHERE ($1::timestamptz IS NULL OR p.created_at >= $1) AND ($2::timestamptz IS NULL OR p.created_at < $2) AND ($3::bigint IS NULL OR p.buyer_id=$3) AND ($4::bigint IS NULL OR p.plan_id=$4) AND ($5='' OR p.state=$5)`, q.Start, q.End, q.BuyerID, q.PlanID, q.State).Scan(&total)
	return total, err
}

func (r Repository) ListPurchases(ctx context.Context, q PurchaseFilter, cursorTime *time.Time, cursorID *int64, limit int) (pgx.Rows, error) {
	return r.pool.Query(ctx, `SELECT p.id,p.public_id,p.plan_snapshot,p.purchase_kind,p.state,p.payment_trade_no,p.created_at,p.paid_at,p.expires_at,p.fulfillment_at,p.buyer_id,p.billing_merchant_id FROM purchases p WHERE ($1::timestamptz IS NULL OR p.created_at >= $1) AND ($2::timestamptz IS NULL OR p.created_at < $2) AND ($3::bigint IS NULL OR p.buyer_id=$3) AND ($4::bigint IS NULL OR p.plan_id=$4) AND ($5='' OR p.state=$5) AND ($6::timestamptz IS NULL OR (p.created_at,p.id)<($6,$7)) ORDER BY p.created_at DESC,p.id DESC LIMIT $8`, q.Start, q.End, q.BuyerID, q.PlanID, q.State, cursorTime, cursorID, limit)
}
