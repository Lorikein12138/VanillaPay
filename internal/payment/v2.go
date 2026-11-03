package payment

import (
	"net/http"
	"net/url"
	"strings"
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
	if err := VerifyV2(p, publicKey, h.Service.clock.Now()); err != nil {
		code := "INVALID_SIGNATURE"
		if err.Error() == "TIMESTAMP_EXPIRED" {
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
		h.v2JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": errorCode(err), "error_code": errorCode(err)})
		return
	}
	cashier := strings.TrimRight(h.PublicBaseURL, "/") + "/pay/" + url.PathEscape(order.TradeNo)
	if !api {
		status := http.StatusSeeOther
		if r.Method == http.MethodGet {
			status = http.StatusFound
		}
		http.Redirect(w, r, "/pay/"+url.PathEscape(order.TradeNo), status)
		return
	}
	h.v2JSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "trade_no": order.TradeNo, "pay_type": "jump", "pay_info": cashier})
}

func (h *Handler) V2Query(w http.ResponseWriter, r *http.Request) {
	p, err := parseProtocolParams(r)
	if err != nil {
		h.v2JSON(w, 200, map[string]any{"code": -1, "msg": "INVALID_SIGNATURE", "error_code": "INVALID_SIGNATURE"})
		return
	}
	id, pub, err := h.authenticateV2(r.Context(), p)
	if err != nil {
		h.v2JSON(w, 200, map[string]any{"code": -1, "msg": errorCode(err), "error_code": errorCode(err)})
		return
	}
	if err := VerifyV2(p, pub, h.Service.clock.Now()); err != nil {
		h.v2JSON(w, 200, map[string]any{"code": -1, "msg": errorCode(err), "error_code": errorCode(err)})
		return
	}
	h.queryProtocol(w, r, id, p, true)
}

func (h *Handler) V2Refund(w http.ResponseWriter, r *http.Request) {
	p, err := parseProtocolParams(r)
	if err != nil {
		h.v2JSON(w, 200, map[string]any{"code": -1, "msg": "INVALID_SIGNATURE", "error_code": "INVALID_SIGNATURE"})
		return
	}
	id, pub, err := h.authenticateV2(r.Context(), p)
	if err != nil {
		h.v2JSON(w, 200, map[string]any{"code": -1, "msg": errorCode(err), "error_code": errorCode(err)})
		return
	}
	if err := VerifyV2(p, pub, h.Service.clock.Now()); err != nil {
		h.v2JSON(w, 200, map[string]any{"code": -1, "msg": errorCode(err), "error_code": errorCode(err)})
		return
	}
	h.refundProtocol(w, r, id, p, true)
}
