package testsupport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
	"vanillapay/internal/app"
	"vanillapay/internal/auth"
	"vanillapay/internal/config"
	"vanillapay/internal/httpx"
	"vanillapay/internal/platform"
)

const fixtureOrigin = "https://pay.example.test"
const fixtureClientRemoteAddr = "192.0.2.44:41000"

// HTTPFixture sends requests through the real router, authentication
// middleware, services, and isolated PostgreSQL schema.
type HTTPFixture struct {
	DB          *Database
	App         *app.App
	Handler     http.Handler
	Mailer      *RecordingMailer
	Network     *PublicNetwork
	Siteverify  *LocalSiteverify
	DeviceToken string
	DataDir     string

	mu   sync.Mutex
	jars map[string]*cookiejar.Jar
	csrf map[string]string
}

// RecordingMailer is a controllable adapter at the application's mail port.
type RecordingMailer struct {
	mu       sync.Mutex
	messages []platform.MailMessage
	err      error
}

func (m *RecordingMailer) Send(_ context.Context, message platform.MailMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, message)
	return m.err
}

func (m *RecordingMailer) Messages() []platform.MailMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]platform.MailMessage(nil), m.messages...)
}

func (m *RecordingMailer) LastCode() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.messages) - 1; i >= 0; i-- {
		if code := m.messages[i].Payload.Code; code != "" {
			return code
		}
	}
	return ""
}

func (m *RecordingMailer) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// LocalSiteverify runs a real local HTTP endpoint and an adapter that sends
// Siteverify-form requests to it. It controls only the external dependency;
// requests still traverse the production HTTP handlers and database-backed
// authentication service.
type LocalSiteverify struct {
	network          *PublicNetwork
	verifier         auth.Siteverify
	mu               sync.Mutex
	mode             string
	expectedHostname string
	seen             []url.Values
}

func NewLocalSiteverify(t *testing.T, network *PublicNetwork) *LocalSiteverify {
	t.Helper()
	fixture := &LocalSiteverify{network: network, mode: "accept"}
	network.SetDNS("challenges.cloudflare.com", []netip.Addr{netip.MustParseAddr(turnstileLogicalIP)})
	network.ServeTLS(t, "challenges.cloudflare.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/turnstile/v0/siteverify" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		fixture.mu.Lock()
		copyForm := make(url.Values, len(r.PostForm))
		for key, values := range r.PostForm {
			copyForm[key] = append([]string(nil), values...)
		}
		fixture.seen = append(fixture.seen, copyForm)
		mode := fixture.mode
		hostname := fixture.expectedHostname
		fixture.mu.Unlock()
		if mode == "unavailable" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		token := r.PostForm.Get("response")
		action := strings.TrimPrefix(token, "ok:")
		success := mode == "accept" && strings.HasPrefix(token, "ok:")
		if mode == "wrong-action" && strings.HasPrefix(token, "wrong-action:") {
			success = true
			action = strings.TrimPrefix(token, "wrong-action:")
		}
		if mode == "wrong-host" && strings.HasPrefix(token, "ok:") {
			success = true
			hostname += ".invalid"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": success, "action": action, "hostname": hostname})
	}))
	fixture.verifier = auth.Siteverify{Client: network.TLSHTTPClient("challenges.cloudflare.com")}
	return fixture
}

func (s *LocalSiteverify) Verify(ctx context.Context, secret, token, action, hostname string) (bool, error) {
	s.mu.Lock()
	s.expectedHostname = hostname
	s.mu.Unlock()
	return s.verifier.Verify(ctx, secret, token, action, hostname)
}

func (s *LocalSiteverify) SetMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
}

func (s *LocalSiteverify) Requests() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]url.Values, len(s.seen))
	for i, values := range s.seen {
		result[i] = make(url.Values, len(values))
		for key, entries := range values {
			result[i][key] = append([]string(nil), entries...)
		}
	}
	return result
}

func NewHTTPFixture(t *testing.T, db *Database) *HTTPFixture {
	return NewHTTPFixtureWithTrustedProxyCIDRs(t, db, nil)
}

// NewHTTPFixtureWithTrustedProxyCIDRs builds the same real application fixture
// with an explicit trusted-proxy configuration for source-address tests.
func NewHTTPFixtureWithTrustedProxyCIDRs(t *testing.T, db *Database, trusted []netip.Prefix) *HTTPFixture {
	t.Helper()
	if db == nil || db.Pool == nil {
		t.Fatal("HTTP fixture requires an isolated database")
	}

	mailer := &RecordingMailer{}
	network := NewPublicNetwork(t)
	siteverify := NewLocalSiteverify(t, network)
	cfg := config.Config{
		AppEnv:            "production",
		DatabaseURL:       db.URL,
		PublicBaseURL:     "https://pay.example.test",
		EncryptionKey:     db.Key,
		ListenAddr:        "127.0.0.1:0",
		DataDir:           t.TempDir(),
		TrustedProxyCIDRs: append([]netip.Prefix(nil), trusted...),
	}
	application, err := app.NewWithDependencies(context.Background(), cfg, app.Dependencies{
		Clock:      db.Clock,
		Mailer:     mailer,
		Turnstile:  siteverify,
		HTTPClient: network.Client(10 * time.Second),
		DatabaseFactory: func(context.Context, config.Config) (app.Database, error) {
			return db.Pool, nil
		},
	})
	if err != nil {
		t.Fatalf("construct HTTP fixture with real app services: %v", err)
	}
	fixture := &HTTPFixture{
		DB:         db,
		App:        application,
		Handler:    httpx.NewRouter(application),
		DataDir:    cfg.DataDir,
		Mailer:     mailer,
		Network:    network,
		Siteverify: siteverify,
		jars:       make(map[string]*cookiejar.Jar),
		csrf:       make(map[string]string),
	}
	t.Cleanup(func() { _ = application.Close() })
	return fixture
}

