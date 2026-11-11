package device_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"vanillapay/internal/db/migrate"
	"vanillapay/internal/db/migrations"
	"vanillapay/internal/device"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_BindingIsSingleUseAndReplacementRevokesOldCredential(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	firstBinding, err := application.Services.Device.NewBinding(ctx, actor, false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := application.Services.Device.Redeem(ctx, device.RedeemRequest{Code: firstBinding.Code, DeviceName: "device one", AppVersion: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Services.Device.Redeem(ctx, device.RedeemRequest{Code: firstBinding.Code, DeviceName: "device one", AppVersion: "1.0"}); !hasCode(err, "BINDING_USED") {
		t.Fatalf("binding replay error = %v", err)
	}
	if _, err := application.Services.Device.NewBinding(ctx, actor, false); !hasCode(err, "DEVICE_ALREADY_BOUND") {
		t.Fatalf("replacement without confirmation error = %v", err)
	}
	secondBinding, err := application.Services.Device.NewBinding(ctx, actor, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.Services.Device.Redeem(ctx, device.RedeemRequest{Code: secondBinding.Code, DeviceName: "device two", AppVersion: "1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Services.Device.Authenticate(ctx, first.DeviceToken); err == nil {
		t.Fatal("replaced device credential remained valid")
	}
	if _, err := application.Services.Device.Authenticate(ctx, second.DeviceToken); err != nil {
		t.Fatalf("replacement credential rejected: %v", err)
	}
}

func hasCode(err error, code string) bool {
	apiErr := &platform.Error{}
	ok := errors.As(err, &apiErr)
	return ok && apiErr.Code == code
}

func TestDB_OfflineEpisodeQueuesOneNotificationAndMail(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	binding, err := application.Services.Device.NewBinding(ctx, actor, false)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := application.Services.Device.Redeem(ctx, device.RedeemRequest{Code: binding.Code, DeviceName: "receipt device", AppVersion: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := application.Services.Device.Authenticate(ctx, credential.DeviceToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Services.Device.Heartbeat(ctx, identity, device.HeartbeatRequest{AppVersion: "1.0"}); err != nil {
		t.Fatal(err)
	}
	db.Clock.Set(db.Clock.Now().Add(91 * time.Second))
	type scanResult struct {
		count int
		err   error
	}
	start := make(chan struct{})
	results := make(chan scanResult, 2)
	for range 2 {
		go func() {
			<-start
			count, err := application.Services.Device.DetectOffline(ctx, 50)
			results <- scanResult{count, err}
		}()
	}
	close(start)
	total := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Errorf("concurrent offline scan: %v", result.err)
		}
		total += result.count
	}
	if total != 1 {
		t.Fatalf("concurrent offline scans queued %d alerts, want 1", total)
	}
	if count, err := application.Services.Device.DetectOffline(ctx, 50); err != nil || count != 0 {
		t.Fatalf("repeated offline scan = %d, %v", count, err)
	}
	var notifications, mails int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE merchant_id=$1 AND kind='device_offline'`, merchantID).Scan(&notifications); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM mail_deliveries WHERE template='device_offline'`).Scan(&mails); err != nil {
		t.Fatal(err)
	}
	if notifications != 1 || mails != 1 {
		t.Fatalf("offline side effects notifications=%d mails=%d", notifications, mails)
	}
}

func TestDB_OfflineAlertMigrationPreservesArmingState(t *testing.T) {
	fixtures := []struct {
		name             string
		elapsed          time.Duration
		episode, alerted int
		revoked          bool
		wantOnlineSince  bool
		wantAlerts       int
	}{
		{"briefly-recovered-and-online", 10 * time.Second, 1, 1, false, true, 0},
		{"stable-recovery-then-offline-before-scan", 100 * time.Second, 1, 1, false, false, 0},
		{"still-in-alerted-outage", time.Hour, 0, 1, false, false, 0},
		{"not-yet-alerted", time.Hour, 0, 0, false, false, 1},
		{"revoked", time.Hour, 0, 0, true, false, 0},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			db := testsupport.NewDatabase(t)
			ctx := t.Context()
			application := db.NewApplication(t)
			legacyDB, err := sql.Open("pgx", db.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = legacyDB.Close() }()
			provider, err := goose.NewProvider(database.DialectPostgres, legacyDB, migrations.FS())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Down(ctx); err != nil {
				t.Fatal(err)
			}

			merchantID, _ := db.SeedMerchant(t)
			var before, after time.Time
			if err := db.Pool.QueryRow(ctx, `SELECT now()`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			var revokedAt *time.Time
			if fixture.revoked {
				revokedAt = &before
			}
			if _, err := db.Pool.Exec(ctx, `INSERT INTO devices
				(public_id,merchant_id,token_hash,name,last_seen_at,ever_online,offline_episode,offline_notified_episode,revoked_at)
				VALUES ($1,$2,$3,$1,$4,true,$5,$6,$7)`,
				fixture.name, merchantID, []byte(fixture.name), before.Add(-fixture.elapsed),
				fixture.episode, fixture.alerted, revokedAt); err != nil {
				t.Fatal(err)
			}
			if err := migrate.Up(ctx, db.URL); err != nil {
				t.Fatal(err)
			}
			var armed bool
			var onlineSince *time.Time
			if err := db.Pool.QueryRow(ctx, `SELECT offline_alert_armed,online_since,now() FROM devices WHERE merchant_id=$1`, merchantID).
				Scan(&armed, &onlineSince, &after); err != nil {
				t.Fatal(err)
			}
			if want := fixture.alerted == 0; armed != want {
				t.Errorf("armed after upgrade = %t, want %t", armed, want)
			}
			if fixture.wantOnlineSince {
				if onlineSince == nil || onlineSince.Before(before) || onlineSince.After(after) {
					t.Errorf("online_since = %v, want the migration timestamp between %v and %v", onlineSince, before, after)
				}
			} else if onlineSince != nil {
				t.Errorf("online_since = %v, want NULL", onlineSince)
			}

			db.Clock.Set(after.Add(91 * time.Second))
			if count, err := application.Services.Device.DetectOffline(ctx, 50); err != nil || count != fixture.wantAlerts {
				t.Errorf("first offline scan = %d, %v, want %d", count, err, fixture.wantAlerts)
			}
			if count, err := application.Services.Device.DetectOffline(ctx, 50); err != nil || count != 0 {
				t.Errorf("repeated offline scan = %d, %v, want 0", count, err)
			}
			var notices, mails int
			if err := db.Pool.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM notifications WHERE merchant_id=$1 AND kind='device_offline'),
				(SELECT count(*) FROM mail_deliveries WHERE template='device_offline')`, merchantID).Scan(&notices, &mails); err != nil {
				t.Fatal(err)
			}
			if notices != fixture.wantAlerts || mails != fixture.wantAlerts {
				t.Errorf("notification rows=%d mail rows=%d, want %d of each", notices, mails, fixture.wantAlerts)
			}
		})
	}
}
