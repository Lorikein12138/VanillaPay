package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5/middleware"

	"vanillapay/internal/app"
	"vanillapay/internal/config"
	"vanillapay/internal/platform"
	"vanillapay/internal/webassets"
)

type testDatabase struct {
	pingErr error
}

func TestUnit_WrappedPlatformErrorPreservesBusinessStatus(t *testing.T) {
	for name, write := range map[string]func(http.ResponseWriter, *http.Request, error){
		"middleware": writePlatformError,
		"response":   func(w http.ResponseWriter, r *http.Request, err error) { writeAPIResult(w, r, nil, err, 200) },
	} {
		t.Run(name, func(t *testing.T) {
			err := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", &platform.Error{Code: "VERSION_CONFLICT", HTTPStatus: http.StatusConflict}))
			response := httptest.NewRecorder()
			write(response, httptest.NewRequest(http.MethodPost, "/api/v1/example", nil), err)
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if decodeErr := json.Unmarshal(response.Body.Bytes(), &body); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if response.Code != http.StatusConflict || body.Error.Code != "VERSION_CONFLICT" {
				t.Fatalf("wrapped error status=%d code=%q", response.Code, body.Error.Code)
			}
		})
	}
}

func (d *testDatabase) Ping(context.Context) error { return d.pingErr }
func (*testDatabase) Close()                       {}

func TestUnit_HealthEndpoints(t *testing.T) {
	router := newTestRouter(t, nil)

	live := httptest.NewRecorder()
	router.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("live returned %d, want %d", live.Code, http.StatusOK)
	}
	if live.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("live Cache-Control = %q, want no-store", live.Header().Get("Cache-Control"))
	}
	if body := strings.TrimSpace(live.Body.String()); body != `{"status":"ok"}` {
		t.Fatalf("live body = %q", body)
	}

	ready := httptest.NewRecorder()
	router.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("ready returned %d, want %d", ready.Code, http.StatusOK)
	}
}

func TestUnit_WriteAPIResultSuccessEnvelopeIncludesNullError(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAPIResult(w, r, map[string]any{"ready": true}, nil, http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusOK)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode success envelope: %v", err)
	}
	if got := string(body["error"]); got != "null" {
		t.Fatalf("error field=%s, want null", got)
	}
	if len(body["data"]) == 0 || len(body["request_id"]) == 0 {
		t.Fatalf("success envelope omitted data or request_id: %s", response.Body.String())
	}
}

func TestUnit_ReadyFailsClosedWithoutLeakingDependencyErrors(t *testing.T) {
	router, database := newTestRouterWithDatabase(t, errors.New("secret database diagnostic"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready returned %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(response.Body.String(), "secret database diagnostic") {
		t.Fatal("ready response leaked an internal dependency error")
	}

	database.pingErr = nil
	recovered := httptest.NewRecorder()
	router.ServeHTTP(recovered, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if recovered.Code != http.StatusOK {
		t.Fatalf("ready returned %d after database recovery, want %d", recovered.Code, http.StatusOK)
	}
}

func TestUnit_UnknownRestrictedRoutesDoNotUseSPAHTML(t *testing.T) {
	router := newTestRouter(t, nil)
	for _, route := range []string{"/api/v1/nope", "/health/nope", "/assets/nope.js", "/media/nope", "/foo.php"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s returned %d, want 404", route, response.Code)
		}
		if strings.Contains(strings.ToLower(response.Header().Get("Content-Type")), "text/html") {
			t.Errorf("%s returned HTML on a restricted miss", route)
		}
		if strings.HasPrefix(route, "/api") && !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
			t.Errorf("%s did not return a JSON API error", route)
		}
	}
}

func TestUnit_SPAServesDeepLinksAndHashAssets(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":                     &fstest.MapFile{Data: []byte("<main>app</main>")},
		"assets/app-0123456789abcdef.js": &fstest.MapFile{Data: []byte("window.app = true;")},
	}
	handler := webassets.SPAHandler(assets)

	deepLink := httptest.NewRecorder()
	handler.ServeHTTP(deepLink, httptest.NewRequest(http.MethodGet, "/merchant/orders", nil))
	if deepLink.Code != http.StatusOK || !strings.Contains(deepLink.Body.String(), "<main>app</main>") {
		t.Fatalf("deep link did not serve index.html: status=%d body=%q", deepLink.Code, deepLink.Body.String())
	}
	if deepLink.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("index Cache-Control = %q, want no-cache", deepLink.Header().Get("Cache-Control"))
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/app-0123456789abcdef.js", nil))
	if asset.Code != http.StatusOK || !strings.Contains(asset.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("hashed asset not immutable: status=%d Cache-Control=%q", asset.Code, asset.Header().Get("Cache-Control"))
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if missing.Code != http.StatusNotFound || strings.Contains(missing.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("missing asset returned SPA content: status=%d content-type=%q", missing.Code, missing.Header().Get("Content-Type"))
	}
}

func TestUnit_HealthDoesNotDependOnDatabaseForLiveness(t *testing.T) {
	router := newTestRouter(t, errors.New("database unavailable"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("live returned %d while database was unavailable", response.Code)
	}
}

func newTestRouter(t *testing.T, pingErr error) http.Handler {
	router, _ := newTestRouterWithDatabase(t, pingErr)
	return router
}

func newTestRouterWithDatabase(t *testing.T, pingErr error) (http.Handler, *testDatabase) {
	t.Helper()
	database := &testDatabase{pingErr: pingErr}
	application, err := app.NewWithDependencies(context.Background(), config.Config{
		DatabaseURL: "postgres://unit-test:unit-test@127.0.0.1:5432/unit_test",
		ListenAddr:  "127.0.0.1:0",
	}, app.Dependencies{
		DatabaseFactory: func(context.Context, config.Config) (app.Database, error) {
			return database, nil
		},
	})
	if err != nil {
		t.Fatalf("construct app: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	return NewRouter(application), database
}
