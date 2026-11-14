package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
)

type userContextKey struct{}
type trustedProxiesContextKey struct{}
type secureCookiesContextKey struct{}

func withTrustedProxies(ctx context.Context, prefixes []netip.Prefix) context.Context {
	return context.WithValue(ctx, trustedProxiesContextKey{}, append([]netip.Prefix(nil), prefixes...))
}
func withSecureCookies(ctx context.Context, secure bool) context.Context {
	return context.WithValue(ctx, secureCookiesContextKey{}, secure)
}
func secureCookies(ctx context.Context) bool {
	secure, ok := ctx.Value(secureCookiesContextKey{}).(bool)
	return !ok || secure
}

func ActorFromContext(ctx context.Context) (platform.Actor, bool) {
	return platform.ActorFromContext(ctx)
}

func clientRequestMetadata(trusted []netip.Prefix) func(http.Handler) http.Handler {
	trusted = append([]netip.Prefix(nil), trusted...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := platform.ResolveClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), trusted)
			ipText := ""
			if ip.IsValid() {
				ipText = ip.String()
			}
			metadata := platform.RequestMetadata{IP: ipText, RequestID: middleware.GetReqID(r.Context())}
			next.ServeHTTP(w, r.WithContext(platform.WithRequestMetadata(r.Context(), metadata)))
		})
	}
}

func attachActorMetadata(ctx context.Context, actor platform.Actor) platform.Actor {
	if metadata, ok := platform.RequestMetadataFromContext(ctx); ok {
		actor.IP = metadata.IP
		actor.RequestID = metadata.RequestID
	}
	return actor
}

func UserFromContext(ctx context.Context) (auth.UserDTO, bool) {
	user, ok := ctx.Value(userContextKey{}).(auth.UserDTO)
	return user, ok
}

func RequireMerchant(service *auth.Service, next http.Handler) http.Handler {
	return requireRole(service, "merchant", next)
}

func RequireAdmin(service *auth.Service, next http.Handler) http.Handler {
	return requireRole(service, "admin", next)
}

type DeviceAuthenticator interface {
	Authenticate(context.Context, string) (platform.DeviceIdentity, error)
}

func RequireDevice(service DeviceAuthenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.SplitN(authorization, " ", 2)
		if service == nil || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			writeAPIError(w, r, http.StatusUnauthorized, "DEVICE_UNAUTHORIZED")
			return
		}
		identity, err := service.Authenticate(r.Context(), strings.TrimSpace(parts[1]))
		if err != nil {
			writePlatformError(w, r, err)
			return
		}
		ctx := platform.WithDevice(r.Context(), identity)
		actor := platform.Actor{Role: "device", SubjectID: identity.DeviceID, MerchantID: identity.MerchantID}
		ctx = platform.WithActor(ctx, attachActorMetadata(ctx, actor))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Recover protects route handlers from panics without exposing panic values.
func Recover(next http.Handler) http.Handler {
	return middleware.Recoverer(next)
}

func requireRole(service *auth.Service, role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookieName := "vp_merchant"
		if role == "admin" {
			cookieName = "vp_console"
		}
		if secureCookies(r.Context()) {
			cookieName = "__Host-" + cookieName
		}
		cookie, err := r.Cookie(cookieName)
		if service == nil {
			writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED")
			return
		}
		if err != nil {
			if hasAuthenticatedOppositeRoleSession(service, r, role) {
				writeAPIError(w, r, http.StatusForbidden, "FORBIDDEN")
				return
			}
			writeAPIError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED")
			return
		}
		write := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		csrf := r.Header.Get("X-CSRF-Token")
		identity, err := service.ResolveSession(r.Context(), cookie.Value, role, csrf, write)
		if err != nil {
			var apiErr *platform.Error
			if errors.As(err, &apiErr) && apiErr.Code == "UNAUTHENTICATED" && hasAuthenticatedOppositeRoleSession(service, r, role) {
				writeAPIError(w, r, http.StatusForbidden, "FORBIDDEN")
				return
			}
			writePlatformError(w, r, err)
			return
		}
		if write && !sameOrigin(r) {
			writeAPIError(w, r, http.StatusForbidden, "CSRF_FAILED")
			return
		}
		if identity.User.Banned && role == "merchant" && !allowedForBannedMerchant(r.URL.Path) {
			writeAPIError(w, r, http.StatusForbidden, "ACCOUNT_BANNED")
			return
		}
		if role == "merchant" && !allowedForBannedMerchant(r.URL.Path) {
			switch identity.User.ApprovalState {
			case "pending":
				writeAPIError(w, r, http.StatusForbidden, "ACCOUNT_PENDING_REVIEW")
				return
			case "rejected":
				writeAPIError(w, r, http.StatusForbidden, "REGISTRATION_REJECTED")
				return
			}
		}
		identity.Actor = attachActorMetadata(r.Context(), identity.Actor)
		ctx := platform.WithActor(r.Context(), identity.Actor)
		ctx = context.WithValue(ctx, userContextKey{}, identity.User)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func hasAuthenticatedOppositeRoleSession(service *auth.Service, r *http.Request, role string) bool {
	oppositeRole := "admin"
	oppositeCookieName := "vp_console"
	if role == "admin" {
		oppositeRole = "merchant"
		oppositeCookieName = "vp_merchant"
	}
	if secureCookies(r.Context()) {
		oppositeCookieName = "__Host-" + oppositeCookieName
	}
	cookie, err := r.Cookie(oppositeCookieName)
	if err != nil || service == nil {
		return false
	}
	_, err = service.ResolveSession(r.Context(), cookie.Value, oppositeRole, "", false)
	return err == nil
}

func allowedForBannedMerchant(path string) bool {
	switch path {
	case "/api/v1/merchant/me", "/api/v1/merchant/banned", "/api/v1/merchant/preferences", "/api/v1/auth/logout":
		return true
	default:
		return false
	}
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if remote := platform.ResolveClientIP(r.RemoteAddr, "", nil); remote.IsValid() {
		trusted, _ := r.Context().Value(trustedProxiesContextKey{}).([]netip.Prefix)
		trustedPeer := false
		for _, prefix := range trusted {
			if prefix.Contains(remote) {
				trustedPeer = true
				break
			}
		}
		if trustedPeer {
			forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
			if strings.Contains(forwarded, ",") {
				return false
			}
			if forwarded == "http" || forwarded == "https" {
				scheme = forwarded
			} else if forwarded != "" {
				return false
			}
		}
	}
	return strings.EqualFold(u.Scheme, scheme) && strings.EqualFold(u.Host, r.Host)
}

func RealIP(remoteAddr, forwardedFor string, trusted []netip.Prefix) netip.Addr {
	return platform.ResolveClientIP(remoteAddr, forwardedFor, trusted)
}

func writePlatformError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	fields := map[string]string{}
	apiErr := &platform.Error{}
	if errors.As(err, &apiErr) {
		if apiErr.HTTPStatus > 0 {
			status = apiErr.HTTPStatus
		}
		if apiErr.Code != "" {
			code = apiErr.Code
		}
		if apiErr.Fields != nil {
			fields = apiErr.Fields
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]any{
		"error":      map[string]any{"code": code, "message": code, "fields": fields, "details": map[string]any{}},
		"request_id": middleware.GetReqID(r.Context()),
	})
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code string) {
	writePlatformError(w, r, &platform.Error{Code: code, HTTPStatus: status})
}
