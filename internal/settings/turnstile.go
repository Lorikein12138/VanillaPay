package settings

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/jackc/pgx/v5"
	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
)

// Verify against a versioned snapshot before taking write locks. The write
// transaction checks that same version again before persisting any changes.
func (s *Service) verifySecurityUpdate(ctx context.Context, version int64, clean, replacement json.RawMessage, token string) error {
	var next SecurityGroupRequest
	if err := json.Unmarshal(clean, &next); err != nil {
		return err
	}
	if !next.TurnstileEnabled {
		return nil
	}
	var enabled bool
	var siteKey string
	var cipher []byte
	err := s.pool.QueryRow(ctx, `SELECT (security->>'turnstile_enabled')::boolean, security->>'turnstile_site_key', (SELECT encrypted_value FROM secret_settings WHERE name='turnstile_secret') FROM site_settings WHERE singleton AND version=$1`, version).Scan(&enabled, &siteKey, &cipher)
	if errors.Is(err, pgx.ErrNoRows) {
		return &platform.Error{Code: "VERSION_CONFLICT", HTTPStatus: 409}
	}
	if err != nil {
		return err
	}
	var secret string
	if replacement != nil {
		if err := json.Unmarshal(replacement, &secret); err != nil {
			return err
		}
	} else if len(cipher) > 0 {
		plain, err := platform.Decrypt(s.key[:], cipher)
		if err != nil {
			return &platform.Error{Code: "TURNSTILE_UNAVAILABLE", HTTPStatus: 503}
		}
		secret = string(plain)
	}
	if secret == "" {
		return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	if enabled && siteKey == next.TurnstileSiteKey && replacement == nil {
		return nil
	}
	publicURL, err := url.Parse(s.publicBaseURL)
	if err != nil || publicURL.Hostname() == "" || s.turnstile == nil {
		return &platform.Error{Code: "TURNSTILE_UNAVAILABLE", HTTPStatus: 503}
	}
	return auth.VerifyTurnstile(ctx, s.turnstile, secret, token, "settings_security", publicURL.Hostname())
}
