package portal

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func TestUnit_PortalSuccessEnvelopeIncludesNullError(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		portalWrite(w, r, map[string]any{"items": []any{}}, nil, http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	var body map[string]json.RawMessage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || string(body["error"]) != "null" || len(body["data"]) == 0 || len(body["request_id"]) == 0 {
		t.Fatalf("portal success envelope status=%d body=%s; data, error=null, and request_id are required", response.Code, response.Body.String())
	}
}

func TestUnit_CursorAuthenticationAndPageBounds(t *testing.T) {
	service := &Service{}
	service.key[0] = 0x91
	created := time.Date(2026, 10, 31, 16, 0, 0, 123, time.UTC)
	cursor := service.encodeCursor(created, 42)
	gotTime, gotID, err := service.decodeCursor(cursor)
	if err != nil || !gotTime.Equal(created) || gotID != 42 {
		t.Fatalf("decodeCursor() = %s, %d, %v", gotTime, gotID, err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatal(err)
	}
	decoded[len(decoded)-1] ^= 1
	if _, _, err := service.decodeCursor(base64.RawURLEncoding.EncodeToString(decoded)); err == nil {
		t.Fatal("modified cursor passed authentication")
	}
	for _, limit := range []int{-1, 101} {
		if _, err := pageLimit(limit); err == nil {
			t.Errorf("pageLimit(%d) unexpectedly succeeded", limit)
		}
	}
	if got, err := pageLimit(0); err != nil || got != 20 {
		t.Fatalf("default page size = %d, %v", got, err)
	}
}

func TestUnit_CSVFormulaEscaping(t *testing.T) {
	for _, value := range []string{"=1+1", "+cmd", "-1", "@SUM(A1)", "\tformula"} {
		if got := csvSafe(value); !strings.HasPrefix(got, "'") {
			t.Errorf("formula-like value %q was not escaped: %q", value, got)
		}
	}
	if got := csvSafe("safe text"); got != "safe text" {
		t.Fatalf("ordinary text changed to %q", got)
	}
}
