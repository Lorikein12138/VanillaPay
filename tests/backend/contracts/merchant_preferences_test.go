package contracts

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestHTTP_MerchantDeviceOfflineEmailPreference(t *testing.T) {
	fixture, proofs := securityFixture(t)
	const merchant = "merchant:matrix-merchant"
	const admin = "admin:matrix-admin"
	const merchantPath = "/api/v1/merchant/preferences"
	const consolePath = "/api/v1/console/preferences"

	checkDevice := func(bound, enabled bool) {
		t.Helper()
		response := fixture.Request(t, http.MethodGet, "/api/v1/merchant/device", nil, merchant)
		var envelope struct {
			Data struct {
				Bound   bool  `json:"bound"`
				Enabled *bool `json:"device_offline_email_enabled"`
			} `json:"data"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &envelope) != nil ||
			envelope.Data.Bound != bound || envelope.Data.Enabled == nil || *envelope.Data.Enabled != enabled {
			t.Fatalf("device status=%d, want bound=%t and offline email enabled=%t", response.Code, bound, enabled)
		}
	}
	checkPatch := func(path, client string, body any, status int) map[string]string {
		t.Helper()
		response := fixture.Request(t, http.MethodPatch, path, body, client)
		if response.Code != status {
			t.Fatalf("PATCH %s as %s: status=%d want=%d", path, client, response.Code, status)
		}
		var envelope struct {
			Data map[string]string `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data
	}

	checkDevice(true, true)
	if response := fixture.Request(t, http.MethodDelete, "/api/v1/merchant/device", map[string]bool{"confirm": true}, merchant); response.Code != http.StatusOK {
		t.Fatalf("remove device: status=%d", response.Code)
	}
	checkDevice(false, true)
	disabled := map[string]bool{"device_offline_email_enabled": false}
	checkPatch(merchantPath, "guest", disabled, http.StatusUnauthorized)
	checkPatch(merchantPath, admin, disabled, http.StatusForbidden)
	proof := proofs["merchant"]
	noCSRF := fixture.RequestWithHeaders(t, http.MethodPatch, merchantPath, strings.NewReader(`{"device_offline_email_enabled":false}`), "guest", http.Header{
		"Content-Type": {"application/json"}, "Cookie": {proof.cookie.Name + "=" + proof.cookie.Value},
	})
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("PATCH without CSRF: status=%d", noCSRF.Code)
	}
	checkPatch(consolePath, admin, disabled, http.StatusBadRequest)
	checkPatch(merchantPath, merchant, map[string]any{}, http.StatusBadRequest)
	checkPatch(merchantPath, merchant, map[string]any{"device_offline_email_enabled": nil}, http.StatusBadRequest)
	checkPatch(merchantPath, merchant, map[string]string{"device_offline_email_enabled": "false"}, http.StatusBadRequest)
	checkDevice(false, true)

	checkPatch(merchantPath, merchant, map[string]string{"locale": "en", "theme": "dark"}, http.StatusOK)
	saved := checkPatch(merchantPath, merchant, disabled, http.StatusOK)
	if saved["locale"] != "en" || saved["theme"] != "dark" {
		t.Fatalf("single-field update changed locale or theme: %v", saved)
	}
	checkDevice(false, false)
	saved = checkPatch(merchantPath, merchant, map[string]any{"theme": "light", "device_offline_email_enabled": nil}, http.StatusOK)
	if saved["theme"] != "light" {
		t.Fatalf("theme update with null device preference was not saved: %v", saved)
	}
	checkDevice(false, false)
	saved = checkPatch(consolePath, admin, map[string]string{"locale": "en", "theme": "dark"}, http.StatusOK)
	if saved["locale"] != "en" || saved["theme"] != "dark" {
		t.Fatalf("console preferences response: %v", saved)
	}
	checkPatch(merchantPath, merchant, map[string]bool{"device_offline_email_enabled": true}, http.StatusOK)
	checkDevice(false, true)
}
