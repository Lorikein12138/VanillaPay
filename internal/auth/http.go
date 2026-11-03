package auth

import (
	"net/http"
	"vanillapay/internal/platform"
)

func (h *Handler) merchantMe(w http.ResponseWriter, r *http.Request) { h.me(w, r, "merchant") }
func (h *Handler) adminMe(w http.ResponseWriter, r *http.Request)    { h.me(w, r, "admin") }

func (h *Handler) me(w http.ResponseWriter, r *http.Request, role string) {
	token := h.sessionToken(r, role)
	csrfToken := ""
	if cookie, err := r.Cookie(h.csrfCookieName(role)); err == nil {
		csrfToken = cookie.Value
	}
	identity, err := h.Service.ResolveSession(r.Context(), token, role, csrfToken, false)
	if err != nil {
		writeResult(w, r, nil, err, http.StatusOK)
		return
	}
	writeResult(w, r, map[string]any{"user": identity.User, "csrf_token": identity.CSRF, "expires_at": identity.ExpiresAt}, nil, http.StatusOK)
}

func (h *Handler) merchantLogout(w http.ResponseWriter, r *http.Request) { h.logout(w, r, "merchant") }
func (h *Handler) adminLogout(w http.ResponseWriter, r *http.Request)    { h.logout(w, r, "admin") }

func (h *Handler) logout(w http.ResponseWriter, r *http.Request, role string) {
	err := h.Service.Logout(r.Context(), role, h.sessionToken(r, role))
	h.clearSession(w, role)
	writeResult(w, r, nil, err, http.StatusOK)
}

