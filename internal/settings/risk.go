package settings

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"vanillapay/internal/platform"
)

func normalizeRisk(risk *platform.PaymentRisk) error {
	if risk.IPDailyOrderLimit < 0 {
		return platform.InvalidJSONField("value.ip_daily_order_limit")
	}
	if len(risk.IPBlacklist) > 500 {
		return platform.InvalidJSONField("value.ip_blacklist")
	}
	normalized := make([]string, 0, len(risk.IPBlacklist))
	seen := make(map[string]bool)
	for _, entry := range risk.IPBlacklist {
		prefix, err := platform.NormalizeIPPrefix(entry)
		if err != nil {
			return platform.InvalidJSONField("value.ip_blacklist")
		}
		value := prefix.String()
		if !seen[value] {
			seen[value] = true
			normalized = append(normalized, value)
		}
	}
	risk.IPBlacklist = normalized
	if len(risk.NameBlockWords) > 200 {
		return platform.InvalidJSONField("value.name_block_words")
	}
	for _, word := range risk.NameBlockWords {
		if strings.TrimSpace(word) == "" || utf8.RuneCountInString(word) > 50 {
			return platform.InvalidJSONField("value.name_block_words")
		}
	}
	return nil
}

func validateCheckoutTurnstile(ctx context.Context, tx pgx.Tx) error {
	var siteKey string
	var secret bool
	err := tx.QueryRow(ctx, `SELECT COALESCE(security->>'turnstile_site_key',''),
		EXISTS(SELECT 1 FROM secret_settings WHERE name='turnstile_secret' AND octet_length(encrypted_value)>0)
		FROM site_settings WHERE singleton`).Scan(&siteKey, &secret)
	if err != nil {
		return err
	}
	if strings.TrimSpace(siteKey) == "" || !secret {
		return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400, Fields: map[string]string{"value.checkout_turnstile_enabled": "turnstile_not_configured"}}
	}
	return nil
}
