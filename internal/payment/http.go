package payment

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
)

type Handler struct {
	Service           *Service
	PublicBaseURL     string
	TrustedProxyCIDRs []netip.Prefix
}

func (h *Handler) ProtocolRoutes() chi.Router {
	r := chi.NewRouter()
	h.RegisterProtocolRoutes(r)
	return r
}

func (h *Handler) APIRoutes() chi.Router {
	r := chi.NewRouter()
	h.RegisterAPIRoutes(r)
	return r
}

func (h *Handler) RegisterProtocolRoutes(r chi.Router) {
	r.Get("/submit.php", h.V1Submit)
	r.Post("/submit.php", h.V1Submit)
	r.Post("/mapi.php", h.V1MAPI)
	r.Get("/api.php", h.V1API)
	r.Post("/api.php", h.V1API)
	r.Get("/api/pay/submit", h.V2Submit)
	r.Post("/api/pay/submit", h.V2Submit)
	r.Post("/api/pay/create", h.V2Create)
	r.Post("/api/pay/query", h.V2Query)
	r.Post("/api/pay/refund", h.V2Refund)
}

func (h *Handler) RegisterAPIRoutes(r chi.Router) {
	r.Get("/checkout/{tradeNo}", h.Checkout)
	r.Post("/checkout/{tradeNo}/channel", h.SelectCheckoutChannel)
	r.Get("/merchant/orders", h.ListOrders)
	r.Get("/merchant/orders/export.csv", h.ExportOrders)
	r.Get("/merchant/orders/{trade_no}", h.OrderDetail)
	r.Post("/merchant/orders/{trade_no}/supplement", h.Supplement)
	r.Post("/merchant/orders/{trade_no}/refund", h.MarkRefunded)
	r.Post("/merchant/refunds/{id}/resolve", h.ResolveRefund)
	r.Post("/merchant/orders/{trade_no}/callback-replay", h.CallbackReplay)
	r.Get("/console/orders", h.ListOrders)
	r.Get("/console/orders/export.csv", h.ExportOrders)
	r.Get("/console/orders/{trade_no}", h.OrderDetail)
}

func (h *Handler) queryProtocol(w http.ResponseWriter, r *http.Request, merchantID int64, p url.Values, v2 bool) {
	if err := h.Service.CheckProtocolIP(r.Context(), merchantID, h.realIP(r)); err != nil {
		h.protocolError(w, v2, errorCode(err))
		return
	}
	trade, out := p.Get("trade_no"), p.Get("out_trade_no")
	if trade == "" && out == "" {
		h.protocolError(w, v2, "VALIDATION_ERROR")
		return
	}
	dto, err := h.Service.Query(r.Context(), h.requestActor(r, merchantID), OrderSelector{TradeNo: trade, OutTradeNo: out, MerchantID: merchantID})
	if err != nil {
		h.protocolError(w, v2, errorCode(err))
		return
	}
	data := h.protocolOrder(dto)
	var pid, buyer, clientIP string
	var refundStatus, refundNo pgtype.Text
	if err := h.Service.pool.QueryRow(r.Context(), `SELECT o.merchant_pid_snapshot,COALESCE(o.buyer_hint,''),COALESCE(host(o.clientip),''),(SELECT status FROM refunds WHERE order_id=o.id ORDER BY requested_at DESC LIMIT 1),(SELECT public_id FROM refunds WHERE order_id=o.id ORDER BY requested_at DESC LIMIT 1) FROM orders o WHERE o.trade_no=$1`, dto.TradeNo).Scan(&pid, &buyer, &clientIP, &refundStatus, &refundNo); err != nil {
		h.protocolError(w, v2, "INTERNAL_ERROR")
		return
	}
	data["pid"] = pid
	data["buyer"] = buyer
	data["clientip"] = clientIP
	if refundStatus.Valid {
		data["refund_status"] = refundStatus.String
	}
	if refundNo.Valid {
		data["refund_no"] = refundNo.String
	}
	if dto.Status == "refunded" {
		data["refundmoney"] = formatMoney(dto.OriginalCents)
	}
	if v2 {
		data["code"] = 0
		provider := ""
		if dto.Status == "paid" {
			_ = h.Service.pool.QueryRow(r.Context(), `SELECT COALESCE(r.provider_trade_no,'') FROM receipts r JOIN orders o ON o.id=r.order_id WHERE o.trade_no=$1 LIMIT 1`, dto.TradeNo).Scan(&provider)
		}
		data["api_trade_no"] = provider
		h.v2JSON(w, 200, data)
	} else {
		v1JSON(w, 200, data)
	}
}

