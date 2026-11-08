package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"vanillapay/internal/platform"
)

type Handler struct {
	Service       *Service
	SecureCookies bool
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/console/bootstrap-status", h.bootstrapStatus)
	r.Post("/console/bootstrap", h.bootstrap)
	r.Post("/console/recovery", h.consoleRecovery)
	r.Post("/auth/email-codes", h.sendEmailCode)
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.loginMerchant)
	r.Post("/console/login", h.loginAdmin)
	r.Post("/auth/second-factor", h.secondFactorMerchant)
	r.Post("/console/second-factor", h.secondFactorAdmin)
	r.Post("/auth/reset-password", h.resetPassword)
	r.Get("/merchant/me", h.merchantMe)
	r.Get("/console/me", h.adminMe)
	r.Post("/auth/logout", h.merchantLogout)
	r.Post("/console/logout", h.adminLogout)
	r.Get("/merchant/banned", h.banned)
	r.Patch("/merchant/profile/username", h.changeUsername)
	r.Patch("/merchant/profile/email", h.changeEmail)
	r.Patch("/merchant/profile/password", h.changePassword)
	r.Patch("/merchant/preferences", h.merchantPreferences)
	r.Patch("/console/preferences", h.adminPreferences)
	r.Post("/merchant/2fa/setup", h.merchantTOTPSetup)
	r.Post("/console/2fa/setup", h.adminTOTPSetup)
	r.Post("/merchant/2fa/confirm", h.merchantTOTPConfirm)
	r.Post("/console/2fa/confirm", h.adminTOTPConfirm)
	r.Delete("/merchant/2fa", h.merchantTOTPDisable)
	r.Delete("/console/2fa", h.adminTOTPDisable)
	r.Patch("/console/password", h.adminPassword)
}

func (h *Handler) consoleRecovery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RecoveryCode string `json:"recovery_code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	session, err := h.Service.RecoverConsole(r.Context(), req.RecoveryCode)
	if err != nil {
		writeResult(w, r, nil, err, http.StatusOK)
		return
	}
	name := "vp_recovery"
	if h.SecureCookies {
		name = "__Host-" + name
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: session.Token, Path: "/", HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode, Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds())})
	w.Header().Set("Cache-Control", "no-store")
	writeResult(w, r, map[string]any{"csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt, "repair_only": true, "settings_version": session.SettingsVersion, "security": session.Security}, nil, http.StatusOK)
}

func (h *Handler) bootstrapStatus(w http.ResponseWriter, r *http.Request) {
	data, err := h.Service.BootstrapStatus(r.Context())
	writeResult(w, r, data, err, http.StatusOK)
}
func (h *Handler) bootstrap(w http.ResponseWriter, r *http.Request) {
	var req BootstrapRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := h.Service.Bootstrap(r.Context(), req)
	if err != nil {
		writeResult(w, r, nil, err, http.StatusOK)
		return
	}
	h.setSession(w, "admin", result)
	writeResult(w, r, result, nil, http.StatusCreated)
}
func (h *Handler) sendEmailCode(w http.ResponseWriter, r *http.Request) {
	var req EmailCodeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor := platform.Actor{}
	if req.Purpose != "register" && req.Purpose != "reset_password" {
		actor = h.actorForEmailCode(r)
	}
	result, err := h.Service.SendEmailCode(r.Context(), actor, req)
	details := map[string]any{}
	var rateLimitError *emailCodeRateLimitError
	if errors.As(err, &rateLimitError) {
		details["retry_after_seconds"] = rateLimitError.RetryAfterSeconds
	}
	writeResultWithDetails(w, r, result, err, http.StatusOK, details)
}
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := h.Service.Register(r.Context(), req)
	writeResult(w, r, map[string]any{"user": result, "next_path": "/login"}, err, http.StatusCreated)
}
func (h *Handler) loginMerchant(w http.ResponseWriter, r *http.Request) { h.login(w, r, "merchant") }
func (h *Handler) loginAdmin(w http.ResponseWriter, r *http.Request)    { h.login(w, r, "admin") }
func (h *Handler) login(w http.ResponseWriter, r *http.Request, role string) {
	var req LoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := h.Service.Login(r.Context(), role, req)
	var data any = result
	if err == nil && result.Session != nil {
		h.setSession(w, role, *result.Session)
		data = *result.Session
	}
	writeResult(w, r, data, err, http.StatusOK)
}
func (h *Handler) secondFactorMerchant(w http.ResponseWriter, r *http.Request) {
	h.secondFactor(w, r, "merchant")
}
func (h *Handler) secondFactorAdmin(w http.ResponseWriter, r *http.Request) {
	h.secondFactor(w, r, "admin")
}
func (h *Handler) secondFactor(w http.ResponseWriter, r *http.Request, role string) {
	var req FactorRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := h.Service.VerifySecondFactor(r.Context(), req)
	if err == nil {
		h.setSession(w, role, result)
	}
	writeResult(w, r, result, err, http.StatusOK)
}
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	err := h.Service.ResetPassword(r.Context(), req)
	writeResult(w, r, nil, err, http.StatusOK)
}

func (h *Handler) setSession(w http.ResponseWriter, role string, result SessionResult) {
	name := "vp_merchant"
	ttl := 7 * 24 * time.Hour
	if role == "admin" {
		name = "vp_console"
		ttl = 12 * time.Hour
	}
	if h.SecureCookies {
		name = "__Host-" + name
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: result.Token, Path: "/", HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: result.ExpiresAt, MaxAge: int(ttl.Seconds())})
	http.SetCookie(w, &http.Cookie{Name: h.csrfCookieName(role), Value: result.CSRFToken, Path: "/", Secure: h.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: result.ExpiresAt, MaxAge: int(ttl.Seconds())})
	w.Header().Set("Cache-Control", "no-store")
}

func (h *Handler) csrfCookieName(role string) string {
	name := "vp_csrf_merchant"
	if role == "admin" {
		name = "vp_csrf_console"
	}
	if h.SecureCookies {
		name = "__Host-" + name
	}
	return name
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := platform.DecodeJSONBody(w, r, 1<<20, dst); err != nil {
		writeResult(w, r, nil, err, http.StatusBadRequest)
		return false
	}
	return true
}

func writeResult(w http.ResponseWriter, r *http.Request, data any, err error, successStatus int) {
	writeResultWithDetails(w, r, data, err, successStatus, nil)
}

func writeResultWithDetails(w http.ResponseWriter, r *http.Request, data any, err error, successStatus int, errorDetails map[string]any) {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = middleware.GetReqID(r.Context())
	}
	status := successStatus
	body := map[string]any{"data": data, "error": nil, "request_id": requestID}
	if err != nil {
		status = http.StatusInternalServerError
		var apiErr *platform.Error
		if errors.As(err, &apiErr) && apiErr.HTTPStatus > 0 {
			status = apiErr.HTTPStatus
		}
		code := "INTERNAL_ERROR"
		message := "INTERNAL_ERROR"
		fields := map[string]string{}
		if errors.As(err, &apiErr) {
			code = apiErr.Code
			message = apiErr.Code
			fields = apiErr.Fields
			if fields == nil {
				fields = map[string]string{}
			}
		}
		if errorDetails == nil {
			errorDetails = map[string]any{}
		}
		body = map[string]any{"error": map[string]any{"code": code, "message": message, "fields": fields, "details": errorDetails}, "request_id": requestID}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
