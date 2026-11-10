package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"vanillapay/internal/app"
	"vanillapay/internal/auth"
	"vanillapay/internal/device"
	"vanillapay/internal/merchant"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/internal/settings"
	"vanillapay/internal/subscription"
	"vanillapay/internal/webassets"
)

// NewRouter builds the versioned website API, public payment protocol routes,
// signed media endpoint, health endpoints, and the SPA fallback.
func NewRouter(a *app.App) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	var trustedProxies []netip.Prefix
	if a != nil {
		trustedProxies = a.TrustedProxyCIDRs()
	}
	router.Use(clientRequestMetadata(trustedProxies))
	router.Use(middleware.Recoverer)
	router.Use(noStoreAPIResponses)
	router.Use(apiAuthentication(a))
	router.Get("/health/live", healthLive)
	router.Get("/health/ready", healthReady(a))

	if a != nil && a.Services != nil {
		s := a.Services
		api := chi.NewRouter()
		// An API route miss must not inherit the parent's SPA fallback.
		// chi still handles unsupported methods on existing paths itself.
		api.NotFound(apiNotFound)
		if s.Auth != nil {
			(&auth.Handler{Service: s.Auth, SecureCookies: a.AppEnvironment() != "development"}).RegisterRoutes(api)
		}
		if s.Merchant != nil {
			(&merchant.Handler{Service: s.Merchant, PublicBaseURL: a.PublicBaseURL()}).RegisterRoutes(api)
		}
		if s.Settings != nil {
			settingsHandler := &settings.Handler{Service: s.Settings, SecureCookies: a.AppEnvironment() != "development"}
			settingsHandler.RegisterRoutes(api)
			router.Get("/media/{asset_id}", settingsHandler.OpenAsset)
		}
		if s.Subscription != nil {
			(&subscription.Handler{Service: s.Subscription}).RegisterRoutes(api)
		}
		if s.Payment != nil {
			pay := &payment.Handler{Service: s.Payment, PublicBaseURL: a.PublicBaseURL(), TrustedProxyCIDRs: a.TrustedProxyCIDRs()}
			pay.RegisterAPIRoutes(api)
			pay.RegisterProtocolRoutes(router)
		}
		if s.Device != nil {
			(&device.Handler{Service: s.Device}).RegisterRoutes(api)
		}
		if s.Portal != nil {
			(&portal.Handler{Service: s.Portal}).RegisterRoutes(api)
		}
		if s.Settings != nil && s.Device != nil {
			api.Get("/merchant/collection", func(w http.ResponseWriter, r *http.Request) {
				actor, _ := platform.ActorFromContext(r.Context())
				codes, err := s.Settings.QRSlots(r.Context(), actor)
				if err != nil {
					writeAPIResult(w, r, nil, err, http.StatusOK)
					return
				}
				collection, err := s.Settings.GetCollection(r.Context(), actor)
				if err != nil {
					writeAPIResult(w, r, nil, err, http.StatusOK)
					return
				}
				deviceStatus, err := s.Device.Device(r.Context(), actor)
				writeAPIResult(w, r, map[string]any{"codes": codes, "settings": collection, "device": deviceStatus}, err, http.StatusOK)
			})
		}
		router.Mount("/api/v1", api)
	}

	router.Handle("/api", http.HandlerFunc(apiNotFound))
	router.Handle("/api/*", http.HandlerFunc(apiNotFound))
	router.Handle("/downloads", http.NotFoundHandler())
	router.Handle("/downloads/*", downloadHandler(a.DownloadsDir()))
	spa := webassets.SPAHandler(webassets.FS())
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/health/") || strings.HasPrefix(r.URL.Path, "/media/") {
			http.NotFound(w, r)
			return
		}
		spa.ServeHTTP(w, r)
	})
	router.Handle("/", spa)
	return router
}

