package settings

import (
	"encoding/json"
	"net/http"
	"testing"

	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_TurnstileAndSMTPFailures(t *testing.T) {
	t.Run("V18.B.r01/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		admin := bootstrapRecoveryAdmin(t, fixture, "bootstrap-repair")
		response := updateSettingsGroupHTTP(t, fixture, admin, "security", map[string]any{
			"turnstile_site_key": "bootstrap-site-key", "turnstile_secret": "bootstrap-site-secret", "turnstile_enabled": true, "turnstile_token": "ok:settings_security",
			"restrict_email_domains": false, "registration_review": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "help@example.test", "en": "help@example.test"},
		})
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		fixture.Siteverify.SetMode("accept")
		login := fixture.Request(t, http.MethodPost, "/api/v1/console/login", map[string]any{
			"identifier": "recovery_fixture_admin", "password": fixturePassword, "turnstile_token": "ok:login",
		}, "guest")
		if login.Code != http.StatusOK {
			t.Fatalf("bootstrap-created console administrator could not log in after Turnstile repair: status=%d body=%s", login.Code, login.Body.String())
		}
	})

	t.Run("V18.B.r04/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		response := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"identifier": "nobody", "password": fixtureWrongPassword,
			"turnstile_token": "ok:login",
		}, "guest")
		assertRecoveryAPIError(t, response, http.StatusUnauthorized, "INVALID_CREDENTIALS")
	})

	t.Run("V18.B.r05/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		admin := bootstrapRecoveryAdmin(t, fixture, "smtp-failure")
		response := fixture.Request(t, http.MethodPost, "/api/v1/console/settings/mail/test", map[string]any{"recipient": "ops@example.test"}, admin)
		if response.Code != http.StatusAccepted {
			t.Fatalf("mail test queue status=%d body=%s", response.Code, response.Body.String())
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal("decode mail test queue response")
		}
		id, _ := envelope.Data["delivery_id"].(string)
		if id == "" || envelope.Data["state"] == "sent" {
			t.Fatalf("mail test reported delivery success without SMTP delivery: %#v", envelope.Data)
		}
		var state string
		if err := db.Pool.QueryRow(t.Context(), `SELECT state FROM mail_deliveries WHERE public_id=$1 AND template='mail_test'`, id).Scan(&state); err != nil || state != "queued" {
			t.Fatalf("SMTP failure fixture delivery state=%q err=%v, want queued (never falsely sent)", state, err)
		}
	})
}
