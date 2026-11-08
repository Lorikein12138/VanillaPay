package contracts

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"vanillapay/internal/portal"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_ConsoleAuditFilters(t *testing.T) {
	fixture, _ := securityFixture(t)
	const path = "/api/v1/console/audit-logs"
	const admin = "admin:matrix-admin"
	merchant := fixture.Request(t, http.MethodGet, "/api/v1/merchant/me", nil, "merchant:matrix-merchant")
	var identity struct {
		Data struct {
			User struct{ ID string } `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(merchant.Body.Bytes(), &identity); err != nil || merchant.Code != http.StatusOK || identity.Data.User.ID == "" {
		t.Fatalf("merchant identity: status=%d, decode error=%v", merchant.Code, err)
	}
	id := identity.Data.User.ID
	filters := url.Values{"merchant_id": {id}, "action": {"auth.login"}, "result": {"success"}, "actor_role": {"merchant"}}
	parameters := testsupport.OpenAPI(t).Paths.Find(path).Get.Parameters
	queryParameters := make(map[string]bool)
	for _, parameter := range parameters {
		if parameter.Value != nil && parameter.Value.In == "query" {
			queryParameters[parameter.Value.Name] = true
		}
	}
	for name := range filters {
		if !queryParameters[name] {
			t.Errorf("OpenAPI missing query parameter %s", name)
		}
	}
	// The fixture validates the real HTTP response against the OpenAPI operation.
	response := fixture.Request(t, http.MethodGet, path+"?"+filters.Encode(), nil, admin)
	var page struct {
		Data portal.Page[portal.AuditDTO] `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil || response.Code != http.StatusOK || page.Data.Total != 1 || len(page.Data.Items) != 1 {
		t.Fatalf("filtered audit page: status=%d total=%d items=%d decode error=%v", response.Code, page.Data.Total, len(page.Data.Items), err)
	}
	item := page.Data.Items[0]
	if item.MerchantID == nil || *item.MerchantID != id || item.Action != "auth.login" || item.Result != "success" || item.ActorRole != "merchant" {
		t.Fatalf("unexpected audit event: %+v", item)
	}
	filters.Set("actor_role", "guest")
	invalid := fixture.Request(t, http.MethodGet, path+"?"+filters.Encode(), nil, admin)
	assertAPIError(t, invalid, http.StatusBadRequest, "VALIDATION_ERROR")
}
