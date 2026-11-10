package payment

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestUnit_PaymentSuccessEnvelopeIncludesNullError(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpJSON(w, r, map[string]any{"ready": true}, nil, http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	var body map[string]json.RawMessage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || string(body["error"]) != "null" || len(body["data"]) == 0 || len(body["request_id"]) == 0 {
		t.Fatalf("payment success envelope status=%d body=%s; data, error=null, and request_id are required", response.Code, response.Body.String())
	}
}

func TestUnit_ExactCentsAndFloatAllocation(t *testing.T) {
	got, err := ParseCents("12.30")
	if err != nil || got != 1230 || FormatMoney(got) != "12.30" {
		t.Fatalf("exact cents parse/format = %d, %q, %v", got, FormatMoney(got), err)
	}
	allocated, err := AllocateAmount(100, "down", 2, 1, 999, map[int64]struct{}{100: {}, 99: {}})
	if err != nil || allocated != 98 {
		t.Fatalf("float allocation = %d, %v; want 98", allocated, err)
	}
	if _, err := AllocateAmount(100, "down", 2, 1, 999, map[int64]struct{}{100: {}, 99: {}, 98: {}}); err == nil || err.Error() != "AMOUNT_CAPACITY_EXHAUSTED" {
		t.Fatalf("exhausted float allocation error = %v", err)
	}
}

func TestUnit_CanonicalPaymentSignatureParameters(t *testing.T) {
	params := url.Values{"z": {"last"}, "a": {"first"}, "empty": {""}, "sign": {"ignored"}, "sign_type": {"ignored"}}
	got, err := Canonical(params)
	if err != nil || got != "a=first&z=last" {
		t.Fatalf("Canonical() = %q, %v", got, err)
	}
	params.Add("a", "duplicate")
	if _, err := Canonical(params); err == nil {
		t.Fatal("repeated signature field was accepted")
	}
}

func TestUnit_ProtocolFormRejectsDuplicatesMixedSourcesAndInvalidUTF8(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
		body string
	}{
		{name: "duplicate-query", url: "/submit?pid=1&pid=2"},
		{name: "duplicate-body", url: "/submit", body: "pid=1&pid=2"},
		{name: "query-and-body", url: "/submit?pid=1", body: "pid=2"},
		{name: "invalid-utf8", url: "/submit", body: "name=%FF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.url, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if values, err := parseProtocolParams(req); err == nil {
				t.Errorf("parseProtocolParams() = %#v, want error", values)
			}
		})
	}
}

func TestUnit_ReturnURLAcceptsReservedPaymentFields(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "https://user:pass@merchant.example/return"} {
		if validReturnURL(raw) {
			t.Errorf("validReturnURL(%q) unexpectedly accepted", raw)
		}
	}
	if !validReturnURL("https://merchant.example/return?campaign=spring&trade_no=old&sign=old") {
		t.Fatal("ordinary HTTPS return URL was rejected")
	}
}
