package payment

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"vanillapay/internal/platform"
)

func idempotencyDigest(parts ...string) []byte {
	hash := sha256.New()
	for index, part := range parts {
		if index > 0 {
			_, _ = hash.Write([]byte{0})
		}
		_, _ = hash.Write([]byte(part))
	}
	return hash.Sum(nil)
}

func beginIdempotencyTx(ctx context.Context, tx pgx.Tx, actor platform.Actor, route, key string, digest []byte, now time.Time) ([]byte, bool, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 || len(key) < 8 || len(key) > 128 || strings.TrimSpace(key) != key || strings.ContainsAny(key, "\r\n\x00") {
		return nil, false, apiErr("VALIDATION_ERROR", 400)
	}
	actorID := actor.SubjectID
	if actorID <= 0 {
		actorID = actor.MerchantID
	}
	lockKey := fmt.Sprintf("%s|%d|%s|%s", actor.Role, actorID, route, key)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM idempotency_records WHERE actor_role=$1 AND actor_id=$2 AND route=$3 AND key=$4 AND expires_at<=$5`, actor.Role, actorID, route, key, now); err != nil {
		return nil, false, err
	}
	var oldDigest, oldResult []byte
	err := tx.QueryRow(ctx, `SELECT request_digest,result FROM idempotency_records WHERE actor_role=$1 AND actor_id=$2 AND route=$3 AND key=$4 AND expires_at>$5 FOR UPDATE`, actor.Role, actorID, route, key, now).Scan(&oldDigest, &oldResult)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !equalDigest(oldDigest, digest) {
		return nil, false, apiErr("IDEMPOTENCY_CONFLICT", 409)
	}
	return oldResult, true, nil
}

func recordIdempotencyTx(ctx context.Context, tx pgx.Tx, actor platform.Actor, route, key string, digest, result []byte, now time.Time) error {
	actorID := actor.SubjectID
	if actorID <= 0 {
		actorID = actor.MerchantID
	}
	_, err := tx.Exec(ctx, `INSERT INTO idempotency_records(actor_role,actor_id,route,key,request_digest,result,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, actor.Role, actorID, route, key, digest, result, now.Add(24*time.Hour))
	return err
}

func equalDigest(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var difference byte
	for i := range a {
		difference |= a[i] ^ b[i]
	}
	return difference == 0
}
