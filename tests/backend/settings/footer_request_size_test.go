package settings

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"vanillapay/internal/db/migrations"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

const footerSettingsPath = "/api/v1/console/settings/footer_legal"

func TestHTTP_MigratedFooterAtPreviousRequestLimitCanBeSaved(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	admin := bootstrapRecoveryAdmin(t, fixture, "footer-migrated-size")
	version := recoverySettingsVersion(t, fixture)
	const legacyText = "旧版说明"
	var legacy settings.UpdateGroup
	if err := json.Unmarshal(footerRequestAtSize(t, version, legacyText, 300<<10), &legacy); err != nil {
		t.Fatal(err)
	}

	migrationDB, err := sql.Open("pgx", db.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrationDB.Close() }()
	provider, err := goose.NewProvider(database.DialectPostgres, migrationDB, migrations.FS())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if _, err := provider.DownTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE site_settings SET footer_legal=$1::jsonb WHERE singleton`, legacy.Value); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 9); err != nil {
		t.Fatal(err)
	}

	groups := readConsoleSettingsGroups(t, fixture, admin)
	var currentVersion int64
	if err := json.Unmarshal(groups["version"], &currentVersion); err != nil || currentVersion != version {
		t.Fatalf("migrated settings version=%d, want %d, err=%v", currentVersion, version, err)
	}
	var footer settings.FooterLegal
	if err := json.Unmarshal(groups["footer_legal"], &footer); err != nil || footer.Items == nil || len(*footer.Items) != 1 {
		t.Fatalf("expected one migrated footer item, err=%v", err)
	}
	item := &(*footer.Items)[0]
	if item.Text != (settings.LanguageText{ZhCN: legacyText}) || !item.Enabled {
		t.Fatalf("unexpected migrated footer item: %+v", item)
	}
	item.Enabled = false
	body, err := json.Marshal(map[string]any{"expected_version": currentVersion, "value": footer})
	if err != nil || len(body) != (300<<10)+19 { // 18 bytes for the bilingual object, 1 for false.
		t.Fatalf("migrated PUT size=%d, want %d, err=%v", len(body), (300<<10)+19, err)
	}
	response := fixture.Request(t, http.MethodPut, footerSettingsPath, json.RawMessage(body), admin)
	assertRecoveryAPIError(t, response, http.StatusOK, "")
}

func TestHTTP_SettingsGroupRequestSizeLimit(t *testing.T) {
	fixture := testsupport.NewHTTPFixture(t, testsupport.NewDatabase(t))
	admin := bootstrapRecoveryAdmin(t, fixture, "footer-size-limit")
	const limit = 301 << 10
	for _, tc := range []struct {
		name   string
		size   int
		status int
		code   string
	}{
		{"at limit", limit, http.StatusOK, ""},
		{"one byte over", limit + 1, http.StatusBadRequest, "VALIDATION_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version := recoverySettingsVersion(t, fixture)
			body := footerRequestAtSize(t, version, settings.LanguageText{ZhCN: "说明", En: "Description"}, tc.size)
			response := fixture.Request(t, http.MethodPut, footerSettingsPath, body, admin)
			assertRecoveryAPIError(t, response, tc.status, tc.code)
		})
	}
}

func footerRequestAtSize(t *testing.T, version int64, text any, size int) json.RawMessage {
	t.Helper()
	const legalLimit = 128 << 10
	privacy := &settings.LanguageText{}
	request := map[string]any{"expected_version": version, "value": map[string]any{
		"items": []map[string]any{{
			"id": "help", "label": settings.LanguageText{ZhCN: "帮助", En: "Help"},
			"text": text, "url": nil, "enabled": true, "sort": 0,
		}},
		"terms":   settings.LanguageText{ZhCN: strings.Repeat("a", legalLimit), En: strings.Repeat("b", legalLimit)},
		"privacy": privacy,
	}}
	base, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	remaining := size - len(base)
	if remaining <= 0 || remaining > legalLimit {
		t.Fatalf("privacy padding %d would violate the field limit", remaining)
	}
	privacy.ZhCN = strings.Repeat("c", remaining)
	body, err := json.Marshal(request)
	if err != nil || len(body) != size {
		t.Fatalf("request size=%d, want %d, err=%v", len(body), size, err)
	}
	return body
}