func (h *Handler) refundProtocol(w http.ResponseWriter, r *http.Request, merchantID int64, p url.Values, v2 bool) {
	if err := h.Service.CheckProtocolIP(r.Context(), merchantID, h.realIP(r)); err != nil {
		h.protocolError(w, v2, errorCode(err))
		return
	}
	trade, out := p.Get("trade_no"), p.Get("out_trade_no")
	if trade == "" && out == "" {
		h.protocolError(w, v2, "VALIDATION_ERROR")
		return
	}
	dto, err := h.Service.Query(r.Context(), h.requestActor(r, merchantID), OrderSelector{TradeNo: trade, OutTradeNo: out, MerchantID: merchantID})
	if err != nil {
		h.protocolError(w, v2, errorCode(err))
		return
	}
	cents, err := ParseCents(p.Get("money"))
	if err != nil {
		h.protocolError(w, v2, "VALIDATION_ERROR")
		return
	}
	digest := sha256Bytes([]byte(dto.TradeNo + "\n" + p.Get("out_refund_no") + "\n" + strconv.FormatInt(cents, 10) + "\n" + p.Get("reason")))
	refund, err := h.Service.RequestRefund(r.Context(), RefundCommand{MerchantID: merchantID, TradeNo: dto.TradeNo, OutRefundNo: p.Get("out_refund_no"), AmountCents: cents, Reason: p.Get("reason"), RequestDigest: digest, Actor: h.requestActor(r, merchantID)})
	if err != nil {
		h.protocolError(w, v2, errorCode(err))
		return
	}
	data := map[string]any{"code": 1, "msg": "已提交", "refund_no": refund.ID, "status": "pending"}
	if v2 {
		data = map[string]any{"code": 0, "msg": "已提交", "refund_no": refund.ID, "out_refund_no": p.Get("out_refund_no"), "status": "pending"}
		h.v2JSON(w, 200, data)
	} else {
		v1JSON(w, 200, data)
	}
}

