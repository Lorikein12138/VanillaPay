package device

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"vanillapay/internal/platform"
)

func TestUnit_DeviceSuccessEnvelopeIncludesNullError(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceWrite(w, r, map[string]any{"online": true}, nil, http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	var body map[string]json.RawMessage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || string(body["error"]) != "null" || len(body["data"]) == 0 || len(body["request_id"]) == 0 {
		t.Fatalf("device success envelope status=%d body=%s; data, error=null, and request_id are required", response.Code, response.Body.String())
	}
}

func TestUnit_DeviceEventAndChannelValidation(t *testing.T) {
	if !uuidPattern.MatchString("123e4567-e89b-12d3-a456-426614174000") {
		t.Fatal("canonical UUID rejected")
	}
	if uuidPattern.MatchString("not-a-uuid") {
		t.Fatal("invalid event identifier accepted")
	}
	for _, channel := range []string{"alipay", "wxpay"} {
		if !platform.ValidPaymentChannel(channel) {
			t.Errorf("valid channel %q rejected", channel)
		}
	}
	if platform.ValidPaymentChannel("unknown") || platform.ValidPaymentChannel("qqpay") {
		t.Fatal("unknown device channel accepted")
	}
}
