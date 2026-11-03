package devices

import (
	"encoding/json"
	"net/http"
	"testing"

	"vanillapay/internal/platform"
	settingsservice "vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_WebBindingResponseDoesNotExposeDeviceToken(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	code, err := fixture.App.Services.Auth.RotateBootstrapCode(t.Context())
	if err != nil {
		t.Fatal("prepare one-use bootstrap code")
	}
	adminRole := "admin:web-binding"
	bootstrap := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", map[string]any{
		"initialization_code": code, "username": "web_binding_admin", "password": fixturePassword, "password_confirmation": fixturePassword,
	}, adminRole)
	if bootstrap.Code != http.StatusCreated {
		t.Fatalf("bootstrap console admin status=%d body=%s", bootstrap.Code, bootstrap.Body.String())
	}
	security := json.RawMessage(`{"turnstile_site_key":"site-key","turnstile_secret":"local-secret","allowed_hostname":"pay.example.test","restrict_email_domains":false,"email_domains":[],"ban_contact":{"zh_CN":"support@example.test","en":"support@example.test"}}`)
	if _, err := fixture.App.Services.Settings.UpdateGroup(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, "security", settingsservice.UpdateGroup{ExpectedVersion: 1, Value: security}); err != nil {
		t.Fatalf("configure local Turnstile without changing auth semantics: %v", err)
	}
	created := fixture.Request(t, http.MethodPost, "/api/v1/console/merchants", map[string]any{
		"username": "web_binding_merchant", "email": "web-binding@example.test", "password": fixturePassword, "password_confirmation": fixturePassword,
	}, adminRole)
	if created.Code != http.StatusCreated {
		t.Fatalf("create merchant for web binding route status=%d body=%s", created.Code, created.Body.String())
	}
	fixture.Siteverify.SetMode("accept")
	merchantRole := "merchant:web-binding"
	login := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"identifier": "web_binding_merchant", "password": fixturePassword, "turnstile_token": "ok:login",
	}, merchantRole)
	if login.Code != http.StatusOK {
		t.Fatalf("login merchant for web binding route status=%d body=%s", login.Code, login.Body.String())
	}
	response := fixture.Request(t, http.MethodPost, "/api/v1/merchant/device/bindings", map[string]any{"replace_confirmed": false}, merchantRole)
	if response.Code != http.StatusCreated {
		t.Fatalf("create web binding status=%d", response.Code)
	}
	var envelope struct {
		Data  map[string]json.RawMessage `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error != nil || len(envelope.Data) == 0 {
		t.Fatal("binding response must contain successful data")
	}
	for _, forbidden := range []string{"device_token", "token", "authorization"} {
		if _, present := envelope.Data[forbidden]; present {
			t.Fatalf("merchant webpage binding response exposed credential field %q", forbidden)
		}
	}
	for _, required := range []string{"binding_id", "code", "qr_content"} {
		var value string
		if err := json.Unmarshal(envelope.Data[required], &value); err != nil || value == "" {
			t.Fatalf("binding response omitted browser exchange field %q", required)
		}
	}
}
