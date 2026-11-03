package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"vanillapay/internal/platform"
)

func TestUnit_RequestMetadataBeforeAuthentication(t *testing.T) {
	var got platform.RequestMetadata
	var actorPresent bool
	handler := middleware.RequestID(clientRequestMetadata([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = platform.RequestMetadataFromContext(r.Context())
		_, actorPresent = platform.ActorFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/config", nil)
	req.RemoteAddr = "10.0.0.2:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.0.0.1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
	if got.IP != "198.51.100.9" || got.RequestID == "" {
		t.Fatalf("metadata = %#v", got)
	}
	if actorPresent {
		t.Fatal("request actor must not be created before authentication")
	}
}

func TestUnit_RequestMetadataContextIsolation(t *testing.T) {
	type observed struct {
		remote string
		meta   platform.RequestMetadata
	}
	results := make(chan observed, 2)
	gate := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(2)
	handler := middleware.RequestID(clientRequestMetadata([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ready.Done()
		<-gate
		meta, _ := platform.RequestMetadataFromContext(r.Context())
		results <- observed{remote: r.RemoteAddr, meta: meta}
	})))
	for _, remote := range []string{"10.0.0.2:41001", "192.0.2.9:41002"} {
		go func(remote string) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/public/config", nil)
			req.RemoteAddr = remote
			req.Header.Set("X-Forwarded-For", "203.0.113.90")
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}(remote)
	}
	ready.Wait()
	close(gate)
	seen := map[string]platform.RequestMetadata{}
	for range 2 {
		value := <-results
		seen[value.remote] = value.meta
	}
	trusted, ok := seen["10.0.0.2:41001"]
	if !ok || trusted.IP != "203.0.113.90" || trusted.RequestID == "" {
		t.Fatalf("trusted request metadata = %#v, present=%t", trusted, ok)
	}
	untrusted, ok := seen["192.0.2.9:41002"]
	if !ok || untrusted.IP != "192.0.2.9" || untrusted.RequestID == "" || untrusted.RequestID == trusted.RequestID {
		t.Fatalf("untrusted/concurrent request metadata = %#v, present=%t", untrusted, ok)
	}
}

func TestUnit_AttachActorMetadata(t *testing.T) {
	ctx := platform.WithRequestMetadata(context.Background(), platform.RequestMetadata{IP: "::ffff:203.0.113.6", RequestID: "rid-17"})
	original := platform.Actor{Role: "admin", SubjectID: 9, MerchantID: 3, RecoveryOnly: true, RecoveryChallengeID: 42}
	got := attachActorMetadata(ctx, original)
	if got.Role != original.Role || got.SubjectID != original.SubjectID || got.MerchantID != original.MerchantID || !got.RecoveryOnly || got.RecoveryChallengeID != 42 {
		t.Fatalf("authentication fields changed: %#v", got)
	}
	if got.IP != "203.0.113.6" || got.RequestID != "rid-17" {
		t.Fatalf("request metadata not attached: %#v", got)
	}
}

type unitDeviceAuthenticator struct{}

func (unitDeviceAuthenticator) Authenticate(context.Context, string) (platform.DeviceIdentity, error) {
	return platform.DeviceIdentity{DeviceID: 73, MerchantID: 19, PublicID: "device-public"}, nil
}

func TestUnit_RequireDeviceAttachesDeviceActor(t *testing.T) {
	ctx := platform.WithRequestMetadata(context.Background(), platform.RequestMetadata{IP: "192.0.2.10", RequestID: "device-request"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/device/receipts", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer device-token")
	response := httptest.NewRecorder()
	RequireDevice(unitDeviceAuthenticator{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := platform.ActorFromContext(r.Context())
		if !ok || actor.Role != "device" || actor.SubjectID != 73 || actor.MerchantID != 19 || actor.IP != "192.0.2.10" || actor.RequestID != "device-request" {
			t.Errorf("device actor = %#v, present=%t", actor, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}
