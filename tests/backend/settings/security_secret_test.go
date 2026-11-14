package settings

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_SecuritySecretOmissionAndEmptyValue(t *testing.T) {
	for _, scenario := range []string{"admin_omitted", "admin_empty", "recovery_empty"} {
		t.Run(scenario, func(t *testing.T) {
			db := testsupport.NewDatabase(t)
			fixture := testsupport.NewHTTPFixture(t, db)
			admin := bootstrapRecoveryAdmin(t, fixture, scenario)
			secret, err := platform.RandomToken(rand.Reader, 32)
			if err != nil {
				t.Fatal("generate security fixture secret")
			}
			value := map[string]any{
				"turnstile_site_key": "fixture-site", "turnstile_secret": secret, "turnstile_enabled": false,
				"restrict_email_domains": false, "registration_review": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "", "en": ""},
			}
			assertRecoveryAPIError(t, updateSettingsGroupHTTP(t, fixture, admin, "security", value), http.StatusOK, "")
			var beforeCipher []byte
			var beforeVersion int64
			var beforeUpdated time.Time
			if err := db.Pool.QueryRow(t.Context(), `SELECT encrypted_value,version,updated_at FROM secret_settings WHERE name='turnstile_secret'`).Scan(&beforeCipher, &beforeVersion, &beforeUpdated); err != nil {
				t.Fatal("read initial secret snapshot")
			}
			siteVersion := recoverySettingsVersion(t, fixture)
			value["turnstile_site_key"] = "changed-site"
			if scenario == "admin_omitted" {
				delete(value, "turnstile_secret")
			} else {
				value["turnstile_secret"] = ""
			}
			role := admin
			headers := http.Header{"Content-Type": {"application/json"}}
			if scenario == "recovery_empty" {
				code, err := fixture.App.Services.Auth.IssueConsoleRecoveryCode(t.Context())
				if err != nil {
					t.Fatal("issue recovery code")
				}
				recovered := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]any{"recovery_code": code}, "guest")
				assertRecoveryAPIError(t, recovered, http.StatusOK, "")
				headers = recoveryHeaders(findRecoveryCookie(t, recovered.Header().Values("Set-Cookie")), responseCSRF(t, recovered.Body.Bytes()))
				role = "guest"
				value = map[string]any{"turnstile_site_key": "changed-site", "turnstile_enabled": false, "turnstile_secret": ""}
			}
			payload, err := json.Marshal(map[string]any{"expected_version": siteVersion, "value": value})
			if err != nil {
				t.Fatal("encode security update")
			}
			response := fixture.RequestWithHeaders(t, http.MethodPut, "/api/v1/console/settings/security", bytes.NewReader(payload), role, headers)
			wantStatus, wantCode := http.StatusBadRequest, "VALIDATION_ERROR"
			if scenario == "admin_omitted" {
				wantStatus, wantCode = http.StatusOK, ""
			}
			assertRecoveryAPIError(t, response, wantStatus, wantCode)
			if bytes.Contains(response.Body.Bytes(), []byte(secret)) || bytes.Contains(response.Body.Bytes(), []byte(`"turnstile_secret"`)) {
				t.Fatal("security response exposed the private secret")
			}
			var afterCipher []byte
			var afterVersion int64
			var afterUpdated time.Time
			if err := db.Pool.QueryRow(t.Context(), `SELECT encrypted_value,version,updated_at FROM secret_settings WHERE name='turnstile_secret'`).Scan(&afterCipher, &afterVersion, &afterUpdated); err != nil {
				t.Fatal("read final secret snapshot")
			}
			if !bytes.Equal(beforeCipher, afterCipher) || beforeVersion != afterVersion || !beforeUpdated.Equal(afterUpdated) {
				t.Fatal("omitted or rejected secret was rewritten")
			}
			stored, err := fixture.App.Services.Settings.Secret(t.Context(), "turnstile_secret")
			if err != nil || stored != secret {
				t.Fatal("stored secret changed")
			}
			wantVersion := siteVersion
			if scenario == "admin_omitted" {
				wantVersion++
			}
			if recoverySettingsVersion(t, fixture) != wantVersion {
				t.Fatal("unexpected settings version after security update")
			}
		})
	}
}
