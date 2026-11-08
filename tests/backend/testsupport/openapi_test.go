package testsupport

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
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
		{"unknown_404_arbitrary_json", "GET", "/api/unknown-contract-route", "application/json", `{"unexpected":true}`, 404, false},
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

func TestUnit_ContractRejectsSensitiveKeysInOpenResponseMaps(t *testing.T) {
	for _, mediaType := range []string{"application/json", "Application/JSON", "application/json; charset=utf-8", "application/problem+json"} {
		for _, key := range []string{
			"PaSsWoRd", "api_kEy", "client_secret", "clientSecret", "Client-Secret", "cLiEnTSeCrEt", "accessTOKENValue",
			"client.secret", "accessToken2", "api_key_v2", "ClientSecret", "clientsecret", "logincredential", "token_count",
		} {
			t.Run(mediaType+"/"+key, func(t *testing.T) {
				request := httptest.NewRequest("PUT", "https://pay.example.test/api/v1/merchant/collection/settings", nil)
				response := httptest.NewRecorder()
				response.Header().Set("Content-Type", mediaType)
				response.WriteHeader(400)
				_, _ = fmt.Fprintf(response, `{"error":{"code":"VALIDATION_ERROR","message":"invalid","fields":{},`+
					`"details":{"items":[[{"nested":{%q:"redacted"}}]]}},"request_id":"fixture"}`, key)
				if err := validateHTTPResponse(request, response); err == nil || !strings.Contains(err.Error(), "sensitive key in open response map") {
					t.Fatalf("expected sensitive-key rejection, got %v", err)
				}
			})
		}
	}
}

func TestUnit_ContractAcceptsJSONContentTypeVariants(t *testing.T) {
	for _, mediaType := range []string{"application/json", "Application/JSON", "application/json; charset=utf-8"} {
		t.Run(mediaType, func(t *testing.T) {
			request := httptest.NewRequest("PUT", "https://pay.example.test/api/v1/merchant/collection/settings", nil)
			response := httptest.NewRecorder()
			response.Header().Set("Content-Type", mediaType)
			response.WriteHeader(400)
			_, _ = response.WriteString(`{"error":{"code":"VALIDATION_ERROR","message":"invalid","fields":{},
				"details":{"items":[{"monkey":1},{"keyboard":2},{"item.count2":3}]}},"request_id":"fixture"}`)
			if err := validateHTTPResponse(request, response); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnit_ContractRejectsMalformedContentType(t *testing.T) {
	request := httptest.NewRequest("GET", "https://pay.example.test/health/live", nil)
	response := httptest.NewRecorder()
	response.Header().Set("Content-Type", "application/json; charset")
	_, _ = response.WriteString(`{"status":"ok"}`)
	if err := validateHTTPResponse(request, response); err == nil || !strings.Contains(err.Error(), "parse response Content-Type") {
		t.Fatalf("expected invalid Content-Type rejection, got %v", err)
	}
}

func TestUnit_ContractAllowsOnlyNumericAuditKeyVersion(t *testing.T) {
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"numeric audit version", `{"data":{"items":[{"safe_metadata":{"key_version":2}}]}}`, true},
		{"string audit version", `{"data":{"items":[{"safe_metadata":{"key_version":"2"}}]}}`, false},
		{"object audit version", `{"data":{"items":[{"safe_metadata":{"key_version":{"value":"redacted"}}}]}}`, false},
		{"nested audit version", `{"data":{"items":[{"safe_metadata":{"nested":[{"key_version":2}]}}]}}`, false},
		{"error details version", `{"error":{"details":{"key_version":2}}}`, false},
		{"ordinary keys", `{"error":{"details":{"monkey":true,"keyboard":true}}}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var body any
			if err := json.Unmarshal([]byte(test.body), &body); err != nil {
				t.Fatal(err)
			}
			if err := validateOpenResponseMaps(body, nil, false); (err == nil) != test.valid {
				t.Fatalf("open response map validation: valid=%v error=%v", test.valid, err)
			}
		})
	}
}
