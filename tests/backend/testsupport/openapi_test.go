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
