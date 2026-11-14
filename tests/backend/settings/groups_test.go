package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_SettingsGroupLifecycleAndValidation(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	admin := bootstrapRecoveryAdmin(t, fixture, "settings-groups")
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "settings group fixture", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	var merchantPID string
	if err := db.Pool.QueryRow(t.Context(), `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&merchantPID); err != nil {
		t.Fatal("read settings group merchant PID")
	}

	groups := []struct {
		name  string
		value map[string]any
	}{
		{"site", map[string]any{"site_name": "Contract Site", "logo_asset_id": nil, "favicon_asset_id": nil}},
		{"home", map[string]any{"mode": "html", "html": "<main>Home contract</main>", "url": nil}},
		{"footer_legal", map[string]any{
			"items": []any{}, "terms": map[string]string{"zh_CN": "服务条款", "en": "Terms"},
			"privacy": map[string]string{"zh_CN": "隐私政策", "en": "Privacy"},
		}},
		{"mail", map[string]any{
			"host": "smtp.example.test", "port": 587, "username": "mailer", "password": fixturePassword,
			"from_address": "noreply@example.test", "from_name": "VanillaPay", "tls_mode": "starttls",
		}},
		{"security", map[string]any{
			"turnstile_site_key": "settings-test-site-key", "turnstile_secret": "turnstile-secret-original", "turnstile_enabled": false,
			"restrict_email_domains": false, "registration_review": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "support@example.test", "en": "support@example.test"},
		}},
		{"payment", map[string]any{
			"min_amount_cents": 1, "max_amount_cents": 999999999, "default_ttl_seconds": 300, "max_ttl_seconds": 3600,
			"float_min_cents": 0, "float_max_cents": 999, "float_default_cents": 100, "float_direction": "down",
		}},
		{"billing", map[string]any{"merchant_pid": merchantPID, "merchant_key": merchantKey}},
	}
	for _, group := range groups {

		response := updateSettingsGroupHTTP(t, fixture, admin, group.name, group.value)
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		current := readConsoleSettingsGroups(t, fixture, admin)
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(current[group.name], &decoded); err != nil {
			t.Fatalf("decode saved %s settings: %v", group.name, err)
		}
		for key, expected := range group.value {
			if key == "password" || key == "turnstile_secret" || key == "merchant_key" {
				continue
			}
			expectedJSON, err := json.Marshal(expected)
			if err != nil {
				t.Fatalf("encode expected %s.%s value: %v", group.name, key, err)
			}
			if string(decoded[key]) != string(expectedJSON) {
				t.Errorf("saved %s.%s=%s, want %s", group.name, key, decoded[key], expectedJSON)
			}
		}
	}

	t.Run("V17.B.r01/v01", func(t *testing.T) {
		current := readConsoleSettingsGroups(t, fixture, admin)
		for _, group := range groups {
			var stored map[string]json.RawMessage
			if err := json.Unmarshal(current[group.name], &stored); err != nil {
				t.Fatalf("decode %s after save: %v", group.name, err)
			}
			for key, expected := range group.value {
				if key == "password" || key == "turnstile_secret" || key == "merchant_key" {
					continue
				}
				encoded, _ := json.Marshal(expected)
				if string(stored[key]) != string(encoded) {
					t.Errorf("settings group %s field %s did not round-trip: got %s want %s", group.name, key, stored[key], encoded)
				}
			}
		}
	})
	t.Run("V17.B.r01/v02", func(t *testing.T) {
		current := readConsoleSettingsGroups(t, fixture, admin)
		checks := []struct {
			group, secretField, markerField string
		}{
			{"mail", "password", "masked_password"},
			{"security", "turnstile_secret", "secret_configured"},
			{"billing", "merchant_key", "key_configured"},
		}
		for _, check := range checks {
			var stored map[string]json.RawMessage
			if err := json.Unmarshal(current[check.group], &stored); err != nil {
				t.Fatalf("decode masked %s group: %v", check.group, err)
			}
			if _, leaked := stored[check.secretField]; leaked {
				t.Errorf("settings GET exposed secret field %s.%s", check.group, check.secretField)
			}
			if check.group == "mail" {
				var mask string
				if err := json.Unmarshal(stored[check.markerField], &mask); err != nil || mask != "••••••••" {
					t.Errorf("mail secret mask=%q err=%v", mask, err)
				}
			} else {
				var configured bool
				if err := json.Unmarshal(stored[check.markerField], &configured); err != nil || !configured {
					t.Errorf("%s configured marker=%t err=%v", check.group, configured, err)
				}
			}
		}
		for _, secret := range []string{fixturePassword, "turnstile-secret-original", merchantKey} {
			if jsonContainsString(current, secret) {
				t.Errorf("settings GET exposed configured secret %q", secret)
			}
		}
	})
	t.Run("V17.B.r02/v01", func(t *testing.T) {
		mail := map[string]any{"host": "smtp.example.test", "port": 587, "username": "mailer", "from_address": "noreply@example.test", "from_name": "VanillaPay", "tls_mode": "starttls"}
		response := updateSettingsGroupHTTP(t, fixture, admin, "mail", mail)
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		secret, err := fixture.App.Services.Settings.Secret(t.Context(), "smtp_password")
		if err != nil || secret != fixturePassword {
			t.Fatalf("omitting SMTP password replaced the stored secret: %q err=%v", secret, err)
		}
	})
	t.Run("V17.B.r02/v02", func(t *testing.T) {
		mail := map[string]any{
			"host": "smtp.example.test", "port": 587, "username": "mailer", "password": fixtureChangedPassword,
			"from_address": "noreply@example.test", "from_name": "VanillaPay", "tls_mode": "starttls",
		}
		response := updateSettingsGroupHTTP(t, fixture, admin, "mail", mail)
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		secret, err := fixture.App.Services.Settings.Secret(t.Context(), "smtp_password")
		if err != nil || secret != fixtureChangedPassword {
			t.Fatalf("non-empty SMTP password did not replace the secret: %q err=%v", secret, err)
		}
	})
	t.Run("V17.B.r02/v03", func(t *testing.T) {
		mail := map[string]any{
			"host": "smtp.example.test", "port": 587, "username": "mailer", "password": nil,
			"from_address": "noreply@example.test", "from_name": "VanillaPay", "tls_mode": "starttls",
		}
		response := updateSettingsGroupHTTP(t, fixture, admin, "mail", mail)
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		if _, err := fixture.App.Services.Settings.Secret(t.Context(), "smtp_password"); err == nil {
			t.Fatal("null SMTP password did not clear the secret")
		}
		current := readConsoleSettingsGroups(t, fixture, admin)
		var stored map[string]json.RawMessage
		if err := json.Unmarshal(current["mail"], &stored); err != nil {
			t.Fatal("decode mail group after null secret clear")
		}
		var configured bool
		if err := json.Unmarshal(stored["password_configured"], &configured); err != nil || configured {
			t.Fatalf("SMTP password_configured=%t err=%v after null clear", configured, err)
		}
	})
	t.Run("V17.B.r03/v01", func(t *testing.T) {
		before := readConsoleSettingsGroups(t, fixture, admin)
		beforeVersion := recoverySettingsVersion(t, fixture)
		response := fixture.Request(t, http.MethodPut, "/api/v1/console/settings/site", map[string]any{
			"expected_version": beforeVersion - 1, "value": map[string]any{"site_name": "must not commit", "logo_asset_id": nil, "favicon_asset_id": nil},
		}, admin)
		assertRecoveryAPIError(t, response, http.StatusConflict, "VERSION_CONFLICT")
		if afterVersion := recoverySettingsVersion(t, fixture); afterVersion != beforeVersion {
			t.Fatalf("stale settings update changed version %d to %d", beforeVersion, afterVersion)
		}
		after := readConsoleSettingsGroups(t, fixture, admin)
		if string(after["site"]) != string(before["site"]) {
			t.Fatalf("stale settings update partially changed site from %s to %s", before["site"], after["site"])
		}
	})
	t.Run("V17.B.r04/v01", func(t *testing.T) {
		value := map[string]any{
			"min_amount_cents": 1, "max_amount_cents": 999999999, "default_ttl_seconds": 3600, "max_ttl_seconds": 1800,
			"float_min_cents": 0, "float_max_cents": 999, "float_default_cents": 100, "float_direction": "down",
		}
		response := updateSettingsGroupHTTP(t, fixture, admin, "payment", value)
		assertRecoveryAPIError(t, response, http.StatusBadRequest, "VALIDATION_ERROR")
	})
	t.Run("V17.B.r04/v02", func(t *testing.T) {
		value := map[string]any{
			"min_amount_cents": 1, "max_amount_cents": 999999999, "default_ttl_seconds": 300, "max_ttl_seconds": 3600,
			"float_min_cents": 10, "float_max_cents": 50, "float_default_cents": 51, "float_direction": "down",
		}
		response := updateSettingsGroupHTTP(t, fixture, admin, "payment", value)
		assertRecoveryAPIError(t, response, http.StatusBadRequest, "VALIDATION_ERROR")
	})
	t.Run("V17.B.r04/v03", func(t *testing.T) {
		for _, tc := range []struct {
			group string
			value map[string]any
		}{
			{"payment", map[string]any{"min_amount_cents": -1, "max_amount_cents": 999999999, "default_ttl_seconds": 300, "max_ttl_seconds": 3600, "float_min_cents": 0, "float_max_cents": 999, "float_default_cents": 100, "float_direction": "down"}},
			{"payment", map[string]any{"min_amount_cents": 1, "max_amount_cents": 1000000000, "default_ttl_seconds": 300, "max_ttl_seconds": 3600, "float_min_cents": 0, "float_max_cents": 999, "float_default_cents": 100, "float_direction": "down"}},
			{"security", map[string]any{"turnstile_site_key": "settings-test-site-key", "turnstile_secret": "turnstile-secret-original", "turnstile_enabled": false, "restrict_email_domains": true, "registration_review": false, "email_domains": []string{"evil/domain"}, "ban_contact": map[string]string{"zh_CN": "support@example.test", "en": "support@example.test"}}},
		} {
			response := updateSettingsGroupHTTP(t, fixture, admin, tc.group, tc.value)
			assertRecoveryAPIError(t, response, http.StatusBadRequest, "VALIDATION_ERROR")
		}
	})
}

func updateSettingsGroupHTTP(t *testing.T, fixture *testsupport.HTTPFixture, admin, group string, value map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	version := recoverySettingsVersion(t, fixture)
	return fixture.Request(t, http.MethodPut, "/api/v1/console/settings/"+group, map[string]any{"expected_version": version, "value": value}, admin)
}

func readConsoleSettingsGroups(t *testing.T, fixture *testsupport.HTTPFixture, admin string) map[string]json.RawMessage {
	t.Helper()
	response := fixture.Request(t, http.MethodGet, "/api/v1/console/settings", nil, admin)
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusOK || envelope.Error != nil || envelope.Data["version"] == nil {
		t.Fatalf("read console setting groups status=%d body=%s err=%v", response.Code, response.Body.String(), err)
	}
	return envelope.Data
}

func jsonContainsString(value any, expected string) bool {
	encoded, err := json.Marshal(value)
	return err == nil && strings.Contains(string(encoded), expected)
}
