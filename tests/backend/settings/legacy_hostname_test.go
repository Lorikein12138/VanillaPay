package settings

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"vanillapay/tests/backend/testsupport"
)

// An image rollback runs the previous binary against the v7 schema and that
// binary still reads security.allowed_hostname, so saves must carry it over
// while neither the console nor the recovery response exposes it.
func TestHTTP_SecuritySavesKeepLegacyAllowedHostname(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	admin := bootstrapRecoveryAdmin(t, fixture, "legacy-hostname")
	if _, err := db.Pool.Exec(t.Context(), `UPDATE site_settings SET security=security || '{"allowed_hostname":"old.example.test"}'::jsonb`); err != nil {
		t.Fatal("seed legacy hostname")
	}
	assertLegacy := func(t *testing.T, step string) {
		t.Helper()
		var hostname string
		if err := db.Pool.QueryRow(t.Context(), `SELECT security->>'allowed_hostname' FROM site_settings`).Scan(&hostname); err != nil || hostname != "old.example.test" {
			t.Fatalf("%s: stored allowed_hostname=%q err=%v", step, hostname, err)
		}
	}
	if groups := readConsoleSettingsGroups(t, fixture, admin); strings.Contains(string(groups["security"]), "allowed_hostname") {
		t.Fatalf("console settings exposed legacy key: %s", groups["security"])
	}

	saved := updateSettingsGroupHTTP(t, fixture, admin, "security", map[string]any{
		"turnstile_site_key": "widget", "turnstile_secret": "legacy-secret", "turnstile_enabled": true, "turnstile_token": "ok:settings_security",
		"restrict_email_domains": false, "registration_review": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "", "en": ""},
	})
	assertRecoveryAPIError(t, saved, http.StatusOK, "")
	if strings.Contains(saved.Body.String(), "allowed_hostname") {
		t.Fatal("security save response exposed legacy key")
	}
	assertLegacy(t, "console save")

	code, err := fixture.App.Services.Auth.IssueConsoleRecoveryCode(t.Context())
	if err != nil {
		t.Fatal("issue recovery code")
	}
	recovered := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]any{"recovery_code": code}, "guest")
	if recovered.Code != http.StatusOK || strings.Contains(recovered.Body.String(), "allowed_hostname") {
		t.Fatalf("recovery session status=%d body=%s", recovered.Code, recovered.Body.String())
	}
	payload, err := json.Marshal(map[string]any{"expected_version": recoverySettingsVersion(t, fixture), "value": map[string]any{
		"turnstile_site_key": "widget", "turnstile_enabled": false,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cookie := findRecoveryCookie(t, recovered.Header().Values("Set-Cookie"))
	response := fixture.RequestWithHeaders(t, http.MethodPut, "/api/v1/console/settings/security", strings.NewReader(string(payload)), "guest", recoveryHeaders(cookie, responseCSRF(t, recovered.Body.Bytes())))
	assertRecoveryAPIError(t, response, http.StatusOK, "")
	if strings.Contains(response.Body.String(), "allowed_hostname") {
		t.Fatal("recovery save response exposed legacy key")
	}
	assertLegacy(t, "recovery save")
}
