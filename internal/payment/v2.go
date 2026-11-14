package payment

import (
	"net/http"
	"net/url"
)

func (h *Handler) V2Submit(w http.ResponseWriter, r *http.Request) { h.v2Create(w, r, false) }

func (h *Handler) V2Create(w http.ResponseWriter, r *http.Request) { h.v2Create(w, r, true) }

func (h *Handler) v2Create(w http.ResponseWriter, r *http.Request, api bool) {
	p, err := parseProtocolParams(r)
	if err != nil {
		h.v2JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": "签名错误", "error_code": "INVALID_SIGNATURE"})
		return
	}
	merchantID, publicKey, err := h.authenticateV2(r.Context(), p)
	if err != nil {
		h.v2JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": "签名错误", "error_code": errorCode(err)})
		return
	}
	canonicalize := Canonical
	if api && p.Get("inline_qr") == "1" {
		canonicalize = inlineQRCanonical
	}
	if err := verifyV2(p, publicKey, h.Service.clock.Now(), canonicalize); err != nil {
		code := "INVALID_SIGNATURE"
		if errorCode(err) == "TIMESTAMP_EXPIRED" {
			code = "TIMESTAMP_EXPIRED"
		}
		h.v2JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": code, "error_code": code})
		return
	}
	if err := validateV2Capabilities(p); err != nil {
		h.v2JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": errorCode(err), "error_code": errorCode(err)})
		return
	}
	in, err := h.externalCommand(r, p, merchantID, "v2", api)
	if err != nil {
		h.v2JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": errorCode(err), "error_code": errorCode(err)})
		return
	}
	order, err := h.Service.Create(r.Context(), in)
	if err != nil {
		code := errorCode(err)
		if !api && submitRiskMessages[code] != "" {
			writeSubmitError(w, code)
			return
		}
		h.v2JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": code, "error_code": code})
		return
	}
	cashier := h.cashierURL(order.TradeNo)
	if !api {
		status := http.StatusSeeOther
		if r.Method == http.MethodGet {
			status = http.StatusFound
		}
		http.Redirect(w, r, "/pay/"+url.PathEscape(order.TradeNo), status)
		return
	}
	data := map[string]any{"code": 0, "msg": "success", "trade_no": order.TradeNo, "pay_type": "jump", "pay_info": cashier, "payurl": cashier}
	h.appendInlineQR(r, p, in, order, data)
	h.v2JSON(w, http.StatusOK, data)
}

func (h *Handler) v2Action(requireRSA bool, action func(http.ResponseWriter, *http.Request, int64, url.Values)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := parseProtocolParams(r)
		if err != nil || requireRSA && p.Get("sign_type") != "RSA" {
			h.protocolError(w, true, "INVALID_SIGNATURE")
			return
		}
		id, pub, err := h.authenticateV2(r.Context(), p)
		if err == nil {
			err = VerifyV2(p, pub, h.Service.clock.Now())
		}
		if err != nil {
			h.protocolError(w, true, errorCode(err))
			return
		}
		action(w, r, id, p)
	}
}
