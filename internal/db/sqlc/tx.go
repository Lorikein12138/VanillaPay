package db

import (
	"context"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrVersionConflict = errors.New("VERSION_CONFLICT")

// InTx applies the shared transaction retry policy. The callback must not
// perform network I/O and must be safe to execute again after a serialization
// failure or deadlock.
func InTx(ctx context.Context, pool *pgxpool.Pool, options pgx.TxOptions, action func(pgx.Tx) error) error {
	if pool == nil || action == nil {
		return errors.New("transaction dependencies are required")
	}
	for attempt := 0; attempt < 3; attempt++ {
		tx, err := pool.BeginTx(ctx, options)
		if err != nil {
			return err
		}
		err = action(tx)
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err == nil {
			return nil
		}
		if !retryableTxError(err) {
			return err
		}
		_ = tx.Rollback(ctx)
	}
	return ErrVersionConflict
}

func retryableTxError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}

// LockBusinessRows applies the global-settings then ascending-merchant lock
// prefix used by all cross-module write transactions.
func LockBusinessRows(ctx context.Context, tx pgx.Tx, merchantIDs []int64, updateSettings bool) error {
	lock := "FOR SHARE"
	if updateSettings {
		lock = "FOR UPDATE"
	}
	var singleton bool
	if err := tx.QueryRow(ctx, "SELECT singleton FROM site_settings WHERE singleton=true "+lock).Scan(&singleton); err != nil {
		return err
	}
	ids := append([]int64(nil), merchantIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	unique := ids[:0]
	for _, id := range ids {
		if id <= 0 || (len(unique) > 0 && unique[len(unique)-1] == id) {
			continue
		}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, "SELECT id FROM merchants WHERE id = ANY($1::bigint[]) ORDER BY id FOR UPDATE", unique)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
	}
	return rows.Err()
}
