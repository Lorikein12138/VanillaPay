package payment

import (
	"html"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"vanillapay/internal/platform"
)

func (h *Handler) V1Submit(w http.ResponseWriter, r *http.Request) { h.v1Create(w, r, false) }

func (h *Handler) V1MAPI(w http.ResponseWriter, r *http.Request) { h.v1Create(w, r, true) }

func (h *Handler) v1Create(w http.ResponseWriter, r *http.Request, api bool) {
	fail := func(code string) {
		if api {
			v1Error(w, code)
			return
		}
		writeSubmitError(w, code)
	}
	p, err := parseProtocolParams(r)
	if err != nil {
		fail("INVALID_SIGNATURE")
		return
	}
	merchantID, _, err := h.authenticateV1PID(r.Context(), p.Get("pid"), 0)
	if err != nil {
		fail(errorCode(err))
		return
	}
	key, err := h.currentV1Key(r.Context(), merchantID)
	if err != nil {
		fail("INVALID_SIGNATURE")
		return
	}
	signType := strings.ToUpper(p.Get("sign_type"))
	inline := api && p.Get("inline_qr") == "1"
	expectedType := "MD5"
	if inline {
		expectedType = "HMAC-SHA256"
	}
	if signType != "" && signType != expectedType {
		fail("INVALID_SIGNATURE")
		return
	}
	if inline {
		err = verifyInlineQRV1(p, key)
	} else {
		err = VerifyV1(p, string(key))
	}
	if err != nil {
		fail("INVALID_SIGNATURE")
		return
	}
	in, err := h.externalCommand(r, p, merchantID, "v1", api)
	if err != nil {
		fail(errorCode(err))
		return
	}
	order, err := h.Service.Create(r.Context(), in)
	if err != nil {
		fail(errorCode(err))
		return
	}
	payURL := h.cashierURL(order.TradeNo)
	if api {
		data := map[string]any{"code": 1, "msg": "success", "trade_no": order.TradeNo, "payurl": payURL}
		h.appendPaymentQR(r, in, order, inline, data)
		v1JSON(w, http.StatusOK, data)
		return
	}
	status := http.StatusSeeOther
	if r.Method == http.MethodGet {
		status = http.StatusFound
	}
	http.Redirect(w, r, "/pay/"+url.PathEscape(order.TradeNo), status)
}

var submitRiskMessages = map[string]string{
	"DAILY_LIMIT_EXCEEDED":    "商户已达到当日收款限额。 / The merchant has reached the daily payment limit.",
	"IP_DAILY_LIMIT_EXCEEDED": "此 IP 地址已达到当日下单上限。 / This IP address has reached the daily order limit.",
	"IP_BLOCKED":              "此 IP 地址无法发起支付。 / Payments are unavailable from this IP address.",
	"NAME_BLOCKED":            "商品名称不符合支付规则。 / The product name is restricted.",
	"URL_DOMAIN_NOT_ALLOWED":  "商户未允许订单使用的回调或跳转域名。 / The merchant has not allowed the callback or return domain.",
}

func writeSubmitError(w http.ResponseWriter, code string) {
	var detail string
	if message := submitRiskMessages[code]; message != "" {
		detail = "<p>" + html.EscapeString(message) + "</p>"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<!doctype html>
<html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>无法发起支付 · Payment unavailable</title>
<style>body{margin:0;background:#faf7f0;color:#28231e;font:1rem/1.6 system-ui,sans-serif;padding:clamp(1rem,8vw,5rem)}
main{max-width:34rem;margin:auto;background:#fffdf8;border:1px solid #e7ded0;border-radius:1rem;padding:clamp(1rem,5vw,2.5rem)}
h1{font:2rem Georgia,serif;margin:0;color:#87452e}code{overflow-wrap:anywhere}</style>
<main><h1>无法发起支付</h1><p>请返回商户网站重试，或联系商户并提供下方错误码。</p>
<p lang="en">Payment could not start. Return to the merchant and try again, or share the error code with them.</p>
`+detail+`<code>`+html.EscapeString(code)+`</code></main></html>`)
}

func (h *Handler) externalCommand(r *http.Request, p url.Values, merchantID int64, protocol string, api bool) (CreateCommand, error) {
	name := p.Get("name")
	if name == "" || len([]byte(name)) > 4096 {
		return CreateCommand{}, apiErr("VALIDATION_ERROR", 400)
	}
	cents, err := ParseCents(p.Get("money"))
	if err != nil {
		return CreateCommand{}, apiErr("VALIDATION_ERROR", 400)
	}
	if len(p.Get("notify_url")) > 2048 || len(p.Get("return_url")) > 2048 {
		return CreateCommand{}, apiErr("INVALID_URL", 400)
	}
	channel := p.Get("type")
	if channel != "" && !platform.ValidPaymentChannel(channel) {
		return CreateCommand{}, apiErr("VALIDATION_ERROR", 400)
	}
	var ttl *int
	if raw := p.Get("expire_seconds"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 {
			return CreateCommand{}, apiErr("TTL_OUT_OF_RANGE", 400)
		}
		ttl = &n
	}
	clientIP := p.Get("clientip")
	if api && clientIP == "" {
		return CreateCommand{}, apiErr("VALIDATION_ERROR", 400)
	}
	if clientIP != "" {
		if _, e := netip.ParseAddr(clientIP); e != nil {
			return CreateCommand{}, apiErr("VALIDATION_ERROR", 400)
		}
	}
	source := h.realIP(r)
	if _, _, err := h.authenticateV1PID(r.Context(), p.Get("pid"), merchantID); err != nil {
		return CreateCommand{}, err
	}
	if !api && p.Get("return_url") == "" {
		return CreateCommand{}, apiErr("INVALID_URL", 400)
	}
	out := p.Get("out_trade_no")
	return CreateCommand{MerchantID: merchantID, Protocol: protocol, Purpose: "external", OutTradeNo: out, Name: name, ProductID: p.Get("product_id"), Param: p.Get("param"), OriginalCents: cents, money: p.Get("money"), NotifyURL: p.Get("notify_url"), ReturnURL: p.Get("return_url"), ClientIP: clientIP, SourceIP: source, Channel: channel, ExpireSeconds: ttl, Actor: h.requestActor(r, merchantID)}, nil
}

func (h *Handler) V1API(w http.ResponseWriter, r *http.Request) {
	p, err := parseV1ActionParams(r)
	if err != nil {
		v1Error(w, "INVALID_SIGNATURE")
		return
	}
	id, key, err := h.authenticateV1(r.Context(), p)
	if err != nil {
		v1Error(w, errorCode(err))
		return
	}
	if p.Get("sign") != "" && VerifyV1(p, string(key)) != nil {
		v1Error(w, "INVALID_SIGNATURE")
		return
	}
	if p.Get("sign_type") != "" && strings.ToUpper(p.Get("sign_type")) != "MD5" {
		v1Error(w, "INVALID_SIGNATURE")
		return
	}
	switch p.Get("act") {
	case "order":
		h.queryProtocol(w, r, id, p, false)
	case "refund":
		h.refundProtocol(w, r, id, p, false)
	case "query":
		h.merchantInfoProtocol(w, r, id, p, false)
	case "orders":
		h.merchantOrdersProtocol(w, r, id, p, false)
	default:
		v1JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": "不支持的操作", "error_code": "UNSUPPORTED_ACTION"})
	}
}
