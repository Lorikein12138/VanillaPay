package subscription

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestUnit_SubscriptionSuccessEnvelopeIncludesNullError(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subWrite(w, r, map[string]any{"active": true}, nil, http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	var body map[string]json.RawMessage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || string(body["error"]) != "null" || len(body["data"]) == 0 || len(body["request_id"]) == 0 {
		t.Fatalf("subscription success envelope status=%d body=%s; data, error=null, and request_id are required", response.Code, response.Body.String())
	}
}

func TestUnit_PlanValidationAndChannelCanonicalization(t *testing.T) {
	valid := PlanCommand{Name: LocalizedName{ZhCN: "订阅", En: "Subscription"}, PriceCents: 1, DurationDays: 1, Channels: []string{"wxpay", "alipay"}}
	if err := validatePlan(valid); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	for _, invalid := range []PlanCommand{
		{Name: LocalizedName{ZhCN: "订阅"}, PriceCents: 0, DurationDays: 1, Channels: []string{"alipay"}},
		{Name: LocalizedName{ZhCN: "订阅"}, PriceCents: 100, DurationDays: 0, Channels: []string{"alipay"}},
		{Name: LocalizedName{ZhCN: "订阅"}, PriceCents: 100, DurationDays: 1, Channels: []string{"unsupported"}},
	} {
		if err := validatePlan(invalid); err == nil {
			t.Errorf("invalid plan unexpectedly accepted: %#v", invalid)
		}
	}
	if got := canonicalChannels(valid.Channels); len(got) != 2 || got[0] != "alipay" || got[1] != "wxpay" {
		t.Fatalf("canonical channel order = %#v", got)
	}
}
