package payment

import (
	"net/http"
	"net/url"
	"time"

	"vanillapay/internal/platform"
)

// appendPaymentQR adds payment instructions for an eligible API order.
// The cashier URL remains available even if the instructions cannot be issued.
func (h *Handler) appendPaymentQR(r *http.Request, in CreateCommand, order OrderDTO, inline bool, data map[string]any) {
	if in.Channel == "" || (!inline && (order.ActualCents == nil || *order.ActualCents != order.OriginalCents)) {
		return
	}
	if !inline {
		h.appendStandardQR(r, in, order.TradeNo, data)
		return
	}
	ctx := r.Context()
	var storedIP string
	if err := h.Service.pool.QueryRow(ctx, `SELECT COALESCE(host(clientip),'')
		FROM orders WHERE trade_no=$1 AND merchant_id=$2`, order.TradeNo, in.MerchantID).Scan(&storedIP); err != nil {
		return
	}
	risk, err := readCheckoutRisk(ctx, h.Service.pool, h.realIP(r), in.ClientIP, storedIP)
	if err != nil || risk.SiteKey != "" {
		return
	}
	checkout, err := h.checkoutData(ctx, order.TradeNo, risk.SiteKey, "")
	if err != nil || !checkout.CanPay || checkout.QRImageURL == nil || checkout.Channel == nil ||
		*checkout.Channel != in.Channel || checkout.ActualCents == nil || !checkout.ExpiresAt.After(checkout.ServerTime) {
		return
	}
	media, err := url.Parse(*checkout.QRImageURL)
	if err != nil {
		return
	}
	query := media.Query()
	query.Set("inline", "1")
	expiry := checkout.ExpiresAt.UTC().Format(time.RFC3339Nano)
	query.Set("expires", expiry)
	query.Set("signature", platform.SignInlineQR(h.Service.key[:], checkout.TradeNo, media.Path, expiry))
	media.RawQuery = query.Encode()
	data["qrcode"] = h.absolutePaymentURL(media)
	data["actual_money"] = formatMoney(*checkout.ActualCents)
	data["type"] = *checkout.Channel
	data["expires_at"] = checkout.ExpiresAt
}

func (h *Handler) appendStandardQR(r *http.Request, in CreateCommand, trade string, data map[string]any) {
	var cipher []byte
	var buyerIP string
	var blacklist []string
	err := h.Service.pool.QueryRow(r.Context(), `SELECT a.qr_payload_cipher,COALESCE(host(o.clientip),''),
		ARRAY(SELECT jsonb_array_elements_text(s.risk->'ip_blacklist'))
		FROM orders o JOIN merchants m ON m.id=o.merchant_id
		JOIN media_assets a ON a.id=o.qr_asset_id AND a.owner_merchant_id=o.merchant_id AND a.channel=o.channel
		CROSS JOIN site_settings s
		WHERE s.singleton AND o.trade_no=$1 AND o.merchant_id=$2 AND o.channel=$3 AND a.kind='qr'
		AND o.purpose='external' AND NOT o.is_test AND o.status='pending' AND o.expires_at>$4
		AND o.actual_cents=o.original_cents AND m.banned_at IS NULL AND m.approval_state='approved'
		AND EXISTS(SELECT 1 FROM devices d WHERE d.merchant_id=m.id AND d.revoked_at IS NULL
			AND d.last_seen_at<=$4 AND d.last_seen_at>$4::timestamptz-interval '90 seconds')
		AND NOT (COALESCE((s.risk->>'checkout_turnstile_enabled')::boolean,false)
			AND COALESCE(btrim(s.security->>'turnstile_site_key'),'') <> ''
			AND EXISTS(SELECT 1 FROM secret_settings WHERE name='turnstile_secret' AND octet_length(encrypted_value)>0))`,
		trade, in.MerchantID, in.Channel, h.Service.clock.Now()).Scan(&cipher, &buyerIP, &blacklist)
	if err != nil {
		return
	}
	for _, ip := range []string{h.realIP(r), in.ClientIP, buyerIP} {
		if enforceCheckoutIP(ip, blacklist) != nil {
			return
		}
	}
	if payload, err := platform.Decrypt(h.Service.key[:], cipher); err == nil && len(payload) > 0 {
		data["qrcode"] = string(payload)
	}
}
