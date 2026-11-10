package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_ConsoleRecoveryRestrictions(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)

	t.Run("V18.B.r01/v01", func(t *testing.T) {
		unconfigured := fixture.Request(t, http.MethodPost, "/api/v1/console/login", map[string]any{
			"identifier": "missing-turnstile-admin", "password": fixturePassword, "turnstile_token": "ok:login",
		}, "guest")
		assertRecoveryAPIError(t, unconfigured, http.StatusUnauthorized, "INVALID_CREDENTIALS")

		adminSession := bootstrapRecoveryAdmin(t, fixture, "recovery-bootstrap")
		version := recoverySettingsVersion(t, fixture)
		configured := fixture.Request(t, http.MethodPut, "/api/v1/console/settings/security", map[string]any{
			"expected_version": version,
			"value": map[string]any{
				"turnstile_site_key": "recovery-test-site-key", "turnstile_secret": "recovery-test-secret", "turnstile_enabled": true, "turnstile_token": "ok:settings_security",
				"restrict_email_domains": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "help@example.test", "en": "help@example.test"},
			},
		}, adminSession)
		assertRecoveryAPIError(t, configured, http.StatusOK, "")
		fixture.Siteverify.SetMode("accept")
		postConfigLogin := fixture.Request(t, http.MethodPost, "/api/v1/console/login", map[string]any{
			"identifier": "missing-turnstile-admin", "password": fixturePassword, "turnstile_token": "ok:login",
		}, "guest")
		assertRecoveryAPIError(t, postConfigLogin, http.StatusUnauthorized, "INVALID_CREDENTIALS")
	})

	t.Run("V18.B.r02/v01", func(t *testing.T) {
		code, err := fixture.App.Services.Auth.IssueConsoleRecoveryCode(t.Context())
		if err != nil {
			t.Fatalf("issue one-use terminal recovery code: %v", err)
		}
		recoveryResponse := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]any{"recovery_code": code}, "guest")
		if recoveryResponse.Code != http.StatusOK {
			t.Fatalf("console recovery status=%d body=%s", recoveryResponse.Code, recoveryResponse.Body.String())
		}
		var envelope struct {
			Data struct {
				CSRFToken  string    `json:"csrf_token"`
				ExpiresAt  time.Time `json:"expires_at"`
				RepairOnly bool      `json:"repair_only"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recoveryResponse.Body.Bytes(), &envelope); err != nil || envelope.Data.CSRFToken == "" || !envelope.Data.RepairOnly || !envelope.Data.ExpiresAt.Equal(db.Clock.Now().Add(10*time.Minute)) {
			t.Fatalf("recovery session payload=%#v err=%v body=%s", envelope.Data, err, recoveryResponse.Body.String())
		}
		recoveryCookie := findRecoveryCookie(t, recoveryResponse.Header().Values("Set-Cookie"))
		csrf := envelope.Data.CSRFToken

		for _, attempt := range []struct{ method, path string }{
			{http.MethodGet, "/api/v1/console/settings"},
			{http.MethodGet, "/api/v1/console/orders?limit=20"},
			{http.MethodGet, "/api/v1/console/merchants?limit=20"},
			{http.MethodPut, "/api/v1/console/settings/mail"},
		} {
			headers := recoveryHeaders(recoveryCookie, csrf)
			body := strings.NewReader("")
			if attempt.method == http.MethodPut {
				payload, _ := json.Marshal(map[string]any{"expected_version": recoverySettingsVersion(t, fixture), "value": map[string]any{"host": "smtp.example.test"}})
				body = strings.NewReader(string(payload))
			}
			response := fixture.RequestWithHeaders(t, attempt.method, attempt.path, body, "guest", headers)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("recovery session accessed restricted route %s %s: status=%d body=%s", attempt.method, attempt.path, response.Code, response.Body.String())
			}
		}

		repair := map[string]any{"expected_version": recoverySettingsVersion(t, fixture), "value": map[string]any{
			"turnstile_site_key": "recovery-site-key", "turnstile_enabled": true, "turnstile_secret": "recovery-secret",
		}}
		encoded, err := json.Marshal(repair)
		if err != nil {
			t.Fatal("encode recovery-only security repair")
		}
		response := fixture.RequestWithHeaders(t, http.MethodPut, "/api/v1/console/settings/security", strings.NewReader(string(encoded)), "guest", recoveryHeaders(recoveryCookie, csrf))
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		if !strings.Contains(strings.Join(response.Header().Values("Set-Cookie"), "\n"), "Max-Age=0") && !strings.Contains(strings.Join(response.Header().Values("Set-Cookie"), "\n"), "Max-Age=-1") {
			t.Fatalf("successful recovery did not clear its restricted cookie: %v", response.Header().Values("Set-Cookie"))
		}
	})

	t.Run("V18.B.r03/v01", func(t *testing.T) {
		code, err := fixture.App.Services.Auth.IssueConsoleRecoveryCode(t.Context())
		if err != nil {
			t.Fatal("issue recovery code for one-use check")
		}
		first := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]any{"recovery_code": code}, "guest")
		if first.Code != http.StatusOK {
			t.Fatalf("first recovery status=%d body=%s", first.Code, first.Body.String())
		}
		replay := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]any{"recovery_code": code}, "guest")
		assertRecoveryAPIError(t, replay, http.StatusUnauthorized, "INVALID_CODE")
	})

	t.Run("V18.B.r02/v02", func(t *testing.T) {
		code, err := fixture.App.Services.Auth.IssueConsoleRecoveryCode(t.Context())
		if err != nil {
			t.Fatal("issue recovery code for expiry check")
		}
		recovered := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]any{"recovery_code": code}, "guest")
		if recovered.Code != http.StatusOK {
			t.Fatalf("recovery session creation status=%d body=%s", recovered.Code, recovered.Body.String())
		}
		csrf := responseCSRF(t, recovered.Body.Bytes())
		cookie := findRecoveryCookie(t, recovered.Header().Values("Set-Cookie"))
		db.Clock.Set(db.Clock.Now().Add(10 * time.Minute))
		payload, _ := json.Marshal(map[string]any{"expected_version": recoverySettingsVersion(t, fixture), "value": map[string]any{
			"turnstile_site_key": "expired-site-key", "turnstile_enabled": true, "turnstile_secret": "expired-secret",
		}})
		response := fixture.RequestWithHeaders(t, http.MethodPut, "/api/v1/console/settings/security", strings.NewReader(string(payload)), "guest", recoveryHeaders(cookie, csrf))
		assertRecoveryAPIError(t, response, http.StatusUnauthorized, "UNAUTHENTICATED")
	})

	t.Run("V18.B.r02/v03", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		admin := bootstrapRecoveryAdmin(t, fixture, "recovery-existing-secret")
		initial := updateSettingsGroupHTTP(t, fixture, admin, "security", map[string]any{
			"turnstile_site_key": "existing-site-key", "turnstile_secret": "existing-recovery-secret", "turnstile_enabled": true, "turnstile_token": "ok:settings_security",
			"restrict_email_domains": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "help@example.test", "en": "help@example.test"},
		})
		assertRecoveryAPIError(t, initial, http.StatusOK, "")

		code, err := fixture.App.Services.Auth.IssueConsoleRecoveryCode(t.Context())
		if err != nil {
			t.Fatal("issue recovery code with an existing Turnstile secret")
		}
		recovered := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]any{"recovery_code": code}, "guest")
		if recovered.Code != http.StatusOK {
			t.Fatalf("recovery session creation status=%d body=%s", recovered.Code, recovered.Body.String())
		}
		payload, err := json.Marshal(map[string]any{"expected_version": recoverySettingsVersion(t, fixture), "value": map[string]any{
			"turnstile_site_key": "updated-site-key", "turnstile_enabled": true,
		}})
		if err != nil {
			t.Fatal("encode recovery update without a replacement secret")
		}
		cookie := findRecoveryCookie(t, recovered.Header().Values("Set-Cookie"))
		response := fixture.RequestWithHeaders(t, http.MethodPut, "/api/v1/console/settings/security", strings.NewReader(string(payload)), "guest", recoveryHeaders(cookie, responseCSRF(t, recovered.Body.Bytes())))
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		if strings.Contains(response.Body.String(), "existing-recovery-secret") || strings.Contains(response.Body.String(), `"turnstile_secret"`) {
			t.Fatal("recovery response exposed the stored Turnstile secret")
		}
		secret, err := fixture.App.Services.Settings.Secret(t.Context(), "turnstile_secret")
		if err != nil || secret != "existing-recovery-secret" {
			t.Fatalf("recovery update changed the stored Turnstile secret: err=%v", err)
		}

		fixture.Siteverify.SetMode("accept")
		login := fixture.Request(t, http.MethodPost, "/api/v1/console/login", map[string]any{
			"identifier": "recovery_fixture_admin", "password": fixturePassword, "turnstile_token": "ok:login",
		}, "guest")
		if login.Code != http.StatusOK {
			t.Fatalf("console login after recovery update status=%d body=%s", login.Code, login.Body.String())
		}
		requests := fixture.Siteverify.Requests()
		if len(requests) == 0 || requests[len(requests)-1].Get("secret") != "existing-recovery-secret" {
			t.Fatalf("console login did not verify Turnstile with the stored secret: requests=%d", len(requests))
		}
	})

	t.Run("V18.B.r02/v04", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		bootstrapRecoveryAdmin(t, fixture, "recovery-no-secret")
		code, err := fixture.App.Services.Auth.IssueConsoleRecoveryCode(t.Context())
		if err != nil {
			t.Fatal("issue recovery code without a configured Turnstile secret")
		}
		recovered := fixture.Request(t, http.MethodPost, "/api/v1/console/recovery", map[string]any{"recovery_code": code}, "guest")
		if recovered.Code != http.StatusOK {
			t.Fatalf("recovery session creation status=%d body=%s", recovered.Code, recovered.Body.String())
		}
		payload, err := json.Marshal(map[string]any{"expected_version": recoverySettingsVersion(t, fixture), "value": map[string]any{
			"turnstile_site_key": "new-site-key", "turnstile_enabled": true,
		}})
		if err != nil {
			t.Fatal("encode recovery update without a configured secret")
		}
		cookie := findRecoveryCookie(t, recovered.Header().Values("Set-Cookie"))
		response := fixture.RequestWithHeaders(t, http.MethodPut, "/api/v1/console/settings/security", strings.NewReader(string(payload)), "guest", recoveryHeaders(cookie, responseCSRF(t, recovered.Body.Bytes())))
		assertRecoveryAPIError(t, response, http.StatusBadRequest, "VALIDATION_ERROR")
	})
}

func bootstrapRecoveryAdmin(t *testing.T, fixture *testsupport.HTTPFixture, client string) string {
	t.Helper()
	code, err := fixture.App.Services.Auth.RotateBootstrapCode(t.Context())
	if err != nil {
		t.Fatal("prepare one-time bootstrap code for recovery fixture")
	}
	role := "admin:" + client
	response := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", map[string]any{
		"initialization_code": code, "username": "recovery_fixture_admin", "password": fixturePassword, "password_confirmation": fixturePassword,
	}, role)
	if response.Code != http.StatusCreated {
		t.Fatalf("bootstrap recovery admin status=%d body=%s", response.Code, response.Body.String())
	}
	return role
}

func recoverySettingsVersion(t *testing.T, fixture *testsupport.HTTPFixture) int64 {
	t.Helper()
	var version int64
	if err := fixture.DB.Pool.QueryRow(t.Context(), `SELECT version FROM site_settings WHERE singleton=true`).Scan(&version); err != nil {
		t.Fatal("read recovery settings version")
	}
	return version
}

func recoveryHeaders(cookie, csrf string) http.Header {
	headers := make(http.Header)
	headers.Set("Cookie", cookie)
	headers.Set("X-CSRF-Token", csrf)
	return headers
}

func findRecoveryCookie(t *testing.T, cookies []string) string {
	t.Helper()
	for _, cookie := range cookies {
		first := strings.SplitN(cookie, ";", 2)[0]
		if strings.HasPrefix(first, "vp_recovery=") || strings.HasPrefix(first, "__Host-vp_recovery=") {
			return first
		}
	}
	t.Fatalf("response did not set recovery cookie: %v", cookies)
	return ""
}

func responseCSRF(t *testing.T, raw []byte) string {
	t.Helper()
	var envelope struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Data.CSRFToken == "" {
		t.Fatalf("decode recovery CSRF token: %v body=%s", err, raw)
	}
	return envelope.Data.CSRFToken
}

func assertRecoveryAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("recovery API status=%d, want %d body=%s", response.Code, status, response.Body.String())
	}
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode recovery API envelope: %v body=%s", err, response.Body.String())
	}
	if code == "" {
		if envelope.Error != nil {
			t.Fatalf("recovery API unexpected error %#v body=%s", envelope.Error, response.Body.String())
		}
		return
	}
	if envelope.Error == nil || envelope.Error.Code != code {
		t.Fatalf("recovery API error=%#v, want %q body=%s", envelope.Error, code, response.Body.String())
	}
}
