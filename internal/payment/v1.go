package payment

import (
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

func (h *Handler) V1Submit(w http.ResponseWriter, r *http.Request) { h.v1Create(w, r, false) }

func (h *Handler) V1MAPI(w http.ResponseWriter, r *http.Request) { h.v1Create(w, r, true) }

func (h *Handler) v1Create(w http.ResponseWriter, r *http.Request, api bool) {
	p, err := parseProtocolParams(r)
	if err != nil {
		v1Error(w, "INVALID_SIGNATURE")
		return
	}
	merchantID, _, err := h.authenticateV1PID(r.Context(), p.Get("pid"), 0)
	if err != nil {
		v1Error(w, errorCode(err))
		return
	}
	key, err := h.currentV1Key(r.Context(), merchantID)
	if err != nil {
		v1Error(w, "INVALID_SIGNATURE")
		return
	}
	if p.Get("sign_type") != "" && strings.ToUpper(p.Get("sign_type")) != "MD5" {
		v1Error(w, "INVALID_SIGNATURE")
		return
	}
	if err := VerifyV1(p, string(key)); err != nil {
		v1Error(w, "INVALID_SIGNATURE")
		return
	}
	in, err := h.externalCommand(r, p, merchantID, "v1", api)
	if err != nil {
		v1Error(w, errorCode(err))
		return
	}
	order, err := h.Service.Create(r.Context(), in)
	if err != nil {
		v1Error(w, errorCode(err))
		return
	}
	payURL := strings.TrimRight(h.PublicBaseURL, "/") + "/pay/" + url.PathEscape(order.TradeNo)
	if api {
		v1JSON(w, http.StatusOK, map[string]any{"code": 1, "msg": "success", "trade_no": order.TradeNo, "payurl": payURL})
		return
	}
	status := http.StatusSeeOther
	if r.Method == http.MethodGet {
		status = http.StatusFound
	}
	http.Redirect(w, r, "/pay/"+url.PathEscape(order.TradeNo), status)
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
		return CreateCommand{}, apiErr("VALIDATION_ERROR", 400)
	}
	channel := p.Get("type")
	if channel != "" && !validChannel(channel) {
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
	if p.Get("device") != "" && !containsValue([]string{"pc", "mobile", "qq", "wechat", "alipay", "douyin", "jump"}, p.Get("device")) {
		return CreateCommand{}, apiErr("VALIDATION_ERROR", 400)
	}
	out := p.Get("out_trade_no")
	return CreateCommand{MerchantID: merchantID, Protocol: protocol, Purpose: "external", OutTradeNo: out, Name: name, ProductID: p.Get("product_id"), Param: p.Get("param"), OriginalCents: cents, NotifyURL: p.Get("notify_url"), ReturnURL: p.Get("return_url"), ClientIP: clientIP, SourceIP: source, Channel: channel, ExpireSeconds: ttl, Actor: h.requestActor(r, merchantID)}, nil
}

func (h *Handler) V1API(w http.ResponseWriter, r *http.Request) {
	p, err := parseProtocolParams(r)
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
	default:
		v1JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": "不支持的操作", "error_code": "UNSUPPORTED_ACTION"})
	}
}
