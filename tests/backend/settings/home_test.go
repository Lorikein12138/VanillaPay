package settings

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_PublicHomeValidation(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	bootstrapSettingsHomeAdmin(t, fixture)

	t.Run("V20.B.r01/v01", func(t *testing.T) {
		setSettingsHome(t, fixture, "home", map[string]any{"mode": "default", "html": nil, "url": nil})
		got := readPublicHome(t, fixture)
		if got.Mode != "default" || got.HTML != nil || got.URL != nil {
			t.Fatalf("default home config = %#v", got)
		}
	})

	t.Run("V20.B.r01/v02", func(t *testing.T) {
		const page = "<main>Local home content</main>"
		setSettingsHome(t, fixture, "home", map[string]any{"mode": "html", "html": page, "url": nil})
		got := readPublicHome(t, fixture)
		if got.Mode != "html" || got.HTML == nil || *got.HTML != page || got.URL != nil {
			t.Fatalf("HTML home config = %#v", got)
		}
	})

	t.Run("V20.B.r01/v03", func(t *testing.T) {
		const target = "https://unregistered-home.example.test/path"
		setSettingsHome(t, fixture, "home", map[string]any{"mode": "url", "html": nil, "url": target})
		got := readPublicHome(t, fixture)
		if got.Mode != "url" || got.URL == nil || *got.URL != target || got.HTML != nil {
			t.Fatalf("URL home config = %#v", got)
		}
	})

	t.Run("V20.B.r01/v04", func(t *testing.T) {
		for _, invalid := range []map[string]any{
			{"mode": "html", "html": "", "url": nil},
			{"mode": "html", "html": nil, "url": nil},
			{"mode": "default", "html": "not default", "url": nil},
			{"mode": "url", "html": nil, "url": "http://home.example.test"},
		} {
			assertSettingsHomeRejected(t, fixture, invalid)
		}
	})

	t.Run("V20.B.r01/v05", func(t *testing.T) {
		version := readSettingsVersion(t, fixture)
		maxHTML := make([]byte, 256<<10)
		for i := range maxHTML {
			maxHTML[i] = 'x'
		}
		setSettingsHome(t, fixture, "home", map[string]any{"mode": "html", "html": string(maxHTML), "url": nil})
		if got := readSettingsVersion(t, fixture); got != version+1 {
			t.Fatalf("exactly 256KiB HTML version=%d, want %d", got, version+1)
		}
		tooLargeHTML := string(append(append([]byte(nil), maxHTML...), 'x'))
		assertSettingsHomeRejected(t, fixture, map[string]any{"mode": "html", "html": tooLargeHTML, "url": nil})
		longURL := "https://home.example.test/" + string(bytesOf('a', 2048-len("https://home.example.test/")))
		if len(longURL) != 2048 {
			t.Fatalf("URL boundary fixture length=%d", len(longURL))
		}
		setSettingsHome(t, fixture, "home", map[string]any{"mode": "url", "html": nil, "url": longURL})
		assertSettingsHomeRejected(t, fixture, map[string]any{"mode": "url", "html": nil, "url": longURL + "a"})
	})

	t.Run("V20.B.r02/v01", func(t *testing.T) {
		seedPublicConfigSecrets(t, fixture)
		config := fixture.Request(t, http.MethodGet, "/api/v1/public/config", nil, "guest")
		if config.Code != http.StatusOK {
			t.Fatalf("public config status=%d body=%s", config.Code, config.Body.String())
		}
		for _, secret := range []string{"smtp_password", "merchant_key", "turnstile_secret", "APP_ENCRYPTION_KEY"} {
			if containsText(config.Body.String(), secret) {
				t.Fatalf("public config exposed secret field name %q", secret)
			}
		}
	})

	t.Run("V20.B.r02/v02", func(t *testing.T) {
		secrets := seedPublicConfigSecrets(t, fixture)
		var logs bytes.Buffer
		previousLogWriter := log.Writer()
		log.SetOutput(&logs)
		t.Cleanup(func() { log.SetOutput(previousLogWriter) })
		response := fixture.Request(t, http.MethodGet, "/api/v1/public/config", nil, "guest")
		if response.Code != http.StatusOK {
			t.Fatalf("public config status=%d body=%s", response.Code, response.Body.String())
		}
		for name, secret := range secrets {
			if strings.Contains(response.Body.String(), secret) || strings.Contains(logs.String(), secret) {
				t.Fatalf("public config or application log exposed %s sentinel", name)
			}
		}
	})

	t.Run("V20.B.r03/v01", func(t *testing.T) {
		const target = "https://unregistered-home.example.test/save-does-not-fetch"
		before := fixture.Network.Evidence()
		setSettingsHome(t, fixture, "home", map[string]any{"mode": "url", "html": nil, "url": target})
		after := fixture.Network.Evidence()
		if len(after.DNSQueries) != len(before.DNSQueries) || len(after.LogicalDials) != len(before.LogicalDials) || len(after.HTTPRequests) != len(before.HTTPRequests) {
			t.Fatalf("saving the configured home URL caused an outbound request: before=%#v after=%#v", before, after)
		}
	})

	t.Run("V20.B.r03/v02", func(t *testing.T) {
		const target = "https://unregistered-home.example.test/read-does-not-fetch"
		setSettingsHome(t, fixture, "home", map[string]any{"mode": "url", "html": nil, "url": target})
		before := fixture.Network.Evidence()
		got := readPublicHome(t, fixture)
		after := fixture.Network.Evidence()
		if got.Mode != "url" || got.URL == nil || *got.URL != target {
			t.Fatalf("public home config = %#v", got)
		}
		if len(after.DNSQueries) != len(before.DNSQueries) || len(after.LogicalDials) != len(before.LogicalDials) || len(after.HTTPRequests) != len(before.HTTPRequests) {
			t.Fatalf("reading the configured home URL caused an outbound request: before=%#v after=%#v", before, after)
		}
	})

}