func (h *Handler) authenticateV1(ctx context.Context, p url.Values) (int64, []byte, error) {
	id, _, err := h.authenticateV1PID(ctx, p.Get("pid"), 0)
	if err != nil {
		return 0, nil, err
	}
	secret, err := h.currentV1Key(ctx, id)
	if err != nil {
		return 0, nil, err
	}
	provided := p.Get("key")
	if len(provided) != len(secret) || subtle.ConstantTimeCompare([]byte(provided), secret) != 1 {
		return 0, nil, apiErr("INVALID_SIGNATURE", 400)
	}
	return id, secret, nil
}
func (h *Handler) currentV1Key(ctx context.Context, id int64) ([]byte, error) {
	var cipher []byte
	if err := h.Service.pool.QueryRow(ctx, `SELECT key_cipher FROM merchants WHERE id=$1`, id).Scan(&cipher); err != nil {
		return nil, apiErr("INVALID_SIGNATURE", 400)
	}
	secret, err := platform.Decrypt(h.Service.key[:], cipher)
	if err != nil {
		return nil, err
	}
	return secret, nil
}
func (h *Handler) authenticateV1PID(ctx context.Context, pid string, expected int64) (int64, []byte, error) {
	if len(pid) != 16 || pid[0] == '0' {
		return 0, nil, apiErr("INVALID_SIGNATURE", 400)
	}
	for _, r := range pid {
		if r < '0' || r > '9' {
			return 0, nil, apiErr("INVALID_SIGNATURE", 400)
		}
	}
	var id int64
	if err := h.Service.pool.QueryRow(ctx, `SELECT id FROM merchants WHERE pid=$1`, pid).Scan(&id); err != nil || expected > 0 && expected != id {
		return 0, nil, apiErr("INVALID_SIGNATURE", 400)
	}
	return id, nil, nil
}
func (h *Handler) authenticateV2(ctx context.Context, p url.Values) (int64, *rsa.PublicKey, error) {
	id, _, err := h.authenticateV1PID(ctx, p.Get("pid"), 0)
	if err != nil {
		return 0, nil, err
	}
	var pemText pgtype.Text
	if err := h.Service.pool.QueryRow(ctx, `SELECT rsa_public_key_pem FROM merchants WHERE id=$1 AND banned_at IS NULL`, id).Scan(&pemText); err != nil || !pemText.Valid {
		return 0, nil, apiErr("INVALID_SIGNATURE", 400)
	}
	block, _ := pem.Decode([]byte(pemText.String))
	if block == nil {
		return 0, nil, apiErr("INVALID_SIGNATURE", 400)
	}
	var public *rsa.PublicKey
	switch block.Type {
	case "PUBLIC KEY":
		parsed, e := x509.ParsePKIXPublicKey(block.Bytes)
		if e == nil {
			public, _ = parsed.(*rsa.PublicKey)
		}
	case "RSA PUBLIC KEY":
		public, _ = x509.ParsePKCS1PublicKey(block.Bytes)
	}
	if public == nil {
		return 0, nil, apiErr("INVALID_SIGNATURE", 400)
	}
	return id, public, nil
}

func (h *Handler) protocolError(w http.ResponseWriter, v2 bool, code string) {
	message := code
	if v2 {
		h.v2JSON(w, 200, map[string]any{"code": -1, "msg": message, "error_code": code})
	} else {
		v1Error(w, code)
	}
}
func (h *Handler) v2JSON(w http.ResponseWriter, status int, data map[string]any) {
	private, keyID, err := h.Service.platformSigningKey(context.Background())
	if err != nil {
		writeProtocolJSON(w, 500, map[string]any{"code": -1, "msg": "INTERNAL_ERROR", "error_code": "INTERNAL_ERROR"})
		return
	}
	params := url.Values{}
	for k, v := range data {
		switch x := v.(type) {
		case string:
			params.Set(k, x)
		case time.Time:
			params.Set(k, x.Format(time.RFC3339Nano))
		case *time.Time:
			if x == nil {
				params.Set(k, "null")
			} else {
				params.Set(k, x.Format(time.RFC3339Nano))
			}
		case *string:
			if x == nil {
				params.Set(k, "null")
			} else {
				params.Set(k, *x)
			}
		case int:
			params.Set(k, strconv.Itoa(x))
		case int64:
			params.Set(k, strconv.FormatInt(x, 10))
		case bool:
			params.Set(k, strconv.FormatBool(x))
		case nil:
			params.Set(k, "")
		default:
			b, _ := json.Marshal(x)
			params.Set(k, string(b))
		}
	}
	params.Set("timestamp", strconv.FormatInt(h.Service.clock.Now().Unix(), 10))
	params.Set("sign_type", "RSA")
	params.Set("key_id", keyID)
	sig, err := SignV2(params, private)
	if err != nil {
		writeProtocolJSON(w, 500, map[string]any{"code": -1, "msg": "INTERNAL_ERROR", "error_code": "INTERNAL_ERROR"})
		return
	}
	out := map[string]any{}
	for k := range data {
		out[k] = data[k]
	}
	out["timestamp"] = params.Get("timestamp")
	out["sign_type"] = "RSA"
	out["key_id"] = keyID
	out["sign"] = sig
	writeProtocolJSON(w, status, out)
}

