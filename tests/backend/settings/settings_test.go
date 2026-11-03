package settings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_CollectionSettingsInheritGlobalBoundsAndRejectOutOfRangeTTL(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	result, err := application.Services.Settings.SaveCollection(ctx, actor, settings.SaveCollectionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Effective.OrderTTLSeconds != 300 || result.Effective.FloatDirection != "down" {
		t.Fatalf("inherited collection values = %#v", result.Effective)
	}
	tooLong := 3600
	_, err = application.Services.Settings.SaveCollection(ctx, actor, settings.SaveCollectionRequest{OrderTTLSeconds: &tooLong})
	if !hasErrorCode(err, "TTL_OUT_OF_RANGE") {
		t.Fatalf("out-of-range TTL error = %v", err)
	}
}

func TestDB_SettingsGroupsUseOptimisticVersion(t *testing.T) {
	db := testsupport.NewDatabase(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	value := json.RawMessage(`{"site_name":"Updated site","logo_asset_id":null}`)
	request := settings.UpdateGroup{ExpectedVersion: 1, Value: value}
	result, err := application.Services.Settings.UpdateGroup(ctx, platform.Actor{Role: "admin", SubjectID: 1}, "site", request)
	if err != nil || result.Version != 2 {
		t.Fatalf("settings update = %#v, %v", result, err)
	}
	if _, err := application.Services.Settings.UpdateGroup(ctx, platform.Actor{Role: "admin", SubjectID: 1}, "site", request); !hasErrorCode(err, "VERSION_CONFLICT") {
		t.Fatalf("stale version error = %v", err)
	}
}

func hasErrorCode(err error, code string) bool {
	var apiErr *platform.Error
	return errors.As(err, &apiErr) && apiErr.Code == code
}
