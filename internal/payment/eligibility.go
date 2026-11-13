package payment

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"vanillapay/internal/platform"
)

type SQLEligibility struct{ Now func() time.Time }

func (e SQLEligibility) CheckCreateTx(ctx context.Context, tx pgx.Tx, merchantID, amount int64) (EligibilitySnapshot, error) {
	return e.load(ctx, tx, merchantID, amount)
}
func (e SQLEligibility) CheckChannelTx(ctx context.Context, tx pgx.Tx, merchantID int64, channel string, amount int64) (EligibilitySnapshot, error) {
	snapshot, err := e.load(ctx, tx, merchantID, amount)
	if err != nil {
		return snapshot, err
	}
	if !containsChannel(snapshot.Channels, channel) {
		return snapshot, apiErr("CHANNEL_NOT_ALLOWED", 403)
	}
	if snapshot.QRAssets[channel] == "" {
		return snapshot, apiErr("QR_UNAVAILABLE", 422)
	}
	return snapshot, nil
}

func (e SQLEligibility) load(ctx context.Context, tx pgx.Tx, merchantID, amount int64) (EligibilitySnapshot, error) {
	var result EligibilitySnapshot
	var bannedAt *time.Time
	var collectionTTL *int
	var floatLimit *int
	var floatDirection *string
	var global []byte
	var snapshot []byte
	var expiry *time.Time
	if err := tx.QueryRow(ctx, `SELECT m.username,m.pid,m.key_cipher,m.key_version,m.banned_at,c.order_ttl_seconds,c.float_limit_cents,c.float_direction,s.payment,sub.snapshot,sub.expires_at FROM merchants m LEFT JOIN merchant_collection_settings c ON c.merchant_id=m.id CROSS JOIN site_settings s LEFT JOIN subscriptions sub ON sub.merchant_id=m.id WHERE m.id=$1 AND s.singleton=true`, merchantID).Scan(&result.MerchantUsername, &result.PID, &result.KeyCipher, &result.KeyVersion, &bannedAt, &collectionTTL, &floatLimit, &floatDirection, &global, &snapshot, &expiry); err != nil {
		return result, err
	}
	if bannedAt != nil {
		result.Banned = true
		return result, apiErr("ACCOUNT_BANNED", 403)
	}
	var settings struct {
		Min          int64  `json:"min_amount_cents"`
		Max          int64  `json:"max_amount_cents"`
		DefaultTTL   int    `json:"default_ttl_seconds"`
		MaxTTL       int    `json:"max_ttl_seconds"`
		FloatMin     int    `json:"float_min_cents"`
		FloatMax     int    `json:"float_max_cents"`
		FloatDefault int    `json:"float_default_cents"`
		Direction    string `json:"float_direction"`
	}
	if err := json.Unmarshal(global, &settings); err != nil {
		return result, err
	}
	result.MinAmountCents = settings.Min
	result.MaxAmountCents = settings.Max
	result.DefaultTTLSeconds = settings.DefaultTTL
	result.MaxTTLSeconds = settings.MaxTTL
	result.FloatMinCents = settings.FloatMin
	result.FloatMaxCents = settings.FloatMax
	result.FloatDefaultCents = settings.FloatDefault
	result.FloatDirection = settings.Direction
	result.MerchantTTL = collectionTTL
	result.MerchantFloatLimit = floatLimit
	result.MerchantFloatDirection = floatDirection
	if amount > 0 && (amount < settings.Min || amount > settings.Max) {
		return result, apiErr("AMOUNT_OUT_OF_RANGE", 400)
	}
	if expiry == nil || !expiry.After(clockNow(e.Now)) {
		return result, apiErr("SUBSCRIPTION_INACTIVE", 403)
	}
	var entitlement struct {
		Channels []string `json:"channels"`
		platform.PlanLimits
	}
	if len(snapshot) == 0 || json.Unmarshal(snapshot, &entitlement) != nil {
		if err := tx.QueryRow(ctx, `SELECT channels FROM plans WHERE id=(SELECT plan_id FROM subscriptions WHERE merchant_id=$1)`, merchantID).Scan(&entitlement.Channels); err != nil {
			return result, apiErr("SUBSCRIPTION_INACTIVE", 403)
		}
	}
	result.Channels = entitlement.Channels
	result.PlanLimits = entitlement.PlanLimits
	var online bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL AND last_seen_at>$2::timestamptz-interval '90 seconds')`, merchantID, clockNow(e.Now)).Scan(&online); err != nil {
		return result, err
	}
	result.DeviceOnline = online
	if !result.DeviceOnline {
		return result, apiErr("DEVICE_OFFLINE", 422)
	}
	rows, err := tx.Query(ctx, `SELECT channel,id FROM media_assets WHERE owner_merchant_id=$1 AND kind='qr' AND active=true ORDER BY created_at DESC`, merchantID)
	if err != nil {
		return result, err
	}
	result.QRAssets = map[string]string{}
	for rows.Next() {
		var channel, id string
		if err := rows.Scan(&channel, &id); err != nil {
			rows.Close()
			return result, err
		}
		if result.QRAssets[channel] == "" {
			result.QRAssets[channel] = id
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	return result, nil
}

func (e SQLEligibility) IPAllowed(ctx context.Context, tx pgx.Tx, merchantID int64, ip string) error {
	var enabled bool
	var entries []string
	if err := tx.QueryRow(ctx, `SELECT enabled,entries::text[] FROM merchant_ip_rules WHERE merchant_id=$1`, merchantID).Scan(&enabled, &entries); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if !enabled {
		return nil
	}
	address, parseErr := netip.ParseAddr(strings.TrimSpace(ip))
	if parseErr != nil || address.Zone() != "" {
		return apiErr("IP_NOT_ALLOWED", 403)
	}
	ip = address.Unmap().String()
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(bool_or($2::inet <<= rule),false) FROM merchant_ip_rules CROSS JOIN LATERAL unnest(entries) rule WHERE merchant_id=$1`, merchantID, ip).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return apiErr("IP_NOT_ALLOWED", 403)
	}
	return nil
}
func clockNow(f func() time.Time) time.Time {
	if f == nil {
		return time.Now()
	}
	return f()
}
func containsChannel(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
