package device

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository contains the device credential lookup shared by HTTP middleware and services.
type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return Repository{pool: pool} }

func (r Repository) DeviceIdentityByTokenHash(ctx context.Context, tokenHash []byte) (DeviceIdentity, error) {
	var identity DeviceIdentity
	err := r.pool.QueryRow(ctx, `SELECT id,merchant_id,public_id,token_hash FROM devices WHERE token_hash=$1 AND revoked_at IS NULL`, tokenHash).Scan(&identity.DeviceID, &identity.MerchantID, &identity.PublicID, &identity.TokenHash)
	return identity, err
}