func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	trade := chi.URLParam(r, "tradeNo")
	dto, err := h.checkoutData(r.Context(), trade)
	httpJSON(w, r, dto, err, 200)
}
func (h *Handler) SelectCheckoutChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
	}
	if decodePaymentJSON(w, r, &req) != nil {
		return
	}
	if !h.sameOrigin(r) {
		httpJSON(w, r, nil, apiErr("CSRF_FAILED", 403), 200)
		return
	}
	dto, err := h.Service.SelectChannel(r.Context(), chi.URLParam(r, "tradeNo"), req.Channel)
	httpJSON(w, r, dto, err, 200)
}

func (h *Handler) sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if remote := remoteAddrIP(r.RemoteAddr); remote.IsValid() && isTrustedProxy(remote, h.TrustedProxyCIDRs) {
		forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
		if forwarded == "http" || forwarded == "https" {
			scheme = forwarded
		} else if forwarded != "" {
			return false
		}
	}
	return strings.EqualFold(u.Scheme, scheme) && strings.EqualFold(u.Host, r.Host)
}
func (h *Handler) checkoutData(ctx context.Context, trade string) (CheckoutDTO, error) {
	var dto CheckoutDTO
	var merchantID int64
	var merchantName string
	var channel, asset, returnURL pgtype.Text
	var actual pgtype.Int8
	var allowed []string
	var banned bool
	var lastSeen pgtype.Timestamptz
	err := h.Service.pool.QueryRow(ctx, `SELECT o.trade_no,o.name,o.product_id,m.id,m.username,o.status,o.original_cents,o.actual_cents,o.channel,o.allowed_channels_snapshot,o.qr_asset_id,o.expires_at,o.return_url,m.banned_at IS NOT NULL,d.last_seen_at FROM orders o JOIN merchants m ON m.id=o.merchant_id LEFT JOIN devices d ON d.merchant_id=m.id AND d.revoked_at IS NULL WHERE o.trade_no=$1`, trade).Scan(&dto.TradeNo, &dto.Name, &dto.ProductID, &merchantID, &merchantName, &dto.Status, &dto.OriginalCents, &actual, &channel, &allowed, &asset, &dto.ExpiresAt, &returnURL, &banned, &lastSeen)
	if err != nil {
		return CheckoutDTO{}, apiErr("NOT_FOUND", 404)
	}
	dto.Merchant = map[string]string{"id": strconv.FormatInt(merchantID, 10), "username": merchantName}
	dto.ServerTime = h.Service.clock.Now()
	dto.RequiresExactAmount = true
	dto.AvailableChannels = []string{}
	if actual.Valid {
		value := actual.Int64
		dto.ActualCents = &value
	}
	if channel.Valid {
		value := channel.String
		dto.Channel = &value
		dto.AvailableChannels = append(dto.AvailableChannels, value)
	} else {
		dto.AvailableChannels = append(dto.AvailableChannels, allowed...)
	}
	dto.DeviceOnline = lastSeen.Valid && !lastSeen.Time.After(dto.ServerTime) && dto.ServerTime.Sub(lastSeen.Time) < 90*time.Second
	var reason string
	switch {
	case banned:
		reason = "merchant_banned"
	case dto.Status == "pending" && !dto.ExpiresAt.After(dto.ServerTime):
		reason = "expired"
		_ = h.Service.expireOne(ctx, trade)
		dto.Status = "expired"
	case dto.Status == "expired":
		reason = "expired"
	case dto.Status == "pending" && !dto.DeviceOnline:
		reason = "device_offline"
	case dto.Status == "pending" && !channel.Valid:
		reason = "no_channel"
	case dto.Status == "pending" && channel.Valid && !asset.Valid:
		reason = "qr_disabled"
	}
	if reason != "" {
		dto.Reason = &reason
	}
	dto.CanPay = dto.Status == "pending" && reason == "" && channel.Valid && actual.Valid
	if dto.CanPay {
		expiresText := strconv.FormatInt(dto.ExpiresAt.Unix(), 10)
		sig := signCheckoutMedia(h.Service.key[:], trade, asset.String, dto.ExpiresAt)
		dto.QRImageURL = stringPtr("/media/" + url.PathEscape(asset.String) + "?trade_no=" + url.QueryEscape(trade) + "&expires=" + expiresText + "&signature=" + url.QueryEscape(sig))
	}
	if dto.Status == "paid" && returnURL.Valid {
		if signed, signErr := h.signedReturnURL(ctx, trade, returnURL.String); signErr == nil {
			dto.ReturnURLSigned = &signed
		}
	}
	return dto, nil
}

