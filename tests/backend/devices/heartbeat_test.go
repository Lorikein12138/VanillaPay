package devices

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_HeartbeatRestoresOnlineStateAndOfflineScanStartsANewEpisode(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	binding, err := application.Services.Device.NewBinding(context.Background(), actor, false)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := application.Services.Device.Redeem(context.Background(), device.RedeemRequest{Code: binding.Code, DeviceName: "heartbeat device", AppVersion: "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("V41.B.r01/v01", func(t *testing.T) {
		var last *time.Time
		var everOnline bool
		if err := db.Pool.QueryRow(t.Context(), `SELECT last_seen_at,ever_online FROM devices WHERE public_id=$1`, credential.DeviceID).Scan(&last, &everOnline); err != nil || last != nil || everOnline {
			t.Fatalf("newly bound device last_seen=%v ever_online=%t err=%v, want offline before heartbeat", last, everOnline, err)
		}
	})
	identity, err := application.Services.Device.Authenticate(context.Background(), credential.DeviceToken)
	if err != nil {
		t.Fatal(err)
	}
	status, err := application.Services.Device.Heartbeat(context.Background(), identity, device.HeartbeatRequest{AppVersion: "1.0"})
	if err != nil || !status.Online {
		t.Fatalf("first heartbeat = %#v, %v", status, err)
	}
	t.Run("V41.B.r01/v02", func(t *testing.T) {
		var everOnline bool
		var last time.Time
		if err := db.Pool.QueryRow(t.Context(), `SELECT last_seen_at,ever_online FROM devices WHERE public_id=$1`, credential.DeviceID).Scan(&last, &everOnline); err != nil || !everOnline || !last.Equal(status.ServerTime.Truncate(time.Microsecond)) {
			t.Fatalf("first heartbeat persistence last_seen=%s ever_online=%t err=%v", last, everOnline, err)
		}
	})
	db.Clock.Set(db.Clock.Now().Add(91 * time.Second))
	if count, err := application.Services.Device.DetectOffline(context.Background(), 10); err != nil || count != 1 {
		t.Fatalf("offline detection = %d, %v", count, err)
	}
	t.Run("V41.B.r03/v01", func(t *testing.T) {
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_deliveries WHERE template='device_offline' AND dedupe_key LIKE 'device_offline:%'`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("first offline episode mail jobs=%d err=%v, want one", count, err)
		}
	})
	t.Run("V41.B.r03/v02", func(t *testing.T) {
		if count, err := application.Services.Device.DetectOffline(t.Context(), 10); err != nil || count != 0 {
			t.Fatalf("repeated offline scan count=%d err=%v, want no duplicate episode", count, err)
		}
	})
	status, err = application.Services.Device.Heartbeat(context.Background(), identity, device.HeartbeatRequest{AppVersion: "1.1"})
	if err != nil || !status.Online {
		t.Fatalf("recovery heartbeat = %#v, %v", status, err)
	}
	db.Clock.Set(db.Clock.Now().Add(91 * time.Second))
	if count, err := application.Services.Device.DetectOffline(context.Background(), 10); err != nil || count != 1 {
		t.Fatalf("second offline episode = %d, %v", count, err)
	}
	t.Run("V41.B.r04/v01", func(t *testing.T) {
		var episodes, jobs int
		if err := db.Pool.QueryRow(t.Context(), `SELECT offline_notified_episode FROM devices WHERE id=$1`, identity.DeviceID).Scan(&episodes); err != nil {
			t.Fatal(err)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_deliveries WHERE template='device_offline' AND dedupe_key LIKE 'device_offline:%'`).Scan(&jobs); err != nil || episodes != 2 || jobs != 2 {
			t.Fatalf("restored second offline episode=%d mail jobs=%d err=%v, want 2/2", episodes, jobs, err)
		}
	})
	db.Clock.Set(db.Clock.Now().Add(10 * time.Minute))
	if count, err := application.Services.Device.DetectOffline(context.Background(), 10); err != nil || count != 0 {
		t.Fatalf("long offline repeated notification = %d, %v", count, err)
	}
	var episodes, notifications int
	if err := db.Pool.QueryRow(context.Background(), `SELECT offline_notified_episode FROM devices WHERE id=$1`, identity.DeviceID).Scan(&episodes); err != nil {
		t.Fatal(err)
	}
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE merchant_id=$1 AND kind='device_offline'`, merchantID).Scan(&notifications); err != nil {
		t.Fatal(err)
	}
	if episodes != 2 || notifications != 2 {
		t.Fatalf("offline episodes=%d notifications=%d", episodes, notifications)
	}
}

func TestDB_DeviceWithoutFirstHeartbeatDoesNotTriggerOfflineNotice(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	merchant := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	binding, err := app.Services.Device.NewBinding(t.Context(), merchant, false)
	if err != nil {
		t.Fatalf("issue device binding: %v", err)
	}
	credential, err := app.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: binding.Code, DeviceName: "never online", AppVersion: "1"})
	if err != nil {
		t.Fatalf("redeem device before first heartbeat: %v", err)
	}
	t.Run("V41.B.r05/v01", func(t *testing.T) {
		db.Clock.Set(db.Clock.Now().Add(10 * time.Minute))
		count, err := app.Services.Device.DetectOffline(t.Context(), 10)
		if err != nil || count != 0 {
			t.Fatalf("offline scan before first heartbeat count=%d err=%v", count, err)
		}
		var jobs int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_deliveries WHERE template='device_offline' AND dedupe_key LIKE 'device_offline:%'`).Scan(&jobs); err != nil || jobs != 0 {
			t.Fatalf("device that never came online produced %d offline mail jobs err=%v", jobs, err)
		}
		if _, err := app.Services.Device.Authenticate(t.Context(), credential.DeviceToken); err != nil {
			t.Fatalf("initial device credential unexpectedly invalid: %v", err)
		}
	})
}

