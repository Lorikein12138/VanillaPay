package settings

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"vanillapay/internal/device"
	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_MediaAndDeviceSlotsDoNotExpandWithPlan(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	firstPlan := db.SeedPlan(t, "resource-limit-basic", 1000, 30, []string{"alipay"}, nil)
	secondPlan := db.SeedPlan(t, "resource-limit-premium", 10000, 365, []string{"alipay", "wxpay", "qqpay"}, nil)
	application := db.NewApplication(t)
	merchant := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	t.Run("V23.B.r03/v01", func(t *testing.T) {
		if _, err := application.Services.Subscription.Grant(t.Context(), admin, merchantID, subscription.GrantRequest{
			PlanID: strconv.FormatInt(firstPlan, 10), ExpiresAt: db.Clock.Now().Add(30 * 24 * time.Hour), Channels: []string{"alipay"}, ExpectedRevision: 0, Reason: "limit basic fixture",
		}); err != nil {
			t.Fatalf("grant basic plan: %v", err)
		}
		for _, item := range []struct{ channel, payload string }{
			{"alipay", "https://qr.alipay.com/slot-a"}, {"wxpay", "wxp://slot?source=qr"}, {"qqpay", "https://i.qianbao.qq.com/slot-c"},
		} {
			if _, err := application.Services.Settings.UploadQR(t.Context(), merchant, item.channel, bytes.NewReader(qrPNG(t, item.payload))); err != nil {
				t.Fatalf("upload %s QR slot: %v", item.channel, err)
			}
		}
		if _, err := application.Services.Settings.UploadQR(t.Context(), merchant, "alipay", bytes.NewReader(qrPNG(t, "https://qr.alipay.com/slot-replacement"))); err != nil {
			t.Fatalf("replace existing Alipay slot instead of allocating a fourth: %v", err)
		}
		if _, err := application.Services.Subscription.Grant(t.Context(), admin, merchantID, subscription.GrantRequest{
			PlanID: strconv.FormatInt(secondPlan, 10), ExpiresAt: db.Clock.Now().Add(365 * 24 * time.Hour), Channels: []string{"alipay", "wxpay", "qqpay"}, ExpectedRevision: 1, Reason: "limit premium fixture",
		}); err != nil {
			t.Fatalf("switch merchant to a plan with all channels: %v", err)
		}
		binding, err := application.Services.Device.NewBinding(t.Context(), merchant, false)
		if err != nil {
			t.Fatalf("create first device binding: %v", err)
		}
		credential, err := application.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: binding.Code, DeviceName: "first", AppVersion: "1"})
		if err != nil || credential.DeviceToken == "" {
			t.Fatalf("redeem first binding: credential=%#v err=%v", credential, err)
		}
		if _, err := application.Services.Device.NewBinding(t.Context(), merchant, false); !resourceLimitError(err, "DEVICE_ALREADY_BOUND") {
			t.Fatalf("premium plan expanded one-device limit: second binding error=%v", err)
		}
		var activeQR, activeDevices int
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM media_assets WHERE owner_merchant_id=$1 AND kind='qr' AND active=true`, merchantID).Scan(&activeQR); err != nil {
			t.Fatal("count active QR slots")
		}
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL`, merchantID).Scan(&activeDevices); err != nil {
			t.Fatal("count active merchant devices")
		}
		if activeQR != 3 || activeDevices != 1 {
			t.Fatalf("plan changed resource cap: active QR=%d active devices=%d, want 3/1", activeQR, activeDevices)
		}
		t.Run("V40.B.r06/v01", func(t *testing.T) {
			_, secondBindingErr := application.Services.Device.NewBinding(t.Context(), merchant, false)
			if activeDevices != 1 || !resourceLimitError(secondBindingErr, "DEVICE_ALREADY_BOUND") {
				t.Fatalf("one active device limit=%d, second binding error=%v, want one device and DEVICE_ALREADY_BOUND", activeDevices, secondBindingErr)
			}
		})
	})
}

func resourceLimitError(err error, code string) bool {
	var apiErr *platform.Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}