func (h *Handler) signedReturnURL(ctx context.Context, trade, target string) (string, error) {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("invalid return URL")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", err
	}
	reserved := []string{"pid", "trade_no", "out_trade_no", "type", "name", "money", "trade_status", "param", "sign_type", "sign", "api_trade_no", "buyer", "addtime", "endtime", "timestamp", "key_id"}
	for _, key := range reserved {
		if _, exists := query[key]; exists {
			return "", errors.New("return URL contains a reserved payment parameter")
		}
	}
	var pid, outTrade, channel, name, param, buyer, clientIP, protocol string
	var amount int64
	var created, paid time.Time
	var cipher []byte
	var provider string
	err = h.Service.pool.QueryRow(ctx, `SELECT merchant_pid_snapshot,out_trade_no,COALESCE(channel,''),name,COALESCE(param,''),COALESCE(buyer_hint,''),COALESCE(host(clientip),''),protocol,original_cents,created_at,paid_at,signing_key_cipher,COALESCE((SELECT r.provider_trade_no FROM receipts r WHERE r.order_id=o.id LIMIT 1),'') FROM orders o WHERE trade_no=$1 AND status='paid'`, trade).Scan(&pid, &outTrade, &channel, &name, &param, &buyer, &clientIP, &protocol, &amount, &created, &paid, &cipher, &provider)
	if err != nil {
		return "", err
	}
	params := url.Values{"pid": {pid}, "trade_no": {trade}, "out_trade_no": {outTrade}, "type": {channel}, "name": {truncateUTF8(name, 127)}, "money": {formatMoney(amount)}, "trade_status": {"TRADE_SUCCESS"}, "sign_type": {"MD5"}}
	for key, values := range query {
		params[key] = append([]string(nil), values...)
	}
	params.Set("pid", pid)
	params.Set("trade_no", trade)
	params.Set("out_trade_no", outTrade)
	params.Set("type", channel)
	params.Set("name", truncateUTF8(name, 127))
	params.Set("money", formatMoney(amount))
	params.Set("trade_status", "TRADE_SUCCESS")
	params.Set("sign_type", "MD5")
	if param != "" {
		params.Set("param", param)
	}
	loc := time.FixedZone("CST", 8*3600)
	params.Set("addtime", created.In(loc).Format("2006-01-02 15:04:05"))
	params.Set("endtime", paid.In(loc).Format("2006-01-02 15:04:05"))
	switch protocol {
	case "v1":
		key, decErr := platform.Decrypt(h.Service.key[:], cipher)
		if decErr != nil {
			return "", decErr
		}
		signature, signErr := SignV1(params, string(key))
		if signErr != nil {
			return "", signErr
		}
		params.Set("sign", signature)
	case "v2":
		params.Set("api_trade_no", provider)
		params.Set("buyer", buyer)
		params.Set("timestamp", strconv.FormatInt(h.Service.clock.Now().Unix(), 10))
		params.Set("sign_type", "RSA")
		private, keyID, keyErr := h.Service.platformSigningKey(ctx)
		if keyErr != nil {
			return "", keyErr
		}
		params.Set("key_id", keyID)
		signature, signErr := SignV2(params, private)
		if signErr != nil {
			return "", signErr
		}
		params.Set("sign", signature)
	default:
		return "", errors.New("unsupported payment protocol")
	}
	for key, values := range params {
		query[key] = values
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func stringPtr(value string) *string { return &value }
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	q, err := parseOrderFilter(r)
	if err != nil {
		httpJSON(w, r, nil, err, 200)
		return
	}
	page, err := portal.NewService(h.Service.pool, h.Service.key, h.Service.clock, h.Service.random).ListOrders(r.Context(), actor, q)
	httpJSON(w, r, page, err, 200)
}

