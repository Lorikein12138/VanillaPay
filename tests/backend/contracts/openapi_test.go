package contracts

import (
	"net/http"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"vanillapay/internal/app"
	"vanillapay/internal/auth"
	"vanillapay/internal/device"
	"vanillapay/internal/httpx"
	"vanillapay/internal/merchant"
	"vanillapay/internal/payment"
	"vanillapay/internal/portal"
	"vanillapay/internal/settings"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

// These exact page/404 handlers and the static download paths are outside the API contract.
const (
	spaRoot             = "/"
	apiRootFallback     = "/api"
	apiWildcardFallback = "/api/*"
)

func TestOpenAPI_DocumentAndRoutes(t *testing.T) {
	doc := testsupport.OpenAPI(t)
	if doc.OpenAPI != "3.0.3" {
		t.Fatalf("OpenAPI version=%q", doc.OpenAPI)
	}
	// Walking does not execute handlers. These are the real service/handler
	// types; constructing the complete route tree must not need a test DB.
	application := &app.App{Services: &app.Services{
		Auth: &auth.Service{}, Device: &device.Service{}, Merchant: &merchant.Service{},
		Payment: &payment.Service{}, Portal: &portal.Service{}, Settings: &settings.Service{}, Subscription: &subscription.Service{},
	}}
	actual := []string{}
	if err := chi.Walk(httpx.NewRouter(application).(chi.Routes), func(method, route string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		switch route {
		case spaRoot, apiRootFallback, apiWildcardFallback, "/downloads", "/downloads/*":
			return nil
		}
		actual = append(actual, method+" "+route)
		function := reflect.ValueOf(handler.ServeHTTP)
		if direct, ok := handler.(http.HandlerFunc); ok {
			function = reflect.ValueOf(direct)
		}
		t.Logf("%s %s -> %s", method, route, runtime.FuncForPC(function.Pointer()).Name())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expected := []string{}
	for path, item := range doc.Paths.Map() {
		for method, operation := range item.Operations() {
			if operation.Security == nil {
				t.Errorf("%s %s must declare security, including [] for public operations", method, path)
			}
			expected = append(expected, strings.ToUpper(method)+" "+path)
		}
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		t.Errorf("route mismatch\nchi: %v\nOpenAPI: %v", actual, expected)
	}
	t.Logf("route alignment: %d operations", len(actual))
}

func TestHTTP_UnknownAPIRouteUses404Fallback(t *testing.T) {
	fixture := testsupport.NewHTTPFixture(t, testsupport.NewDatabase(t))
	response := fixture.Request(t, http.MethodGet, "/api/unknown-contract-route", nil, "guest")
	if response.Code != http.StatusNotFound || !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("unknown API route status=%d content-type=%q", response.Code, response.Header().Get("Content-Type"))
	}
}
