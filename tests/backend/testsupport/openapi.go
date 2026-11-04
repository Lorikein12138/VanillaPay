package testsupport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
)

var loadOpenAPI = sync.OnceValues(func() (*openapi3.T, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("locate API contract")
	}
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(filepath.Join(filepath.Dir(source), "../../../api/openapi.yaml"))
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, err
	}
	return doc, nil
})

// OpenAPI loads and validates the one checked-in contract, including for tests
// that do not need PostgreSQL. It is never inferred from observed responses.
func OpenAPI(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := loadOpenAPI()
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}
	return doc
}

var loadContractRouter = sync.OnceValues(func() (routers.Router, error) {
	doc, err := loadOpenAPI()
	if err != nil {
		return nil, err
	}
	return legacy.NewRouter(doc)
})

// ValidateHTTPResponse applies kin-openapi to the actual response. Request
// validation is intentionally separate: negative tests must reach real input,
// authentication and CSRF handling rather than be stopped by this fixture.
func ValidateHTTPResponse(t *testing.T, request *http.Request, response *httptest.ResponseRecorder) {
	t.Helper()
	if err := validateHTTPResponse(request, response); err != nil {
		t.Errorf("OpenAPI response %s %s status=%d: %v", request.Method, request.URL.Path, response.Code, err)
	}
}

func validateHTTPResponse(request *http.Request, response *httptest.ResponseRecorder) error {
	router, err := loadContractRouter()
	if err != nil {
		return fmt.Errorf("construct OpenAPI router: %w", err)
	}
	route, params, err := router.FindRoute(request)
	headers := response.Header()
	if err != nil {
		var routeError *routers.RouteError
		if !errors.As(err, &routeError) || (routeError.Reason != routers.ErrPathNotFound.Error() && routeError.Reason != routers.ErrMethodNotAllowed.Error()) {
			return fmt.Errorf("find contract route: %w", err)
		}
		methodNotAllowed := routeError.Reason == routers.ErrMethodNotAllowed.Error()
		// legacy reports path-not-found for an unsupported method on a
		// templated path. Probe supported methods through the same router
		// to distinguish it from an actually unknown path.
		if !methodNotAllowed {
			probe := request.Clone(request.Context())
			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions, http.MethodTrace} {
				probe.Method = method
				if _, _, probeErr := router.FindRoute(probe); probeErr == nil {
					methodNotAllowed = true
					break
				}
			}
		}
		name := "UnmatchedRoute"
		status := http.StatusNotFound
		if methodNotAllowed {
			name = "MethodNotAllowed"
			status = http.StatusMethodNotAllowed
		}
		if response.Code != status {
			return fmt.Errorf("uncontracted route: %s %s status=%d want=%d", request.Method, request.URL.Path, response.Code, status)
		}
		if methodNotAllowed && response.Body.Len() == 0 && headers.Get("Content-Type") == "" {
			// chi emits an empty 405 without a media type. Supply the
			// declared empty-string media type only to schema validation.
			headers = headers.Clone()
			headers.Set("Content-Type", "text/plain")
		}
		doc, err := loadOpenAPI()
		if err != nil {
			return err
		}
		route = &routers.Route{Operation: &openapi3.Operation{Responses: openapi3.NewResponses(openapi3.WithName(fmt.Sprint(response.Code), doc.Components.Responses[name].Value))}}
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: request, Route: route, PathParams: params},
		Status:                 response.Code, Header: headers, Body: io.NopCloser(bytes.NewReader(response.Body.Bytes())),
		Options: &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	}
	return openapi3filter.ValidateResponse(request.Context(), input)
}

// ContractHandler covers integration tests that call a protocol router
// directly rather than using HTTPFixture.Request.
func ContractHandler(t *testing.T, next http.Handler) http.Handler {
	t.Helper()
	OpenAPI(t)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := httptest.NewRecorder()
		next.ServeHTTP(recorder, r)
		ValidateHTTPResponse(t, r, recorder)
		for name, values := range recorder.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
	})
}