func (h *Handler) OrderDetail(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := portal.NewService(h.Service.pool, h.Service.key, h.Service.clock, h.Service.random).OrderDetail(r.Context(), actor, chi.URLParam(r, "trade_no"))
	httpJSON(w, r, dto, err, 200)
}
func (h *Handler) ExportOrders(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	allowedOrigin := h.sameOrigin(r)
	if len(r.Header.Values("Origin")) == 0 {
		allowedOrigin = r.Header.Get("Sec-Fetch-Site") == "same-origin"
	}
	if !allowedOrigin {
		httpJSON(w, r, nil, apiErr("CSRF_FAILED", 403), 200)
		return
	}
	q, err := parseOrderFilter(r)
	if err != nil {
		httpJSON(w, r, nil, err, 200)
		return
	}
	w.Header().Set("Trailer", "X-Export-Status")
	stream := &csvResponseWriter{ResponseWriter: w}
	if err := portal.NewService(h.Service.pool, h.Service.key, h.Service.clock, h.Service.random).ExportOrders(r.Context(), actor, q, stream); err != nil {
		if stream.started {
			w.Header().Set("X-Export-Status", "failed")
		} else {
			httpJSON(w, r, nil, err, 500)
		}
		return
	}
	if stream.started {
		w.Header().Set("X-Export-Status", "complete")
	}
}

type csvResponseWriter struct {
	http.ResponseWriter
	started bool
}

func (w *csvResponseWriter) Write(data []byte) (int, error) {
	if !w.started {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=orders.csv")
		w.WriteHeader(http.StatusOK)
		w.started = true
	}
	return w.ResponseWriter.Write(data)
}
func (h *Handler) Supplement(w http.ResponseWriter, r *http.Request) {
	var req SupplementRequest
	if decodePaymentJSON(w, r, &req) != nil {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	trade := chi.URLParam(r, "trade_no")
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		httpJSON(w, r, nil, apiErr("VALIDATION_ERROR", 400), 200)
		return
	}
	err := h.Service.SupplementIdempotent(r.Context(), actor, trade, req, key)
	var order any
	if err == nil {
		order, err = h.orderResponse(r, actor, trade)
	}
	httpJSON(w, r, order, err, 200)
}
func (h *Handler) MarkRefunded(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Confirm bool   `json:"confirm"`
		Reason  string `json:"reason"`
	}
	if decodePaymentJSON(w, r, &req) != nil {
		return
	}
	if !req.Confirm {
		httpJSON(w, r, nil, apiErr("VALIDATION_ERROR", 400), 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	trade := chi.URLParam(r, "trade_no")
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		httpJSON(w, r, nil, apiErr("VALIDATION_ERROR", 400), 200)
		return
	}
	err := h.Service.MarkRefundedIdempotent(r.Context(), actor, trade, req.Reason, key)
	var order any
	if err == nil {
		order, err = h.orderResponse(r, actor, trade)
	}
	httpJSON(w, r, order, err, 200)
}

