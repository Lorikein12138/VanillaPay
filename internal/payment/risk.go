package payment

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
)

func enforceCheckoutIP(ip string, rules []string) error {
	address, parseErr := netip.ParseAddr(ip)
	if parseErr == nil && blacklisted(address, rules) {
		return apiErr("IP_BLOCKED", 403)
	}
	return nil
}

func blacklisted(address netip.Addr, rules []string) bool {
	for _, rule := range rules {
		if prefix, err := netip.ParsePrefix(rule); err == nil &&
			(prefix.Contains(address.Unmap()) || prefix.Contains(netip.AddrFrom16(address.As16()))) {
			return true
		}
	}
	return false
}

func enforceCreateRisk(ctx context.Context, tx pgx.Tx, in CreateCommand, risk platform.PaymentRisk) error {
	if err := enforceCheckoutIP(in.ClientIP, risk.IPBlacklist); err != nil {
		return err
	}
	if len(risk.NameBlockWords) != 0 {
		name := platform.NormalizePaymentName(in.Name)
		for _, word := range risk.NameBlockWords {
			if strings.Contains(name, platform.NormalizePaymentName(word)) {
				return apiErr("NAME_BLOCKED", 403)
			}
		}
	}
	var enabled bool
	var entries []string
	err := tx.QueryRow(ctx, `SELECT enabled,entries FROM merchant_notify_domains WHERE merchant_id=$1`, in.MerchantID).Scan(&enabled, &entries)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if enabled {
		for _, raw := range []string{in.NotifyURL, in.ReturnURL} {
			if raw == "" {
				continue
			}
			u, err := url.Parse(raw)
			if err != nil || !platform.NotifyDomainAllowed(u.Hostname(), entries) {
				return apiErr("URL_DOMAIN_NOT_ALLOWED", 403)
			}
		}
	}
	return nil
}

func loadCreateRisk(ctx context.Context, tx pgx.Tx) (platform.PaymentRisk, error) {
	var raw []byte
	var risk platform.PaymentRisk
	if err := tx.QueryRow(ctx, `SELECT COALESCE(to_jsonb(s)->'risk','{}'::jsonb) FROM site_settings s WHERE singleton`).Scan(&raw); err != nil {
		return risk, err
	}
	return risk, json.Unmarshal(raw, &risk)
}

func checkCreateLimits(ctx context.Context, tx pgx.Tx, in CreateCommand, limits platform.PlanLimits, ipLimit int32, now func() time.Time) (time.Time, error) {
	if (limits.MinOrderCents != nil && in.OriginalCents < *limits.MinOrderCents) ||
		(limits.MaxOrderCents != nil && in.OriginalCents > *limits.MaxOrderCents) {
		return time.Time{}, apiErr("AMOUNT_OUT_OF_RANGE", 400)
	}
	address, _ := netip.ParseAddr(in.ClientIP) // The create input was already validated.
	countIP := ipLimit > 0 && address.IsValid()
	if countIP {
		address = address.Unmap()
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "order-ip:"+address.String()); err != nil {
			return time.Time{}, err
		}
	}
	created := now()
	if !countIP && limits.DailyLimitCents == nil {
		return created, nil
	}
	loc, err := time.LoadLocation(portal.DefaultStatsTimezone)
	if err != nil {
		return time.Time{}, err
	}
	y, m, d := created.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	if limits.DailyLimitCents != nil {
		var exceeded bool
		if err := tx.QueryRow(ctx, `SELECT
			COALESCE((SELECT sum(original_cents) FROM orders WHERE merchant_id=$1 AND purpose='external' AND status='paid' AND paid_at >= $2 AND paid_at < $3),0)
			+ COALESCE((SELECT sum(original_cents) FROM orders WHERE merchant_id=$1 AND purpose='external' AND status='pending' AND expires_at > $4),0)
			> $5::bigint`, in.MerchantID, start, end, created, *limits.DailyLimitCents-in.OriginalCents).Scan(&exceeded); err != nil {
			return time.Time{}, err
		}
		if exceeded {
			return time.Time{}, apiErr("DAILY_LIMIT_EXCEEDED", 409)
		}
	}
	if countIP {
		// Count both spellings of IPv4 so old mapped addresses share the same limit.
		var count int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM (
			SELECT 1 FROM orders WHERE purpose='external' AND clientip IN ($1::inet,$2::inet)
				AND created_at >= $3 AND created_at < $4 LIMIT $5
		) recent`, address.String(), netip.AddrFrom16(address.As16()).String(), start, end, ipLimit).Scan(&count); err != nil {
			return time.Time{}, err
		}
		if count >= int64(ipLimit) {
			return time.Time{}, apiErr("IP_DAILY_LIMIT_EXCEEDED", 429)
		}
	}
	return created, nil
}
