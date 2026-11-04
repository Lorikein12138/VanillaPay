package contracts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTP_UnknownConsolePathsKeepAuthorizationBeforeRouting(t *testing.T) {
	fixture, proofs := securityFixture(t)
	paths := []string{
		"/api/v1/console/orders/admin-readonly-trade/supplement",
		"/api/v1/console/orders/admin-readonly-trade/refund",
		"/api/v1/console/orders/admin-readonly-trade/callback-replay",
		"/api/v1/console/not-a-route",
	}
	for _, test := range []struct {
		role, code string
		status     int
	}{{"guest", "UNAUTHENTICATED", 401}, {"merchant", "FORBIDDEN", 403}} {
		for _, path := range paths {
			t.Run(test.role+path, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodPost, "https://pay.example.test"+path, strings.NewReader(`{}`))
				request.Header.Set("Origin", "https://pay.example.test")
				if proof, ok := proofs[test.role]; ok {
					request.AddCookie(proof.cookie)
					request.Header.Set("X-CSRF-Token", proof.csrf)
				}
				response := httptest.NewRecorder()
				// Exercise pre-routing middleware directly: these denials are
				// not route misses. The dispatched 404/405 responses are checked
				// through HTTPFixture in TestHTTP_AdminMutationRoutesAbsent.
				fixture.Handler.ServeHTTP(response, request)
				var envelope struct {
					Error struct{ Code string } `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if response.Code != test.status || envelope.Error.Code != test.code {
					t.Fatalf("authorization status=%d code=%q, want %d/%s", response.Code, envelope.Error.Code, test.status, test.code)
				}
			})
		}
	}
}
