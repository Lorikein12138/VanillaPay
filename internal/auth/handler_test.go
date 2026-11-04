package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestUnit_WriteResultSuccessEnvelopeIncludesNullError(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeResult(w, r, map[string]any{"ready": true}, nil, http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusOK)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode success envelope: %v", err)
	}
	if got := string(body["error"]); got != "null" {
		t.Fatalf("error field=%s, want null", got)
	}
	if len(body["data"]) == 0 || len(body["request_id"]) == 0 {
		t.Fatalf("success envelope omitted data or request_id: %s", response.Body.String())
	}
}
