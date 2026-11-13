package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestUnit_SettingsSuccessEnvelopeIncludesNullError(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settingsResult(w, r, map[string]any{"configured": true}, nil, http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	var body map[string]json.RawMessage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || string(body["error"]) != "null" || len(body["data"]) == 0 || len(body["request_id"]) == 0 {
		t.Fatalf("settings success envelope status=%d body=%s; data, error=null, and request_id are required", response.Code, response.Body.String())
	}
}

func TestUnit_SettingsGroupValidationAndSecretHandling(t *testing.T) {
	value := json.RawMessage(`{"turnstile_site_key":"site-key","turnstile_secret":"secret","turnstile_enabled":false,"restrict_email_domains":false,"email_domains":[],"ban_contact":{"zh_CN":"联系支持","en":"Contact support"}}`)
	clean, secrets, err := ValidateGroup("security", value)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := secrets["turnstile_secret"]; !ok {
		t.Fatal("Turnstile secret was not extracted for encryption")
	}
	var returned map[string]json.RawMessage
	if err := json.Unmarshal(clean, &returned); err != nil {
		t.Fatal(err)
	}
	if _, ok := returned["turnstile_secret"]; ok {
		t.Fatal("clean settings retained a plaintext secret")
	}
	value = json.RawMessage(`{"site_name":"Site","unknown_field":true}`)
	if _, _, err := ValidateGroup("site", value); err == nil {
		t.Fatal("unknown setting field was accepted")
	}
}

func TestUnit_QRPayloadRequiresExactProviderHost(t *testing.T) {
	cases := []struct {
		channel string
		payload string
		valid   bool
	}{
		{"alipay", "https://qr.alipay.com/abc", true},
		{"alipay", "https://qr.alipay.com.attacker.example/abc", false},
		{"alipay", "https://user@qr.alipay.com/abc", false},
		{"wxpay", "wxp://f2f0abc", true},
		{"wxpay", "https://wx.tenpay.com/cgi-bin/mmpayweb-bin/checkmweb", true},
		{"wxpay", "https://evil.example/?next=wx.tenpay.com", false},
		{"qqpay", "https://i.qianbao.qq.com/qr", false},
	}
	for _, tc := range cases {
		if got := validQRPayload(tc.channel, tc.payload); got != tc.valid {
			t.Errorf("validQRPayload(%q, %q) = %t, want %t", tc.channel, tc.payload, got, tc.valid)
		}
	}
}
