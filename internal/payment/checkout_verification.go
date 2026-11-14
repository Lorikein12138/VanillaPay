package payment

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
)

type checkoutRisk struct {
	SiteKey string
	Secret  []byte
}

type checkoutRiskReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readCheckoutRisk(ctx context.Context, db checkoutRiskReader, ip string) (checkoutRisk, error) {
	var risk checkoutRisk
	var blacklist []string
	var enabled bool
	err := db.QueryRow(ctx, `SELECT COALESCE(ARRAY(SELECT jsonb_array_elements_text(to_jsonb(s)->'risk'->'ip_blacklist')),ARRAY[]::text[]),
		COALESCE((to_jsonb(s)->'risk'->>'checkout_turnstile_enabled')::boolean,false),
		COALESCE(security->>'turnstile_site_key',''),
		(SELECT encrypted_value FROM secret_settings WHERE name='turnstile_secret')
		FROM site_settings s WHERE singleton`).Scan(&blacklist, &enabled, &risk.SiteKey, &risk.Secret)
	if err != nil {
		return checkoutRisk{}, err
	}
	if err := enforceCheckoutIP(ip, blacklist); err != nil {
		return checkoutRisk{}, err
	}
	if !enabled || strings.TrimSpace(risk.SiteKey) == "" || len(risk.Secret) == 0 {
		return checkoutRisk{}, nil
	}
	return risk, nil
}

func (s *Service) checkoutVerified(trade, token string, expires time.Time) bool {
	return platform.CheckoutVerified(s.key[:], trade, token, s.clock.Now(), expires)
}

func (h *Handler) VerifyCheckout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	risk, err := readCheckoutRisk(ctx, h.Service.pool, h.realIP(r))
	if err != nil {
		httpJSON(w, r, nil, err, 200)
		return
	}
	var req CheckoutVerifyRequest
	if decodePaymentJSON(w, r, &req) != nil {
		return
	}
	if !h.sameOrigin(r) {
		httpJSON(w, r, nil, apiErr("CSRF_FAILED", 403), 200)
		return
	}
	trade := chi.URLParam(r, "tradeNo")
	var state string
	var expires time.Time
	err = h.Service.pool.QueryRow(ctx, `SELECT status,expires_at FROM orders WHERE trade_no=$1`, trade).Scan(&state, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		err = apiErr("NOT_FOUND", 404)
	}
	if err != nil {
		httpJSON(w, r, nil, err, 200)
		return
	}
	if risk.SiteKey == "" || state != "pending" || !expires.After(h.Service.clock.Now()) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	origin, parseErr := url.Parse(h.Service.publicBaseURL)
	if parseErr != nil || origin.Hostname() == "" || h.Service.turnstile == nil {
		httpJSON(w, r, nil, apiErr("TURNSTILE_UNAVAILABLE", 503), 200)
		return
	}
	secret, err := platform.Decrypt(h.Service.key[:], risk.Secret)
	if err != nil {
		httpJSON(w, r, nil, apiErr("TURNSTILE_UNAVAILABLE", 503), 200)
		return
	}
	if err := auth.VerifyTurnstile(ctx, h.Service.turnstile, string(secret), req.TurnstileToken, "checkout", origin.Hostname()); err != nil {
		httpJSON(w, r, nil, err, 200)
		return
	}
	if expires.Unix() > h.Service.clock.Now().Unix() {
		http.SetCookie(w, &http.Cookie{
			Name: platform.CheckoutCookieName(trade), Value: platform.CheckoutCookieValue(h.Service.key[:], trade, expires.Unix()),
			Path: "/", Expires: time.Unix(expires.Unix(), 0),
			HttpOnly: true, Secure: origin.Scheme == "https" || r.TLS != nil || strings.HasPrefix(r.Header.Get("Origin"), "https://"),
			SameSite: http.SameSiteLaxMode,
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