func TestDB_DeviceOnlineThresholdControlsOrderEligibility(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "device online threshold", 1000, 3650, []string{"alipay"}, nil)
	app := db.NewApplication(t)
	db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
	var lastSeen time.Time
	if err := db.Pool.QueryRow(t.Context(), `SELECT last_seen_at FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL`, merchantID).Scan(&lastSeen); err != nil {
		t.Fatalf("read billing device server heartbeat time: %v", err)
	}
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	t.Run("V41.B.r02/v01", func(t *testing.T) {
		db.Clock.Set(lastSeen.Add(89*time.Second + 999*time.Millisecond))
		order, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "heartbeat-89-999",
			Name: "inside online threshold", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.91", Actor: actor,
		})
		if err != nil || order.TradeNo == "" {
			t.Fatalf("order at heartbeat +89.999s was rejected: %#v err=%v", order, err)
		}
	})
	t.Run("V41.B.r02/v02", func(t *testing.T) {
		db.Clock.Set(lastSeen.Add(90 * time.Second))
		_, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "heartbeat-90-000",
			Name: "at offline threshold", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.92", Actor: actor,
		})
		if err == nil {
			t.Fatal("order at heartbeat +90s was accepted")
		}
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no='heartbeat-90-000'`, merchantID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("offline-boundary rejection left %d orders err=%v", count, err)
		}
	})
}

func TestHTTP_ClientClockCannotExtendDeviceOnlineWindow(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	merchant := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	binding, err := fixture.App.Services.Device.NewBinding(t.Context(), merchant, false)
	if err != nil {
		t.Fatalf("issue binding before client-clock request: %v", err)
	}
	credential, err := fixture.App.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: binding.Code, DeviceName: "clock-boundary", AppVersion: "1"})
	if err != nil {
		t.Fatalf("redeem binding before client-clock request: %v", err)
	}
	response := fixture.Request(t, "POST", "/api/v1/device/heartbeat", map[string]any{"app_version": "1"}, "device:"+credential.DeviceToken)
	if response.Code != 200 {
		t.Fatalf("server-time heartbeat status=%d body=%s", response.Code, response.Body.String())
	}
	var lastSeen time.Time
	if err := db.Pool.QueryRow(t.Context(), `SELECT last_seen_at FROM devices WHERE public_id=$1`, credential.DeviceID).Scan(&lastSeen); err != nil {
		t.Fatal("read server-authored heartbeat time")
	}
	t.Run("V41.B.r04/v02", func(t *testing.T) {
		db.Clock.Set(lastSeen.Add(89*time.Second + 999*time.Millisecond))
		clientRollback := fixture.Request(t, "POST", "/api/v1/device/heartbeat", map[string]any{"app_version": "1", "client_time": "1970-01-01T00:00:00Z"}, "device:"+credential.DeviceToken)
		var errorEnvelope struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if clientRollback.Code != 400 || json.Unmarshal(clientRollback.Body.Bytes(), &errorEnvelope) != nil || errorEnvelope.Error == nil || errorEnvelope.Error.Code != "VALIDATION_ERROR" {
			t.Fatalf("uncontracted client clock request status=%d body=%s", clientRollback.Code, clientRollback.Body.String())
		}
		deviceDTO, err := fixture.App.Services.Device.Device(t.Context(), merchant)
		if err != nil || !deviceDTO.Online || deviceDTO.LastSeenAt == nil || !deviceDTO.LastSeenAt.Equal(lastSeen) {
			t.Fatalf("client clock modified server heartbeat window: device=%#v err=%v", deviceDTO, err)
		}
		db.Clock.Set(lastSeen.Add(90 * time.Second))
		deviceDTO, err = fixture.App.Services.Device.Device(t.Context(), merchant)
		if err != nil || deviceDTO.Online {
			t.Fatalf("client timestamp rollback extended online status to the 90s boundary: device=%#v err=%v", deviceDTO, err)
		}
	})
}