func (h *Handler) orderResponse(r *http.Request, actor platform.Actor, trade string) (any, error) {
	_, err := h.Service.Query(r.Context(), actor, OrderSelector{TradeNo: trade, MerchantID: actor.MerchantID})
	if err != nil {
		return nil, err
	}
	detail, err := portal.NewService(h.Service.pool, h.Service.key, h.Service.clock, h.Service.random).OrderDetail(r.Context(), actor, trade)
	if err != nil {
		return nil, err
	}
	return detail["order"], nil
}
func (h *Handler) ResolveRefund(w http.ResponseWriter, r *http.Request) {
	var req RefundResolution
	if decodePaymentJSON(w, r, &req) != nil {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	refundID := chi.URLParam(r, "id")
	err := h.Service.ResolveRefund(r.Context(), actor, refundID, req)
	var data any
	if err == nil {
		var dto RefundDTO
		var outNo, reason, handledBy pgtype.Text
		var handledAt pgtype.Timestamptz
		err = h.Service.pool.QueryRow(r.Context(), `SELECT r.public_id,o.trade_no,r.out_refund_no,r.requested_cents,r.status,r.reason,r.requested_at,r.handled_at,r.handled_by::text FROM refunds r JOIN orders o ON o.id=r.order_id WHERE r.public_id=$1 AND o.merchant_id=$2`, refundID, actor.MerchantID).Scan(&dto.ID, &dto.TradeNo, &outNo, &dto.AmountCents, &dto.Status, &reason, &dto.RequestedAt, &handledAt, &handledBy)
		if err == nil {
			if outNo.Valid {
				value := outNo.String
				dto.OutRefundNo = &value
			}
			if reason.Valid {
				value := reason.String
				dto.Reason = &value
			}
			if handledAt.Valid {
				value := handledAt.Time
				dto.HandledAt = &value
			}
			if handledBy.Valid {
				value := handledBy.String
				dto.HandledBy = &value
			}
			data = dto
		}
	}
	httpJSON(w, r, data, err, 200)
}
func (h *Handler) CallbackReplay(w http.ResponseWriter, r *http.Request) {
	var req struct{}
	if decodePaymentJSON(w, r, &req) != nil {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		httpJSON(w, r, nil, apiErr("VALIDATION_ERROR", 400), 200)
		return
	}
	event, err := h.Service.ReplayCallback(r.Context(), actor, chi.URLParam(r, "trade_no"), key)
	httpJSON(w, r, event, err, 202)
}

func (h *Handler) realIP(r *http.Request) string {
	if metadata, ok := platform.RequestMetadataFromContext(r.Context()); ok {
		return metadata.IP
	}
	ip := platform.ResolveClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), h.TrustedProxyCIDRs)
	if !ip.IsValid() {
		return ""
	}
	return ip.String()
}

func (h *Handler) requestActor(r *http.Request, merchantID int64) platform.Actor {
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID, IP: h.realIP(r), RequestID: middleware.GetReqID(r.Context())}
	if metadata, ok := platform.RequestMetadataFromContext(r.Context()); ok {
		actor.IP = metadata.IP
		actor.RequestID = metadata.RequestID
	}
	return actor
}

