package payment

import (
	"net/http"
	"time"
)

type paymentInfo struct {
	Version         int    `json:"version"`
	ActualAmount    string `json:"actual_amount"`
	QRImageURL      string `json:"qr_image_url,omitempty"`
	ExpiresAt       string `json:"expires_at"`
	RequiresCashier bool   `json:"requires_cashier"`
}

// appendPaymentInfo is presentation-only; it never changes the order's amounts.
func (h *Handler) appendPaymentInfo(r *http.Request, in CreateCommand, order OrderDTO, data map[string]any) {
	info := paymentInfo{
		Version:         1,
		ExpiresAt:       order.ExpiresAt.UTC().Format(time.RFC3339Nano),
		RequiresCashier: true,
	}
	if order.ActualCents != nil {
		info.ActualAmount = formatMoney(*order.ActualCents)
	}
	checkout, image := h.paymentQRImage(r, in, order, false)
	// Use the persisted deadline even when issuing an image is disallowed.
	// PostgreSQL stores microseconds; a new order DTO can still have nanoseconds.
	if !checkout.ExpiresAt.IsZero() {
		info.ExpiresAt = checkout.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if checkout.ActualCents != nil {
		info.ActualAmount = formatMoney(*checkout.ActualCents)
	}
	if image != "" {
		info.QRImageURL = image
		info.RequiresCashier = false
	} else {
		// A later eligibility failure must also suppress an earlier legacy code.
		delete(data, "qrcode")
	}
	if info.ActualAmount == "" {
		return
	}
	data["payment_info"] = info
}
