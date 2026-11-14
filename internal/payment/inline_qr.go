package payment

import (
	"net/http"
	"net/url"
	"time"

	"vanillapay/internal/platform"
)

// appendInlineQR adds payment instructions only for an explicit API opt-in.
// The cashier URL remains available even if the instructions cannot be issued.
func (h *Handler) appendInlineQR(r *http.Request, p url.Values, in CreateCommand, order OrderDTO, data map[string]any) {
	if p.Get("inline_qr") != "1" || in.Channel == "" {
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
