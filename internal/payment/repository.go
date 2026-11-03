package payment

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository contains order lookups shared by the protocol and worker adapters.
type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return Repository{pool: pool} }

func (r Repository) OrderID(ctx context.Context, trade string) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `SELECT id FROM orders WHERE trade_no=$1`, trade).Scan(&id)
	return id, err
}

func (r Repository) KeyCipherForOrder(ctx context.Context, trade string) ([]byte, error) {
	var key []byte
	err := r.pool.QueryRow(ctx, `SELECT signing_key_cipher FROM orders WHERE trade_no=$1`, trade).Scan(&key)
	return key, err
}
