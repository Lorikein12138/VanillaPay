package settings

import (
	"context"
	"encoding/json"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_LegalUpdatesCreateImmutableTermsAndPrivacyRevisions(t *testing.T) {
	db := testsupport.NewDatabase(t)
	application := db.NewApplication(t)
	value := json.RawMessage(`{"items":[],"terms":{"zh_CN":"条款一","en":""},"privacy":{"zh_CN":"隐私一","en":""}}`)
	actor := platform.Actor{Role: "admin", SubjectID: 1}
	if _, err := application.Services.Settings.UpdateGroup(context.Background(), actor, "footer_legal", settings.UpdateGroup{ExpectedVersion: 1, Value: value}); err != nil {
		t.Fatal(err)
	}
	value = json.RawMessage(`{"items":[],"terms":{"zh_CN":"条款二","en":"Terms two"},"privacy":{"zh_CN":"隐私二","en":"Privacy two"}}`)
	if _, err := application.Services.Settings.UpdateGroup(context.Background(), actor, "footer_legal", settings.UpdateGroup{ExpectedVersion: 2, Value: value}); err != nil {
		t.Fatal(err)
	}
	var terms, privacy int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE kind='terms'),count(*) FILTER (WHERE kind='privacy') FROM legal_versions`).Scan(&terms, &privacy); err != nil {
		t.Fatal(err)
	}
	if terms != 2 || privacy != 2 {
		t.Fatalf("legal revision counts terms=%d privacy=%d", terms, privacy)
	}
}
