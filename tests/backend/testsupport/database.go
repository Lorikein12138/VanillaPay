package testsupport

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"vanillapay/internal/app"
	"vanillapay/internal/config"
	dbmigrate "vanillapay/internal/db/migrate"
	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/internal/subscription"
)

var seedSequence atomic.Uint64

type Clock struct{ At time.Time }

func (c *Clock) Now() time.Time      { return c.At }
func (c *Clock) Set(value time.Time) { c.At = value }

type Database struct {
	Pool   *pgxpool.Pool
	URL    string
	Schema string
	Key    [32]byte
	Clock  *Clock
}

func NewDatabase(t *testing.T) *Database {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("open TEST_DATABASE_URL")
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		admin.Close()
		t.Fatal("generate isolated test schema")
	}
	schema := "vp_backend_" + hex.EncodeToString(raw[:])
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close()
		t.Fatalf("create isolated test schema: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
		t.Fatal("parse TEST_DATABASE_URL")
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	testURL := parsed.String()
	if err := dbmigrate.Up(ctx, testURL); err != nil {
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
		t.Fatalf("apply migrations to isolated schema: %v", err)
	}
	pool, err := pgxpool.New(ctx, testURL)
	if err != nil {
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
		t.Fatal("open isolated test schema")
	}
	var version int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(max(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); err != nil || version < 6 {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
		t.Fatalf("isolated schema migrations incomplete: version=%d err=%v", version, err)
	}
	seed := sha256.Sum256([]byte(schema))
	fixture := &Database{Pool: pool, URL: testURL, Schema: schema, Clock: &Clock{At: time.Now().UTC()}}
	copy(fixture.Key[:], seed[:])
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
	})
	return fixture
}

func (d *Database) SeedMerchant(t *testing.T) (int64, string) {
	t.Helper()
	n := seedSequence.Add(1)
	key := fmt.Sprintf("%064x", n)
	cipher, err := platform.Encrypt(d.Key[:], []byte(key), rand.Reader)
	if err != nil {
		t.Fatal("encrypt merchant key")
	}
	username := fmt.Sprintf("test_merchant_%d", n)
	email := fmt.Sprintf("merchant-%d@example.test", n)
	pid := fmt.Sprintf("9%015d", n)
	var id int64
	err = d.Pool.QueryRow(context.Background(), `INSERT INTO merchants(username,username_normalized,email,email_normalized,pid,password_hash,key_cipher) VALUES($1,$1,$2,$2,$3,'test-password-hash',$4) RETURNING id`, username, email, pid, cipher).Scan(&id)
	if err != nil {
		t.Fatal("seed merchant")
	}
	if _, err := d.Pool.Exec(context.Background(), `INSERT INTO merchant_collection_settings(merchant_id) VALUES($1)`, id); err != nil {
		t.Fatal("seed merchant collection settings")
	}
	if _, err := d.Pool.Exec(context.Background(), `INSERT INTO merchant_ip_rules(merchant_id) VALUES($1)`, id); err != nil {
		t.Fatal("seed merchant IP settings")
	}
	return id, key
}

func (d *Database) SeedPlan(t *testing.T, name string, price int64, days int, channels []string, limit *int) int64 {
	t.Helper()
	var id int64
	err := d.Pool.QueryRow(context.Background(), `INSERT INTO plans(name_zh,name_en,price_cents,duration_days,channels,purchase_limit) VALUES($1,$1,$2,$3,$4,$5) RETURNING id`, name, price, days, channels, limit).Scan(&id)
	if err != nil {
		t.Fatal("seed subscription plan")
	}
	return id
}

func (d *Database) NewApplication(t *testing.T) *app.App {
	t.Helper()
	cfg := config.Config{AppEnv: "production", DatabaseURL: d.URL, PublicBaseURL: "https://pay.example.test", EncryptionKey: d.Key, ListenAddr: "127.0.0.1:0", DataDir: t.TempDir()}
	application, err := app.NewWithDependencies(context.Background(), cfg, app.Dependencies{Clock: d.Clock})
	if err != nil {
		t.Fatalf("construct application with real services: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	return application
}

func (d *Database) PrepareBillingMerchant(t *testing.T, application *app.App, merchantID int64, merchantKey string, planID int64) string {
	t.Helper()
	ctx := context.Background()
	expiry := d.Clock.Now().AddDate(10, 0, 0)
	var channels []string
	if err := d.Pool.QueryRow(ctx, `SELECT channels FROM plans WHERE id=$1`, planID).Scan(&channels); err != nil {
		t.Fatalf("read billing plan channels: %v", err)
	}
	if _, err := application.Services.Subscription.Grant(ctx, platform.Actor{Role: "admin", SubjectID: 1}, merchantID, subscription.GrantRequest{PlanID: fmt.Sprint(planID), ExpiresAt: expiry, Channels: channels, ExpectedRevision: 0, Reason: "integration fixture"}); err != nil {
		t.Fatalf("grant billing merchant subscription: %v", err)
	}
	var pid string
	if err := d.Pool.QueryRow(ctx, `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&pid); err != nil {
		t.Fatal("read billing merchant PID")
	}
	value, err := json.Marshal(map[string]any{"merchant_pid": pid, "merchant_key": merchantKey})
	if err != nil {
		t.Fatal("encode billing configuration")
	}
	var version int64
	if err := d.Pool.QueryRow(ctx, `SELECT version FROM site_settings WHERE singleton=true`).Scan(&version); err != nil {
		t.Fatalf("read site settings version before billing fixture: %v", err)
	}
	if _, err := application.Services.Settings.UpdateGroup(ctx, platform.Actor{Role: "admin", SubjectID: 1}, "billing", settingsUpdate(version, value)); err != nil {
		t.Fatalf("configure billing merchant: %v", err)
	}
	qrPayloads := map[string]string{"alipay": "https://qr.alipay.com/fixture", "wxpay": "wxp://fixture", "qqpay": "https://qianbao.qq.com/fixture"}
	for _, channel := range channels {
		assetID := fmt.Sprintf("fixture-qr-%d", seedSequence.Add(1))
		payloadCipher, err := platform.Encrypt(d.Key[:], []byte(qrPayloads[channel]), rand.Reader)
		if err != nil {
			t.Fatal("encrypt QR fixture")
		}
		checksum := sha256.Sum256([]byte(assetID))
		if _, err := d.Pool.Exec(ctx, `INSERT INTO media_assets(id,owner_merchant_id,kind,channel,file_path,sha256,mime,qr_payload_cipher,active) VALUES($1,$2,'qr',$3,$4,$5,'image/png',$6,true)`, assetID, merchantID, channel, "assets/"+assetID+".png", checksum[:], payloadCipher); err != nil {
			t.Fatalf("seed billing merchant %s QR slot: %v", channel, err)
		}
	}
	token := fmt.Sprintf("device-fixture-token-%d", seedSequence.Add(1))
	tokenHash := sha256.Sum256([]byte(token))
	deviceID := fmt.Sprintf("fixture-device-%d", seedSequence.Add(1))
	if _, err := d.Pool.Exec(ctx, `INSERT INTO devices(public_id,merchant_id,token_hash,name,app_version,last_seen_at,ever_online) VALUES($1,$2,$3,'billing fixture','1.0',$4,true)`, deviceID, merchantID, tokenHash[:], d.Clock.Now()); err != nil {
		t.Fatal("seed online billing device")
	}
	return token
}

func settingsUpdate(version int64, value json.RawMessage) settings.UpdateGroup {
	return settings.UpdateGroup{ExpectedVersion: version, Value: value}
}
