package foundation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	dbmigrate "vanillapay/internal/db/migrate"
	"vanillapay/internal/db/migrations"
)

func TestDB_FoundationMigrationsAreRepeatableInAnIsolatedSchema(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal("open TEST_DATABASE_URL")
	}
	t.Cleanup(admin.Close) // Registered first so the schema cleanup below runs before closing.
	var schemaBytes [8]byte
	if _, err := rand.Read(schemaBytes[:]); err != nil {
		t.Fatal("generate test schema name")
	}
	schema := "vp_test_" + hex.EncodeToString(schemaBytes[:])
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal("create isolated test schema")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop isolated test schema %s: %v", schema, err)
		}
	})

	schemaURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal("parse TEST_DATABASE_URL")
	}
	query := schemaURL.Query()
	query.Set("search_path", schema)
	schemaURL.RawQuery = query.Encode()

	if err := dbmigrate.Up(ctx, schemaURL.String()); err != nil {
		t.Fatalf("first migration up failed: %v", err)
	}
	t.Run("V03.B.r01/v01", func(t *testing.T) {
		var version int64
		if err := admin.QueryRow(ctx, "SELECT COALESCE(max(version_id),0) FROM "+pgx.Identifier{schema}.Sanitize()+".goose_db_version WHERE is_applied").Scan(&version); err != nil || version != 7 {
			t.Fatalf("empty isolated schema goose version=%d err=%v, want 7", version, err)
		}
	})

	migrationDB, err := sql.Open("pgx", schemaURL.String())
	if err != nil {
		t.Fatal("open isolated schema for down/up verification")
	}
	defer func() { _ = migrationDB.Close() }() // Cleanup must not replace the operation result.
	provider, err := goose.NewProvider(database.DialectPostgres, migrationDB, migrations.FS())
	if err != nil {
		t.Fatal("initialize isolated schema migration provider")
	}
	t.Run("V03.B.r02/v01", func(t *testing.T) {
		before := applicationCatalog(ctx, t, admin, schema)
		for _, kind := range []string{"constraint c ", "constraint f ", "constraint p ", "constraint u ", "index "} {
			if !catalogHasPrefix(before, kind) {
				t.Fatalf("first migration up produced no %q entries", strings.TrimSpace(kind))
			}
		}
		if _, err := provider.DownTo(ctx, 0); err != nil {
			t.Fatalf("down all application migrations: %v", err)
		}
		if remaining := applicationCatalog(ctx, t, admin, schema); len(remaining) != 0 {
			t.Fatalf("down to 0 left application constraints/indexes: %s", strings.Join(remaining, "\n"))
		}
		if _, err := provider.UpTo(ctx, 7); err != nil {
			t.Fatalf("reapply all application migrations: %v", err)
		}
		after := applicationCatalog(ctx, t, admin, schema)
		if strings.Join(before, "\n") != strings.Join(after, "\n") {
			t.Fatalf("down/up changed application constraints/indexes\nbefore:\n%s\nafter:\n%s", strings.Join(before, "\n"), strings.Join(after, "\n"))
		}
	})
	t.Run("turnstile upgrade", func(t *testing.T) {
		for _, configured := range []bool{true, false} {
			if _, err := provider.DownTo(ctx, 6); err != nil {
				t.Fatal(err)
			}
			if _, err := migrationDB.ExecContext(ctx, `UPDATE site_settings SET security=security || '{"turnstile_site_key":"widget","allowed_hostname":"old.example.test"}'::jsonb`); err != nil {
				t.Fatal(err)
			}
			if configured {
				if _, err := migrationDB.ExecContext(ctx, `INSERT INTO secret_settings(name,encrypted_value) VALUES('turnstile_secret',$1)`, []byte{1}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := provider.UpTo(ctx, 7); err != nil {
				t.Fatal(err)
			}
			var enabled bool
			var legacyHost string
			if err := migrationDB.QueryRowContext(ctx, `SELECT (security->>'turnstile_enabled')::boolean, COALESCE(security->>'allowed_hostname','') FROM site_settings`).Scan(&enabled, &legacyHost); err != nil || enabled != configured || legacyHost != "old.example.test" {
				t.Fatalf("configured=%t enabled=%t legacy=%q err=%v", configured, enabled, legacyHost, err)
			}
			if _, err := migrationDB.ExecContext(ctx, `DELETE FROM secret_settings WHERE name='turnstile_secret'`); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := provider.DownTo(ctx, 6); err != nil {
			t.Fatal(err)
		}
		var legacyHost string
		var hasEnabled bool
		if err := migrationDB.QueryRowContext(ctx, `SELECT security->>'allowed_hostname', security ? 'turnstile_enabled' FROM site_settings`).Scan(&legacyHost, &hasEnabled); err != nil || legacyHost != "old.example.test" || hasEnabled {
			t.Fatalf("down to 6 lost the legacy hostname: %q enabled-key=%t err=%v", legacyHost, hasEnabled, err)
		}
		if _, err := provider.UpTo(ctx, 7); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("V03.B.r05/v01", func(t *testing.T) {
		if err := dbmigrate.Up(ctx, schemaURL.String()); err != nil {
			t.Fatalf("repeat application and River migration up: %v", err)
		}
		var riverJobExists bool
		if err := admin.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", schema+".river_job").Scan(&riverJobExists); err != nil || !riverJobExists {
			t.Fatalf("River job table is absent from the same isolated DB schema: present=%t err=%v", riverJobExists, err)
		}
	})
	t.Run("V03.B.r05/v02", func(t *testing.T) {
		entries, err := fs.ReadDir(migrations.FS(), ".")
		if err != nil {
			t.Fatal("list embedded application migrations")
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
				continue
			}
			source, err := fs.ReadFile(migrations.FS(), entry.Name())
			if err != nil {
				t.Fatalf("read embedded migration %s: %v", entry.Name(), err)
			}
			if strings.Contains(strings.ToLower(string(source)), "create table river_job") {
				t.Fatalf("application migration %s conflicts with River-owned table", entry.Name())
			}
		}
	})

}

// applicationCatalog lists every constraint and index definition on the
// application tables of schema; River and goose bookkeeping tables are excluded.
func applicationCatalog(ctx context.Context, t *testing.T, admin *pgxpool.Pool, schema string) []string {
	t.Helper()
	rows, err := admin.Query(ctx, `
SELECT 'constraint ' || c.contype::text || ' ' || r.relname::text || '.' || c.conname::text || ' ' || pg_get_constraintdef(c.oid)
FROM pg_constraint c JOIN pg_class r ON r.oid = c.conrelid
WHERE c.connamespace = $1::text::regnamespace AND r.relname NOT LIKE 'river%' AND r.relname <> 'goose_db_version'
UNION ALL
SELECT 'index ' || i.tablename::text || '.' || i.indexname::text || ' ' || replace(i.indexdef, quote_ident($1::text) || '.', '')
FROM pg_indexes i
WHERE i.schemaname = $1::text AND i.tablename NOT LIKE 'river%' AND i.tablename <> 'goose_db_version'
ORDER BY 1`, schema)
	if err != nil {
		t.Fatalf("read application constraints/indexes: %v", err)
	}
	entries, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect application constraints/indexes: %v", err)
	}
	return entries
}

func catalogHasPrefix(entries []string, prefix string) bool {
	for _, entry := range entries {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