func parseProtocolParams(r *http.Request) (url.Values, error) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		return nil, errors.New("unsupported method")
	}
	if r.Method == http.MethodPost {
		mediaType, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.EqualFold(mediaType, "application/x-www-form-urlencoded") || parameters["charset"] != "" && !strings.EqualFold(parameters["charset"], "utf-8") {
			return nil, errors.New("form content type required")
		}
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	for _, v := range query {
		if len(v) != 1 {
			return nil, errors.New("duplicate query parameter")
		}
		if !utf8.ValidString(v[0]) {
			return nil, errors.New("query parameter is not valid UTF-8")
		}
	}
	if r.Method == http.MethodGet {
		return query, nil
	}
	if r.ContentLength > 64<<10 {
		return nil, errors.New("request body too large")
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(body) {
		return nil, errors.New("form body is not valid UTF-8")
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	for _, v := range form {
		if len(v) != 1 {
			return nil, errors.New("duplicate form parameter")
		}
		if !utf8.ValidString(v[0]) {
			return nil, errors.New("form parameter is not valid UTF-8")
		}
	}
	for k := range query {
		if _, ok := form[k]; ok {
			return nil, errors.New("parameter supplied in query and form")
		}
	}
	for k, v := range form {
		query[k] = v
	}
	return query, nil
}
func decodePaymentJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		httpJSON(w, r, nil, apiErr("VALIDATION_ERROR", 400), 200)
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		err := errors.New("invalid trailing JSON")
		httpJSON(w, r, nil, apiErr("VALIDATION_ERROR", 400), 200)
		return err
	}
	return nil
}
func httpJSON(w http.ResponseWriter, r *http.Request, data any, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	id := middleware.GetReqID(r.Context())
	if err != nil {
		var api *platform.Error
		code := "INTERNAL_ERROR"
		if errors.As(err, &api) {
			code = api.Code
			if api.HTTPStatus > 0 {
				status = api.HTTPStatus
			} else {
				status = 500
			}
		} else {
			status = 500
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": code, "fields": map[string]string{}, "details": map[string]any{}}, "request_id": id})
		return
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "error": nil, "request_id": id})
}
func writeProtocolJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func v1Error(w http.ResponseWriter, code string) {
	v1JSON(w, 200, map[string]any{"code": -1, "msg": code, "error_code": code})
}
func v1JSON(w http.ResponseWriter, status int, value any) { writeProtocolJSON(w, status, value) }
func errorCode(err error) string {
	var api *platform.Error
	if errors.As(err, &api) {
		return api.Code
	}
	if errors.Is(err, ErrInvalidSignature) {
		return "INVALID_SIGNATURE"
	}
	return "INTERNAL_ERROR"
}
func validateV2Capabilities(p url.Values) error {
	for _, name := range []string{"auth_code", "sub_openid", "sub_appid", "is_applet", "cert_no", "cert_name", "min_age"} {
		if p.Get(name) != "" {
			return apiErr("UNSUPPORTED_METHOD", 400)
		}
	}
	for _, name := range []string{"merchant_id", "channel_id"} {
		if p.Get(name) != "" {
			return apiErr("UNSUPPORTED_PARAMETER", 400)
		}
	}
	if p.Get("fee_mode") != "" && p.Get("fee_mode") != "0" {
		return apiErr("UNSUPPORTED_PARAMETER", 400)
	}
	method := p.Get("method")
	if method != "" && method != "web" && method != "jump" {
		return apiErr("UNSUPPORTED_METHOD", 400)
	}
	return nil
}
func (h *Handler) protocolOrder(d OrderDTO) map[string]any {
	status := 0
	switch d.Status {
	case "paid":
		status = 1
	case "refunded":
		status = 2
	}
	channel := ""
	if d.Channel != nil {
		channel = *d.Channel
	}
	add := d.CreatedAt.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
	var end any
	if d.PaidAt != nil {
		end = d.PaidAt.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
	}
	actual := any(nil)
	if d.ActualCents != nil {
		actual = formatMoney(*d.ActualCents)
	}
	return map[string]any{"code": 1, "msg": "success", "pid": d.MerchantID, "trade_no": d.TradeNo, "out_trade_no": d.OutTradeNo, "type": channel, "status": status, "addtime": add, "endtime": end, "name": d.Name, "money": formatMoney(d.OriginalCents), "param": d.Param, "buyer": "", "clientip": "", "product_id": d.ProductID, "order_status": d.Status, "expire_reason": d.ExpireReason, "actual_money": actual, "expires_at": d.ExpiresAt, "refund_status": nil, "refund_no": nil, "refunded_at": d.RefundedAt}
}
func parseOrderFilter(r *http.Request) (portal.OrderFilter, error) {
	start, err := parseFilterTime(r.URL.Query().Get("start"))
	if err != nil {
		return portal.OrderFilter{}, err
	}
	end, err := parseFilterTime(r.URL.Query().Get("end"))
	if err != nil {
		return portal.OrderFilter{}, err
	}
	if start != nil && end != nil && !end.After(*start) {
		return portal.OrderFilter{}, apiErr("VALIDATION_ERROR", 400)
	}
	return portal.OrderFilter{Start: start, End: end, Status: r.URL.Query().Get("status"), Channel: r.URL.Query().Get("channel"), Query: r.URL.Query().Get("query"), Cursor: r.URL.Query().Get("cursor"), Limit: parseIntDefault(r.URL.Query().Get("limit"), 20)}, nil
}
func parseFilterTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, apiErr("VALIDATION_ERROR", 400)
	}
	return &parsed, nil
}
func parseIntDefault(v string, fallback int) int {
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}
func containsValue(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func remoteAddrIP(value string) netip.Addr {
	host := value
	if h, _, err := net.SplitHostPort(value); err == nil {
		host = h
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}
	}
	return ip.Unmap()
}
func isTrustedProxy(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
func sha256Bytes(v []byte) []byte { sum := sha256.Sum256(v); return sum[:] }