func apiAuthentication(a *app.App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a != nil {
				r = r.WithContext(withTrustedProxies(r.Context(), a.TrustedProxyCIDRs()))
				r = r.WithContext(withSecureCookies(r.Context(), a.AppEnvironment() != "development"))
			}
			if strings.HasPrefix(r.URL.Path, "/media/") {
				cookieName := "vp_merchant"
				if secureCookies(r.Context()) {
					cookieName = "__Host-" + cookieName
				}
				if _, err := r.Cookie(cookieName); err == nil {
					var authService *auth.Service
					if a != nil && a.Services != nil {
						authService = a.Services.Auth
					}
					RequireMerchant(authService, next).ServeHTTP(w, r)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
				next.ServeHTTP(w, r)
				return
			}
			path := strings.TrimPrefix(r.URL.Path, "/api/v1")
			var authService *auth.Service
			if a != nil && a.Services != nil {
				authService = a.Services.Auth
			}
			if path == "/auth/email-codes" && r.Method == http.MethodPost {
				if r.Body == nil {
					writeAPIError(w, r, http.StatusBadRequest, "VALIDATION_ERROR")
					return
				}
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
				if err != nil {
					writeAPIError(w, r, http.StatusBadRequest, "VALIDATION_ERROR")
					return
				}
				var request auth.EmailCodeRequest
				if err := platform.DecodeStrictJSON(bytes.NewReader(body), &request); err != nil {
					writePlatformError(w, r, err)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				if merchantEmailCodePurpose(request.Purpose) {
					RequireMerchant(authService, next).ServeHTTP(w, r)
					return
				}
			}
			if path == "/console/recovery" {
				if r.Method != http.MethodPost || !sameOrigin(r) {
					writeAPIError(w, r, http.StatusForbidden, "CSRF_FAILED")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if path == "/console/settings/security" && r.Method == http.MethodPut {
				recoveryCookieName := "vp_recovery"
				if secureCookies(r.Context()) {
					recoveryCookieName = "__Host-" + recoveryCookieName
				}
				if recoveryCookie, err := r.Cookie(recoveryCookieName); err == nil {
					if authService == nil || !sameOrigin(r) {
						writeAPIError(w, r, http.StatusForbidden, "CSRF_FAILED")
						return
					}
					challengeID, err := authService.ResolveConsoleRecovery(r.Context(), recoveryCookie.Value, r.Header.Get("X-CSRF-Token"))
					if err != nil {
						writePlatformError(w, r, err)
						return
					}
					actor := attachActorMetadata(r.Context(), platform.Actor{Role: "admin", SubjectID: 1, RecoveryOnly: true, RecoveryChallengeID: challengeID})
					ctx := platform.WithActor(r.Context(), actor)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			if path == "/device/heartbeat" || path == "/device/receipts" || path == "/device/unbind" {
				var deviceService DeviceAuthenticator
				if a != nil && a.Services != nil {
					deviceService = a.Services.Device
				}
				RequireDevice(deviceService, next).ServeHTTP(w, r)
				return
			}
			merchantOnly := strings.HasPrefix(path, "/merchant/") || path == "/auth/logout"
			adminOnly := strings.HasPrefix(path, "/console/") && path != "/console/bootstrap" && path != "/console/bootstrap-status" && path != "/console/login" && path != "/console/second-factor" && path != "/console/recovery"
			if merchantOnly {
				RequireMerchant(authService, next).ServeHTTP(w, r)
				return
			}
			if adminOnly || path == "/console/logout" {
				RequireAdmin(authService, next).ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func merchantEmailCodePurpose(purpose string) bool {
	switch purpose {
	case "change_username", "change_email_old", "change_email_new", "change_password", "reset_key":
		return true
	default:
		return false
	}
}

func healthLive(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func healthReady(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a == nil || a.Ready(r.Context()) != nil {
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func apiNotFound(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "NOT_FOUND"}})
}

func noStoreAPIResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIResult(w http.ResponseWriter, r *http.Request, data any, err error, successStatus int) {
	if err == nil {
		writeJSON(w, successStatus, map[string]any{"data": data, "error": nil, "request_id": middleware.GetReqID(r.Context())})
		return
	}
	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	var apiErr *platform.Error
	if errors.As(err, &apiErr) {
		code = apiErr.Code
		if apiErr.HTTPStatus > 0 {
			status = apiErr.HTTPStatus
		}
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": code, "fields": map[string]string{}, "details": map[string]any{}}, "request_id": middleware.GetReqID(r.Context())})
}
