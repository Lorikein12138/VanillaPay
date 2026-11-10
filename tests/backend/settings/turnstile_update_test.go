package settings

import (
	"net/http"
	"testing"

	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_TurnstileSettingsProofUsesProposedCredentials(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	admin := bootstrapRecoveryAdmin(t, fixture, "turnstile-proof")
	value := map[string]any{"turnstile_enabled": true, "turnstile_site_key": "proposed-widget", "restrict_email_domains": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "", "en": ""}}
	save := func(status int, code string) {
		t.Helper()
		assertRecoveryAPIError(t, updateSettingsGroupHTTP(t, fixture, admin, "security", value), status, code)
	}
	version := recoverySettingsVersion(t, fixture)
	save(http.StatusBadRequest, "VALIDATION_ERROR")
	value["turnstile_secret"] = "fixture-first-secret"
	save(http.StatusBadRequest, "TURNSTILE_FAILED")
	value["turnstile_token"] = "rejected"
	save(http.StatusBadRequest, "TURNSTILE_FAILED")
	if recoverySettingsVersion(t, fixture) != version {
		t.Fatal("rejected enable changed settings")
	}
	value["turnstile_token"] = "ok:settings_security"
	save(http.StatusOK, "")
	version = recoverySettingsVersion(t, fixture)
	value["turnstile_secret"] = "fixture-next-secret"
	delete(value, "turnstile_token")
	save(http.StatusBadRequest, "TURNSTILE_FAILED")
	value["turnstile_token"] = "ok:settings_security"
	fixture.Siteverify.SetMode("wrong-host")
	save(http.StatusBadRequest, "TURNSTILE_FAILED")
	if recoverySettingsVersion(t, fixture) != version {
		t.Fatal("rejected rotation changed settings")
	}
	if secret, err := fixture.App.Services.Settings.Secret(t.Context(), "turnstile_secret"); err != nil || secret != "fixture-first-secret" {
		t.Fatal("rejected rotation changed the stored secret")
	}
	requests := fixture.Siteverify.Requests()
	if len(requests) == 0 || requests[len(requests)-1].Get("secret") != "fixture-next-secret" {
		t.Fatal("rotation proof did not use proposed secret")
	}
	fixture.Siteverify.SetMode("accept")
	save(http.StatusOK, "")
	value["turnstile_site_key"] = "other-widget"
	delete(value, "turnstile_secret")
	delete(value, "turnstile_token")
	save(http.StatusBadRequest, "TURNSTILE_FAILED")
	value["turnstile_enabled"] = false
	value["turnstile_secret"] = nil
	save(http.StatusOK, "")
}
