package device_test

import (
	"context"
	"errors"
	"testing"
	"time"

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
	if count, err := application.Services.Device.DetectOffline(ctx, 50); err != nil || count != 1 {
		t.Fatalf("first offline scan = %d, %v", count, err)
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
