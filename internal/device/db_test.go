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
	for index, gap := range []time.Duration{90 * time.Second, 30 * time.Second, 30 * time.Second} {
		db.Clock.Set(db.Clock.Now().Add(gap))
		status, err := application.Services.Device.Heartbeat(ctx, identity, device.HeartbeatRequest{})
		want := device.HealthAbnormal
		if index == 2 {
			want = device.HealthOnline
		}
		if err != nil || !status.Online || status.HealthStatus != want {
			t.Fatalf("heartbeat after abnormal gap %d = %#v, %v, want %s", index, status, err, want)
		}
	}
	db.Clock.Set(db.Clock.Now().Add(300 * time.Second))
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
	heartbeatResult := make(chan error, 1)
	go func() {
		<-start
		status, err := application.Services.Device.Heartbeat(ctx, identity, device.HeartbeatRequest{})
		if err == nil && (!status.Online || status.HealthStatus != device.HealthAbnormal) {
			t.Errorf("heartbeat racing offline scan = %#v", status)
		}
		heartbeatResult <- err
	}()
	close(start)
	total := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Errorf("concurrent offline scan: %v", result.err)
		}
		total += result.count
	}
	if err := <-heartbeatResult; err != nil {
		t.Fatalf("heartbeat racing offline scan: %v", err)
	}
	if total > 1 {
		t.Fatalf("concurrent offline scans queued %d alerts, want at most 1", total)
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

func TestDB_HeartbeatRecoveryMigrationPreservesLegacyEpisodes(t *testing.T) {
	db := testsupport.NewDatabase(t)
	ctx := t.Context()
	legacyDB, err := sql.Open("pgx", db.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = legacyDB.Close() }()
	provider, err := goose.NewProvider(database.DialectPostgres, legacyDB, migrations.FS())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 12); err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name             string
		age, oldRun      time.Duration
		armed, wantArmed bool
		wantCount        int16
		wantEpisode      int
	}{
		{"recent-armed", 20 * time.Second, time.Minute, true, true, 3, 8},
		{"recent-effective", 20 * time.Second, 211 * time.Second, false, true, 3, 8},
		{"recent-unstable", 20 * time.Second, 2 * time.Minute, false, false, 0, 7},
		{"older-effective", 6 * time.Minute, 211 * time.Second, false, true, 0, 8},
		{"older-formula-boundary", 6 * time.Minute, 210 * time.Second, false, false, 0, 7},
	}
	var now time.Time
	if err := db.Pool.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		merchantID, _ := db.SeedMerchant(t)
		last := now.Add(-fixture.age)
		if _, err := db.Pool.Exec(ctx, `INSERT INTO devices
			(public_id,merchant_id,token_hash,name,last_seen_at,ever_online,online_since,offline_alert_armed,offline_notified_episode)
			VALUES($1,$2,$3,$1,$4,true,$5,$6,7)`, fixture.name, merchantID, []byte(fixture.name), last, last.Add(-fixture.oldRun), fixture.armed); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrate.Up(ctx, db.URL); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		var count int16
		var armed bool
		var episode int
		err := db.Pool.QueryRow(ctx, `SELECT recovery_count,offline_alert_armed,offline_notified_episode
			FROM devices WHERE public_id=$1`, fixture.name).Scan(&count, &armed, &episode)
		if err != nil || count != fixture.wantCount || armed != fixture.wantArmed || episode != 7 {
			t.Fatalf("%s migrated count=%d armed=%t episode=%d err=%v", fixture.name, count, armed, episode, err)
		}
	}
	service := db.NewApplication(t).Services.Device
	db.Clock.Set(now)
	if count, err := service.DetectOffline(ctx, 10); err != nil || count != 1 {
		t.Fatalf("initial migrated scan = %d, %v", count, err)
	}
	db.Clock.Set(now.Add(300 * time.Second))
	if count, err := service.DetectOffline(ctx, 10); err != nil || count != 2 {
		t.Fatalf("later migrated scan = %d, %v", count, err)
	}
	for _, fixture := range fixtures {
		var episode int
		err := db.Pool.QueryRow(ctx, `SELECT offline_notified_episode FROM devices WHERE public_id=$1`, fixture.name).Scan(&episode)
		if err != nil || episode != fixture.wantEpisode {
			t.Fatalf("%s scanned episode=%d err=%v, want %d", fixture.name, episode, err, fixture.wantEpisode)
		}
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
			if _, err := provider.DownTo(ctx, 7); err != nil {
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

			db.Clock.Set(after.Add(300 * time.Second))
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

func TestDB_OfflineEmailPreferenceAppliesAtFirstScan(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	services := db.NewApplication(t).Services
	ctx := t.Context()
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	binding, err := services.Device.NewBinding(ctx, actor, false)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := services.Device.Redeem(ctx, device.RedeemRequest{Code: binding.Code, DeviceName: "offline email device"})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := services.Device.Authenticate(ctx, credential.DeviceToken)
	if err != nil {
		t.Fatal(err)
	}
	heartbeat := func() {
		t.Helper()
		if _, err := services.Device.Heartbeat(ctx, identity, device.HeartbeatRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	rearm := func() {
		heartbeat()
		for range 2 {
			db.Clock.Set(db.Clock.Now().Add(30 * time.Second))
			heartbeat()
		}
	}
	setEmail := func(enabled bool) {
		t.Helper()
		if _, err := services.Auth.UpdatePreferences(ctx, actor, "", "", &enabled); err != nil {
			t.Fatal(err)
		}
	}
	crossOfflineThreshold := func() {
		t.Helper()
		db.Clock.Set(db.Clock.Now().Add(300 * time.Second))
		status, err := services.Device.Device(ctx, actor)
		if err != nil || !status.Bound || status.Online || status.HealthStatus != device.HealthOffline {
			t.Fatalf("expected bound device to be offline before first scan, bound=%t online=%t err=%v", status.Bound, status.Online, err)
		}
	}
	checkScan := func(wantAlerts, wantNotices, wantMails int) {
		t.Helper()
		count, err := services.Device.DetectOffline(ctx, 10)
		if err != nil || count != wantAlerts {
			t.Fatalf("offline scan = %d, %v, want %d", count, err, wantAlerts)
		}
		var notices, mails int
		if err := db.Pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM notifications WHERE merchant_id=$1 AND kind='device_offline'),
			(SELECT count(*) FROM mail_deliveries WHERE template='device_offline')`, merchantID).Scan(&notices, &mails); err != nil {
			t.Fatal(err)
		}
		if notices != wantNotices || mails != wantMails {
			t.Fatalf("offline notices=%d mails=%d, want %d/%d", notices, mails, wantNotices, wantMails)
		}
	}

	heartbeat()
	crossOfflineThreshold()
	checkScan(1, 1, 1)
	rearm()
	crossOfflineThreshold()
	setEmail(false)
	checkScan(1, 2, 1)
	setEmail(true)
	checkScan(0, 2, 1)
	setEmail(false)
	rearm()
	crossOfflineThreshold()
	setEmail(true)
	checkScan(1, 3, 2)
}