// Request performs a JSON request as a guest, merchant, admin, or bound device.
// Use role forms merchant:<client> and admin:<client> for independent cookie
// jars; use device:<token> to supply only the device Bearer credential.
func (f *HTTPFixture) Request(t *testing.T, method, path string, body any, role string) *httptest.ResponseRecorder {
	t.Helper()
	request, err := buildFixtureRequest(method, path, body)
	if err != nil {
		t.Fatalf("construct fixture request: %v", err)
	}
	request.RemoteAddr = fixtureClientRemoteAddr
	request.URL.Scheme = "https"
	request.URL.Host = "pay.example.test"
	request.Host = "pay.example.test"
	request.TLS = &tls.ConnectionState{}
	request.Header.Set("Accept", "application/json")
	actor, client := splitFixtureRole(role)
	switch actor {
	case "merchant", "admin":
		for _, cookie := range f.jar(client).Cookies(request.URL) {
			request.AddCookie(cookie)
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			request.Header.Set("Origin", fixtureOrigin)
			f.mu.Lock()
			csrf := f.csrf[client]
			f.mu.Unlock()
			if csrf != "" {
				request.Header.Set("X-CSRF-Token", csrf)
			}
		}
	case "device":
		token := client
		if token == "" {
			token = f.DeviceToken
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead && !hasExplicitHeader(request.Header, "Origin") {
		request.Header.Set("Origin", fixtureOrigin)
	}

	recorder := httptest.NewRecorder()
	f.Handler.ServeHTTP(recorder, request)
	ValidateHTTPResponse(t, request, recorder)
	if actor == "merchant" || actor == "admin" {
		f.jar(client).SetCookies(request.URL, recorder.Result().Cookies())
		f.rememberCSRF(client, recorder.Body.Bytes())
	}
	return recorder
}

// RequestWithHeaders supports multipart uploads and exact protocol forms while
// preserving the same real router path and per-client authentication state.
func (f *HTTPFixture) RequestWithHeaders(t *testing.T, method, path string, body io.Reader, role string, headers http.Header) *httptest.ResponseRecorder {
	return f.RequestWithRemoteAddr(t, method, path, body, role, headers, "")
}

// RequestWithRemoteAddr is the same authenticated router path as
// RequestWithHeaders, with an explicit server-side peer address for tests that
// exercise trusted-proxy and protocol-IP behavior.
func (f *HTTPFixture) RequestWithRemoteAddr(t *testing.T, method, path string, body io.Reader, role string, headers http.Header, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	request, err := http.NewRequest(method, fixtureOrigin+path, body)
	if err != nil {
		t.Fatalf("construct fixture request: %v", err)
	}
	if remoteAddr == "" {
		remoteAddr = fixtureClientRemoteAddr
	}
	request.RemoteAddr = remoteAddr
	request.TLS = &tls.ConnectionState{}
	request.Header = headers.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set("Accept", "application/json")
	actor, client := splitFixtureRole(role)
	switch actor {
	case "merchant", "admin":
		for _, cookie := range f.jar(client).Cookies(request.URL) {
			request.AddCookie(cookie)
		}
		if method != http.MethodGet && method != http.MethodHead {
			request.Header.Set("Origin", fixtureOrigin)
			f.mu.Lock()
			if csrf := f.csrf[client]; csrf != "" {
				request.Header.Set("X-CSRF-Token", csrf)
			}
			f.mu.Unlock()
		}
	case "device":
		token := client
		if token == "" {
			token = f.DeviceToken
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	if method != http.MethodGet && method != http.MethodHead && !hasExplicitHeader(request.Header, "Origin") {
		request.Header.Set("Origin", fixtureOrigin)
	}
	recorder := httptest.NewRecorder()
	f.Handler.ServeHTTP(recorder, request)
	ValidateHTTPResponse(t, request, recorder)
	if actor == "merchant" || actor == "admin" {
		f.jar(client).SetCookies(request.URL, recorder.Result().Cookies())
		f.rememberCSRF(client, recorder.Body.Bytes())
	}
	return recorder
}

func (f *HTTPFixture) jar(client string) *cookiejar.Jar {
	if client == "" {
		client = "default"
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if jar := f.jars[client]; jar != nil {
		return jar
	}
	jar, _ := cookiejar.New(nil)
	f.jars[client] = jar
	return jar
}

func (f *HTTPFixture) rememberCSRF(client string, payload []byte) {
	var response struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &response) != nil || response.Data.CSRFToken == "" {
		return
	}
	f.mu.Lock()
	f.csrf[client] = response.Data.CSRFToken
	f.mu.Unlock()
}

func splitFixtureRole(role string) (actor, client string) {
	if role == "" {
		return "", ""
	}
	actor, client, _ = strings.Cut(role, ":")
	return actor, client
}

func hasExplicitHeader(header http.Header, name string) bool {
	for key := range header {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func buildFixtureRequest(method, path string, body any) (*http.Request, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("fixture path must be absolute: %q", path)
	}
	var reader io.Reader
	if body != nil {
		if raw, ok := body.([]byte); ok {
			reader = bytes.NewReader(raw)
		} else {
			encoded, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			reader = bytes.NewReader(encoded)
		}
	}
	request, err := http.NewRequest(method, fixtureOrigin+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		if _, ok := body.([]byte); !ok {
			request.Header.Set("Content-Type", "application/json")
		}
	}
	return request, nil
}
