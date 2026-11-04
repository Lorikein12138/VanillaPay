package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"vanillapay/internal/db/migrations"
)

const advisoryLockID int64 = 0x56504d4947524154

// Up applies application migrations followed by River's migrations while
// holding one database advisory lock.
func Up(ctx context.Context, databaseURL string) error {
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("database pool initialization failed")
	}
	defer pool.Close()

	lockTx, err := pool.Begin(ctx)
	if err != nil {
		return errors.New("database migration lock could not be acquired")
	}
	defer func() {
		_ = lockTx.Rollback(context.Background())
	}()
	if _, err := lockTx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", advisoryLockID); err != nil {
		return errors.New("database migration lock could not be acquired")
	}

	if err := runGoose(ctx, databaseURL); err != nil {
		return fmt.Errorf("application migrations failed: %w", err)
	}
	if err := runRiver(ctx, pool); err != nil {
		return fmt.Errorf("river migrations failed: %w", err)
	}
	if err := lockTx.Commit(ctx); err != nil {
		return errors.New("database migration lock could not be released cleanly")
	}
	return nil
}

func runGoose(ctx context.Context, databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errors.New("database connection configuration failed")
	}
	defer func() { _ = db.Close() }() // Cleanup must not replace the operation result.
	db.SetMaxOpenConns(2)
	if err := db.PingContext(ctx); err != nil {
		return errors.New("database connection failed")
	}
	if err := goose.SetDialect("postgres"); err != nil {
		return errors.New("goose dialect configuration failed")
	}
	migrationFS := migrations.FS()
	files, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		return errors.New("goose migration discovery failed")
	}
	if len(files) == 0 {
		return nil
	}
	goose.SetBaseFS(migrationFS)
	return goose.UpContext(ctx, db, ".")
}

func runRiver(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return errors.New("river migrator initialization failed")
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return err
	}
	return nil
}
