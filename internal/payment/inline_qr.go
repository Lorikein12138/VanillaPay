package payment

import (
	"net/http"
	"net/url"
	"strings"

	"vanillapay/internal/platform"
)

// appendInlineQR adds payment instructions only for an explicit API opt-in.
// The cashier URL remains available even if the instructions cannot be issued.
func (h *Handler) appendInlineQR(r *http.Request, p url.Values, in CreateCommand, order OrderDTO, data map[string]any) {
	if p.Get("inline_qr") != "1" || in.Channel == "" {
		return
	}
	ctx := r.Context()
	risk, err := readCheckoutRisk(ctx, h.Service.pool, h.realIP(r), in.ClientIP)
	if err != nil || risk.SiteKey != "" {
		return
	}
	checkout, err := h.checkoutData(ctx, order.TradeNo, risk.SiteKey, "")
	if err != nil || !checkout.CanPay || checkout.QRImageURL == nil || checkout.Channel == nil ||
		*checkout.Channel != in.Channel || checkout.ActualCents == nil || checkout.ExpiresAt.Unix() <= checkout.ServerTime.Unix() {
		return
	}
	media, err := url.Parse(*checkout.QRImageURL)
	if err != nil {
		return
	}
	query := media.Query()
	query.Set("inline", "1")
	query.Set("signature", platform.SignInlineQR(h.Service.key[:], checkout.TradeNo, media.Path, query.Get("expires")))
	media.RawQuery = query.Encode()
	data["qrcode"] = strings.TrimRight(h.PublicBaseURL, "/") + media.String()
	data["actual_money"] = formatMoney(*checkout.ActualCents)
	data["type"] = *checkout.Channel
	data["expires_at"] = checkout.ExpiresAt
}
