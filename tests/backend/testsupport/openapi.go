package testsupport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode"

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
		contractResponse := doc.Components.Responses[name].Value
		if len(contractResponse.Content) == 0 && response.Body.Len() != 0 {
			return fmt.Errorf("uncontracted route: status %d has no declared response body", response.Code)
		}
		route = &routers.Route{Operation: &openapi3.Operation{Responses: openapi3.NewResponses(openapi3.WithName(fmt.Sprint(response.Code), contractResponse))}}
	}
	contentType := headers.Get("Content-Type")
	mediaType, _, mediaErr := mime.ParseMediaType(contentType)
	if contentType != "" && mediaErr != nil {
		return fmt.Errorf("parse response Content-Type: %w", mediaErr)
	}
	if strings.HasSuffix(mediaType, "/json") || strings.HasSuffix(mediaType, "+json") {
		if response.Body.Len() != 0 {
			var body any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				return err
			}
			if err := validateOpenResponseMaps(body, nil, false); err != nil {
				return err
			}
		}
		headers = headers.Clone()
		headers.Set("Content-Type", mediaType)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: request, Route: route, PathParams: params},
		Status:                 response.Code, Header: headers, Body: io.NopCloser(bytes.NewReader(response.Body.Bytes())),
		Options: &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	}
	return openapi3filter.ValidateResponse(request.Context(), input)
}

// Credential-bearing fields are legitimate in some typed DTOs. Only check the
// deliberately open maps (including nested objects and arrays within them).
func validateOpenResponseMaps(value any, path []string, sensitive bool) error {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			name := strings.ToLower(key)
			childPath := append(path, key)
			if sensitive && sensitiveResponseKey(key) && !publicResponseMetadata(childPath, child) {
				return fmt.Errorf("sensitive key in open response map: %q", key)
			}
			if err := validateOpenResponseMaps(child, childPath, sensitive || name == "safe_metadata" || name == "details"); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := validateOpenResponseMaps(child, append(path, "[]"), sensitive); err != nil {
				return err
			}
		}
	}
	return nil
}

func publicResponseMetadata(path []string, value any) bool {
	// Allow the numeric version published by merchant and console audit lists.
	if slices.Equal(path, []string{"data", "items", "[]", "safe_metadata", "key_version"}) {
		_, numeric := value.(float64)
		return numeric
	}
	return false
}

func sensitiveResponseKey(key string) bool {
	parts := strings.FieldsFunc(key, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, part := range parts {
		if sensitiveWord(part) {
			return true
		}
		runes := []rune(part)
		start := 0
		for i := 1; i <= len(runes); i++ {
			if i == len(runes) || wordBoundary(runes, i) {
				if sensitiveWord(string(runes[start:i])) {
					return true
				}
				start = i
			}
		}
	}
	compact := strings.ToLower(strings.Join(parts, ""))
	for _, word := range []string{"password", "secret", "token", "credential"} {
		if strings.Contains(compact, word) {
			return true
		}
	}
	return false
}

func sensitiveWord(word string) bool {
	switch strings.ToLower(word) {
	case "key", "totp", "recovery":
		return true
	}
	return false
}

func wordBoundary(word []rune, i int) bool {
	prev, current := word[i-1], word[i]
	return unicode.IsNumber(prev) != unicode.IsNumber(current) ||
		unicode.IsUpper(current) && (unicode.IsLower(prev) || i+1 < len(word) && unicode.IsLower(word[i+1]))
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
