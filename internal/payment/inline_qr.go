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
	if !inline {
		h.appendStandardQR(r, in, checkout, data)
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

func (h *Handler) appendStandardQR(r *http.Request, in CreateCommand, checkout CheckoutDTO, data map[string]any) {
	var cipher []byte
	err := h.Service.pool.QueryRow(r.Context(), `SELECT a.qr_payload_cipher
		FROM orders o JOIN merchants m ON m.id=o.merchant_id
		JOIN media_assets a ON a.id=o.qr_asset_id AND a.owner_merchant_id=o.merchant_id AND a.channel=o.channel
		WHERE o.trade_no=$1 AND o.merchant_id=$2 AND o.channel=$3 AND a.kind='qr'
		AND o.purpose='external' AND NOT o.is_test AND o.status='pending' AND o.expires_at>$4
		AND o.actual_cents=o.original_cents AND m.banned_at IS NULL AND m.approval_state='approved'`,
		checkout.TradeNo, in.MerchantID, in.Channel, h.Service.clock.Now()).Scan(&cipher)
	if err != nil {
		return
	}
	if payload, err := platform.Decrypt(h.Service.key[:], cipher); err == nil && len(payload) > 0 {
		data["qrcode"] = string(payload)
	}
}