type publicHomeView struct {
	Mode string  `json:"mode"`
	HTML *string `json:"html"`
	URL  *string `json:"url"`
}

func bootstrapSettingsHomeAdmin(t *testing.T, fixture *testsupport.HTTPFixture) {
	t.Helper()
	code, err := fixture.App.Services.Auth.RotateBootstrapCode(t.Context())
	if err != nil {
		t.Fatal("prepare settings home bootstrap code")
	}
	response := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", map[string]any{
		"initialization_code": code, "username": "settings_home_admin", "password": fixturePassword, "password_confirmation": fixturePassword,
	}, "admin:settings-home")
	if response.Code != http.StatusCreated {
		t.Fatalf("bootstrap settings home admin status=%d body=%s", response.Code, response.Body.String())
	}
}

func setSettingsHome(t *testing.T, fixture *testsupport.HTTPFixture, group string, value map[string]any) {
	t.Helper()
	setSettingsGroup(t, fixture, group, value)
}

func setSettingsGroup(t *testing.T, fixture *testsupport.HTTPFixture, group string, value map[string]any) {
	t.Helper()
	update := map[string]any{"expected_version": readSettingsVersion(t, fixture), "value": value}
	response := fixture.Request(t, http.MethodPut, "/api/v1/console/settings/"+group, update, "admin:settings-home")
	if response.Code != http.StatusOK {
		t.Fatalf("save %s settings status=%d body=%s", group, response.Code, response.Body.String())
	}
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error != nil {
		t.Fatalf("save %s settings error=%#v decode=%v body=%s", group, envelope.Error, err, response.Body.String())
	}
}

func seedPublicConfigSecrets(t *testing.T, fixture *testsupport.HTTPFixture) map[string]string {
	t.Helper()
	secrets := map[string]string{
		"smtp_password":    fixturePassword,
		"turnstile_secret": "turnstile-public-config-sentinel",
	}
	setSettingsGroup(t, fixture, "mail", map[string]any{
		"host": "mail.example.test", "port": 587, "username": "smtp-user-sentinel",
		"password": secrets["smtp_password"], "from_address": "noreply@example.test",
		"from_name": "VanillaPay", "tls_mode": "starttls",
	})
	setSettingsGroup(t, fixture, "security", map[string]any{
		"turnstile_site_key": "turnstile-site-key-sentinel", "turnstile_secret": secrets["turnstile_secret"],
		"turnstile_enabled": false, "restrict_email_domains": false,
		"email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "", "en": ""},
	})

	merchantID, _ := fixture.DB.SeedMerchant(t)
	var cipher []byte
	if err := fixture.DB.Pool.QueryRow(t.Context(), `SELECT key_cipher FROM merchants WHERE id=$1`, merchantID).Scan(&cipher); err != nil {
		t.Fatal("read merchant key for public config secret fixture")
	}
	key, err := platform.Decrypt(fixture.DB.Key[:], cipher)
	if err != nil {
		t.Fatal("decrypt merchant key for public config secret fixture")
	}
	secrets["merchant_key"] = string(key)
	var pid string
	if err := fixture.DB.Pool.QueryRow(t.Context(), `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&pid); err != nil {
		t.Fatal("read billing merchant PID for public config secret fixture")
	}
	setSettingsGroup(t, fixture, "billing", map[string]any{"merchant_pid": pid, "merchant_key": string(key)})
	return secrets
}

func assertSettingsHomeRejected(t *testing.T, fixture *testsupport.HTTPFixture, value map[string]any) {
	t.Helper()
	version := readSettingsVersion(t, fixture)
	response := fixture.Request(t, http.MethodPut, "/api/v1/console/settings/home", map[string]any{"expected_version": version, "value": value}, "admin:settings-home")
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusBadRequest || envelope.Error == nil || envelope.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("invalid home value status=%d error=%#v decode=%v body=%s", response.Code, envelope.Error, err, response.Body.String())
	}
	if got := readSettingsVersion(t, fixture); got != version {
		t.Fatalf("rejected home value partially changed settings version from %d to %d", version, got)
	}
}

func readPublicHome(t *testing.T, fixture *testsupport.HTTPFixture) publicHomeView {
	t.Helper()
	response := fixture.Request(t, http.MethodGet, "/api/v1/public/config", nil, "guest")
	var envelope struct {
		Data struct {
			Home publicHomeView `json:"home"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusOK || envelope.Data.Home.Mode == "" {
		t.Fatalf("public home DTO status=%d data=%#v err=%v body=%s", response.Code, envelope.Data.Home, err, response.Body.String())
	}
	return envelope.Data.Home
}

func readSettingsVersion(t *testing.T, fixture *testsupport.HTTPFixture) int64 {
	t.Helper()
	var version int64
	if err := fixture.DB.Pool.QueryRow(t.Context(), `SELECT version FROM site_settings WHERE singleton=true`).Scan(&version); err != nil {
		t.Fatal("read settings version")
	}
	return version
}

func bytesOf(value byte, length int) []byte {
	result := make([]byte, length)
	for i := range result {
		result[i] = value
	}
	return result
}

func containsText(value, fragment string) bool { return strings.Contains(value, fragment) }
