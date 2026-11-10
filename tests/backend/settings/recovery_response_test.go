package settings

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"testing"

	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_RecoveryResponseConfigurationSnapshot(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "without_secret", true: "with_secret"}[configured], func(t *testing.T) {
			db := testsupport.NewDatabase(t)
			fixture := testsupport.NewHTTPFixture(t, db)
			admin := bootstrapRecoveryAdmin(t, fixture, "snapshot")
			secret := rand.Text()
			if configured {
				value := map[string]any{"turnstile_site_key": "snapshot-site", "turnstile_enabled": true, "turnstile_token": "ok:settings_security", "turnstile_secret": secret, "restrict_email_domains": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "", "en": ""}}
				assertRecoveryAPIError(t, updateSettingsGroupHTTP(t, fixture, admin, "security", value), http.StatusOK, "")
			} else if _, err := db.Pool.Exec(t.Context(), `UPDATE site_settings SET security=jsonb_set(security,'{turnstile_site_key}','"snapshot-site"'),version=version+1 WHERE singleton=true`); err != nil {
				t.Fatal("configure public security fields")
			}
			code, err := fixture.App.Services.Auth.IssueConsoleRecoveryCode(t.Context())
			if err != nil {
				t.Fatal("issue recovery code")
			}
			// HTTPFixture validates this actual response against OpenAPI, including
			// both closed objects. Never print a response containing CSRF on failure.
			response := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]string{"recovery_code": code}, "guest")
			if response.Code != http.StatusOK {
				t.Fatal("recover console")
			}
			var envelope struct {
				Data struct {
					Version  int64  `json:"settings_version"`
					CSRF     string `json:"csrf_token"`
					Security struct {
						SiteKey    string `json:"turnstile_site_key"`
						Enabled    bool   `json:"turnstile_enabled"`
						Configured bool   `json:"turnstile_secret_configured"`
					} `json:"security"`
				} `json:"data"`
			}
			if json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.Data.Version != recoverySettingsVersion(t, fixture) || envelope.Data.Security.SiteKey != "snapshot-site" || envelope.Data.Security.Enabled != configured || envelope.Data.Security.Configured != configured {
				t.Fatal("invalid recovery configuration snapshot")
			}
			if bytes.Contains(response.Body.Bytes(), []byte(secret)) || bytes.Contains(response.Body.Bytes(), []byte(`"turnstile_secret"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"encrypted_value"`)) {
				t.Fatal("recovery response leaked a secret")
			}
			replay := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]string{"recovery_code": code}, "guest")
			if replay.Code != http.StatusUnauthorized {
				t.Fatal("recovery code was reusable")
			}
		})
	}
}