func (h *Handler) sessionToken(r *http.Request, role string) string {
	name := "vp_merchant"
	if role == "admin" {
		name = "vp_console"
	}
	if h.SecureCookies {
		name = "__Host-" + name
	}
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (h *Handler) clearSession(w http.ResponseWriter, role string) {
	name := "vp_merchant"
	if role == "admin" {
		name = "vp_console"
	}
	if h.SecureCookies {
		name = "__Host-" + name
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: h.csrfCookieName(role), Value: "", Path: "/", Secure: h.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func (h *Handler) banned(w http.ResponseWriter, r *http.Request) {
	actor, ok := platform.ActorFromContext(r.Context())
	if !ok {
		writeResult(w, r, nil, appError("UNAUTHENTICATED", http.StatusUnauthorized), http.StatusOK)
		return
	}
	data, err := h.Service.BannedInfo(r.Context(), actor)
	writeResult(w, r, data, err, http.StatusOK)
}

func (h *Handler) changeUsername(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string        `json:"username"`
		Proof    SecurityProof `json:"security_proof"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	user, err := h.Service.ChangeIdentity(r.Context(), actor, IdentityChange{Username: req.Username, EmailCode: req.Proof.EmailCode, TOTPCode: req.Proof.TOTPCode})
	writeResult(w, r, user, err, http.StatusOK)
}

func (h *Handler) changeEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email        string `json:"email"`
		OldEmailCode string `json:"old_email_code"`
		NewEmailCode string `json:"new_email_code"`
		TOTPCode     string `json:"totp_code,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	user, err := h.Service.ChangeIdentity(r.Context(), actor, IdentityChange{Email: req.Email, OldEmailCode: req.OldEmailCode, NewEmailCode: req.NewEmailCode, TOTPCode: req.TOTPCode})
	writeResult(w, r, user, err, http.StatusOK)
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password             string        `json:"password"`
		PasswordConfirmation string        `json:"password_confirmation"`
		Proof                SecurityProof `json:"security_proof"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	_, err := h.Service.ChangeIdentity(r.Context(), actor, IdentityChange{Password: req.Password, Confirmation: req.PasswordConfirmation, EmailCode: req.Proof.EmailCode, TOTPCode: req.Proof.TOTPCode})
	if err == nil {
		h.clearSession(w, "merchant")
	}
	writeResult(w, r, nil, err, http.StatusOK)
}

func (h *Handler) merchantPreferences(w http.ResponseWriter, r *http.Request) {
	h.preferences(w, r, "merchant")
}
func (h *Handler) adminPreferences(w http.ResponseWriter, r *http.Request) {
	h.preferences(w, r, "admin")
}

func (h *Handler) preferences(w http.ResponseWriter, r *http.Request, role string) {
	var req struct {
		Locale string `json:"locale"`
		Theme  string `json:"theme"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, ok := platform.ActorFromContext(r.Context())
	if !ok || actor.Role != role {
		writeResult(w, r, nil, appError("UNAUTHENTICATED", 401), http.StatusOK)
		return
	}
	data, err := h.Service.UpdatePreferences(r.Context(), actor, req.Locale, req.Theme)
	writeResult(w, r, data, err, http.StatusOK)
}

func (h *Handler) merchantTOTPSetup(w http.ResponseWriter, r *http.Request) {
	h.totpSetup(w, r, "merchant")
}
func (h *Handler) adminTOTPSetup(w http.ResponseWriter, r *http.Request) { h.totpSetup(w, r, "admin") }

func (h *Handler) totpSetup(w http.ResponseWriter, r *http.Request, role string) {
	var req struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, ok := platform.ActorFromContext(r.Context())
	if !ok || actor.Role != role {
		writeResult(w, r, nil, appError("UNAUTHENTICATED", 401), http.StatusOK)
		return
	}
	result, err := h.Service.SetupTOTP(r.Context(), actor, req.Password)
	writeResult(w, r, result, err, http.StatusOK)
}

func (h *Handler) merchantTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	h.totpConfirm(w, r, "merchant")
}
func (h *Handler) adminTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	h.totpConfirm(w, r, "admin")
}

func (h *Handler) totpConfirm(w http.ResponseWriter, r *http.Request, role string) {
	var req TOTPConfirm
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, ok := platform.ActorFromContext(r.Context())
	if !ok || actor.Role != role {
		writeResult(w, r, nil, appError("UNAUTHENTICATED", 401), http.StatusOK)
		return
	}
	codes, err := h.Service.ConfirmTOTP(r.Context(), actor, req)
	if err == nil {
		writeResult(w, r, map[string]any{"enabled": true, "recovery_codes": codes}, nil, http.StatusOK)
		return
	}
	writeResult(w, r, nil, err, http.StatusOK)
}

func (h *Handler) merchantTOTPDisable(w http.ResponseWriter, r *http.Request) {
	h.totpDisable(w, r, "merchant")
}
func (h *Handler) adminTOTPDisable(w http.ResponseWriter, r *http.Request) {
	h.totpDisable(w, r, "admin")
}

func (h *Handler) totpDisable(w http.ResponseWriter, r *http.Request, role string) {
	var req FactorProof
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, ok := platform.ActorFromContext(r.Context())
	if !ok || actor.Role != role {
		writeResult(w, r, nil, appError("UNAUTHENTICATED", 401), http.StatusOK)
		return
	}
	err := h.Service.DisableTOTP(r.Context(), actor, req)
	data := any(nil)
	if err == nil {
		data = map[string]any{"enabled": false}
	}
	writeResult(w, r, data, err, http.StatusOK)
}

func (h *Handler) adminPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword      string `json:"current_password"`
		Password             string `json:"password"`
		PasswordConfirmation string `json:"password_confirmation"`
		TOTPCode             string `json:"totp_code,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, ok := platform.ActorFromContext(r.Context())
	if !ok || actor.Role != "admin" {
		writeResult(w, r, nil, appError("UNAUTHENTICATED", 401), http.StatusOK)
		return
	}
	err := h.Service.ChangeAdminPassword(r.Context(), actor, req.CurrentPassword, req.Password, req.PasswordConfirmation, req.TOTPCode)
	if err == nil {
		h.clearSession(w, "admin")
	}
	writeResult(w, r, nil, err, http.StatusOK)
}

func (h *Handler) actorForEmailCode(r *http.Request) platform.Actor {
	for _, role := range []string{"merchant", "admin"} {
		if token := h.sessionToken(r, role); token != "" {
			if identity, err := h.Service.ResolveSession(r.Context(), token, role, "", false); err == nil {
				return identity.Actor
			}
		}
	}
	return platform.Actor{}
}
