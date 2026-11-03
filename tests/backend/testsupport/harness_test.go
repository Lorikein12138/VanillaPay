package testsupport

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

func TestUnit_BarrierReleaseAndCancellation(t *testing.T) {
	barrier := NewBarrier(2)
	first := make(chan error, 1)
	go func() { first <- barrier.Wait(context.Background()) }()
	if err := barrier.Wait(context.Background()); err != nil {
		t.Fatalf("second barrier participant: %v", err)
	}
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("first barrier participant: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("barrier did not release its first participant")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewBarrier(2).Wait(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled barrier wait = %v, want context.Canceled", err)
	}
}

func TestUnit_HTTPRequestConstructionAndIsolation(t *testing.T) {
	request, err := buildFixtureRequest(http.MethodPost, "/api/v1/auth/login", map[string]string{"identifier": "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if request.URL.String() != fixtureOrigin+"/api/v1/auth/login" || request.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("constructed request = %s headers=%v", request.URL, request.Header)
	}
	if _, err := buildFixtureRequest(http.MethodGet, "api/v1/auth/login", nil); err == nil {
		t.Fatal("relative request path was accepted")
	}
	if actor, client := splitFixtureRole("merchant:m2"); actor != "merchant" || client != "m2" {
		t.Fatalf("split role = %q/%q", actor, client)
	}

	fixture := &HTTPFixture{jars: make(map[string]*cookiejar.Jar), csrf: make(map[string]string)}
	first := fixture.jar("merchant:first")
	second := fixture.jar("merchant:second")
	first.SetCookies(request.URL, []*http.Cookie{{Name: "vp_merchant", Value: "one", Path: "/"}})
	second.SetCookies(request.URL, []*http.Cookie{{Name: "vp_merchant", Value: "two", Path: "/"}})
	if got := fixture.jar("merchant:first").Cookies(request.URL)[0].Value; got != "one" {
		t.Fatalf("first client cookie jar value = %q", got)
	}
	if got := fixture.jar("merchant:second").Cookies(request.URL)[0].Value; got != "two" {
		t.Fatalf("second client cookie jar value = %q", got)
	}
}

func TestUnit_HTTPFixtureHeaderlessRequestIsSafe(t *testing.T) {
	fixture := &HTTPFixture{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" || r.Header.Get("Origin") != "https://pay.example.test" {
			t.Errorf("headerless request defaults = %#v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"error":null,"request_id":"headerless-fixture"}`))
	})}
	response := fixture.RequestWithRemoteAddr(t, http.MethodPost, "/api/v1/auth/logout", nil, "guest", nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("headerless request status = %d", response.Code)
	}
}
