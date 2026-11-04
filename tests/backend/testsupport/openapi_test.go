package testsupport

import (
	"net/http/httptest"
	"testing"
)

func TestUnit_ContractRejectsUndocumentedResponses(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"valid", `{"status":"ok"}`, 200, true},
		{"unknown_status", `{"status":"ok"}`, 418, false},
		{"missing_field", `{}`, 200, false},
		{"wrong_type", `{"status":1}`, 200, false},
		{"extra_field", `{"status":"ok","unexpected":true}`, 200, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "https://pay.example.test/health/live", nil)
			response := httptest.NewRecorder()
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(test.status)
			_, _ = response.WriteString(test.body)
			if err := validateHTTPResponse(request, response); (err == nil) != test.valid {
				t.Fatalf("contract validation: valid=%v error=%v", test.valid, err)
			}
		})
	}
}

func TestUnit_ContractDistinguishesMissingPathAndMethod(t *testing.T) {
	for _, test := range []struct {
		name, method, path, contentType, body string
		status                                int
		valid                                 bool
	}{
		{"unknown_404", "GET", "/api/unknown-contract-route", "application/json", `{"error":{"code":"NOT_FOUND"}}`, 404, true},
		{"unknown_405", "GET", "/api/unknown-contract-route", "", "", 405, false},
		{"unknown_405_json", "GET", "/api/unknown-contract-route", "application/json", `{}`, 405, false},
		{"known_405", "POST", "/health/live", "", "", 405, true},
		{"templated_405", "DELETE", "/api/v1/checkout/unknown-trade", "", "", 405, true},
		{"known_405_json", "POST", "/health/live", "application/json", `{}`, 405, false},
		{"known_405_text", "POST", "/health/live", "text/plain", "arbitrary body", 405, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "https://pay.example.test"+test.path, nil)
			response := httptest.NewRecorder()
			if test.contentType != "" {
				response.Header().Set("Content-Type", test.contentType)
			}
			response.WriteHeader(test.status)
			_, _ = response.WriteString(test.body)
			if err := validateHTTPResponse(request, response); (err == nil) != test.valid {
				t.Fatalf("contract fallback: valid=%v error=%v", test.valid, err)
			}
		})
	}
}
