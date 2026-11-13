package payment

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"vanillapay/internal/portal"
)

func (h *Handler) cashierURL(tradeNo string) string {
	return strings.TrimRight(h.PublicBaseURL, "/") + "/pay/" + url.PathEscape(tradeNo)
}

func formatProtocolTime(at time.Time) string {
	return at.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
}

func protocolOrderStatus(status string) int {
	switch status {
	case "paid":
		return 1
	case "refunded":
		return 2
	default:
		return 0
	}
}

func (h *Handler) authorizeProtocolIP(w http.ResponseWriter, r *http.Request, id int64, v2 bool) bool {
	if err := h.Service.CheckProtocolIP(r.Context(), id, h.realIP(r)); err != nil {
		h.protocolError(w, v2, errorCode(err))
		return false
	}
	return true
}

func (h *Handler) merchantInfoProtocol(w http.ResponseWriter, r *http.Request, id int64, _ url.Values, v2 bool) {
	if !h.authorizeProtocolIP(w, r, id, v2) {
		return
	}
	loc, err := time.LoadLocation(portal.DefaultStatsTimezone)
	if err != nil {
		h.protocolError(w, v2, "INTERNAL_ERROR")
		return
	}
	year, month, day := h.Service.clock.Now().In(loc).Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, loc)
	var pid string
	var active, total, countToday, countYesterday, moneyToday, moneyYesterday int64
	err = h.Service.pool.QueryRow(r.Context(), `
		SELECT m.pid, CASE WHEN m.banned_at IS NULL THEN 1 ELSE 0 END,
			count(o.id),
			count(*) FILTER (WHERE o.status='paid' AND o.paid_at >= $2 AND o.paid_at < $3),
			count(*) FILTER (WHERE o.status='paid' AND o.paid_at >= $4 AND o.paid_at < $2),
			COALESCE(sum(o.original_cents) FILTER (WHERE o.status='paid' AND o.paid_at >= $2 AND o.paid_at < $3),0),
			COALESCE(sum(o.original_cents) FILTER (WHERE o.status='paid' AND o.paid_at >= $4 AND o.paid_at < $2),0)
		FROM merchants m LEFT JOIN orders o ON o.merchant_id=m.id AND o.purpose='external'
		WHERE m.id=$1 GROUP BY m.id`, id, today, today.AddDate(0, 0, 1), today.AddDate(0, 0, -1)).
		Scan(&pid, &active, &total, &countToday, &countYesterday, &moneyToday, &moneyYesterday)
	if err != nil {
		h.protocolError(w, v2, "INTERNAL_ERROR")
		return
	}
	data := map[string]any{"pid": pid, "order_money_today": formatMoney(moneyToday), "order_money_lastday": formatMoney(moneyYesterday)}
	if v2 {
		data["code"], data["status"], data["order_num"] = 0, active, total
		data["order_num_today"], data["order_num_lastday"] = countToday, countYesterday
		h.v2JSON(w, http.StatusOK, data)
		return
	}
	data["code"], data["active"], data["orders"] = 1, active, total
	data["orders_today"], data["orders_lastday"] = countToday, countYesterday
	v1JSON(w, http.StatusOK, data)
}

func parseProtocolOrderPage(p url.Values) (limit, offset, status int, err error) {
	limit, status = 10, -1
	if raw := p.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 0 {
			return 0, 0, 0, apiErr("VALIDATION_ERROR", 400)
		}
		limit = min(limit, 50)
	}
	if raw := p.Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 10000 {
			return 0, 0, 0, apiErr("VALIDATION_ERROR", 400)
		}
	}
	if raw := p.Get("status"); raw != "" {
		status, err = strconv.Atoi(raw)
		if err != nil || status < 0 || status > 2 {
			return 0, 0, 0, apiErr("VALIDATION_ERROR", 400)
		}
	}
	return limit, offset, status, nil
}

func (h *Handler) merchantOrdersProtocol(w http.ResponseWriter, r *http.Request, id int64, p url.Values, v2 bool) {
	if !h.authorizeProtocolIP(w, r, id, v2) {
		return
	}
	limit, offset, status, err := parseProtocolOrderPage(p)
	if err != nil {
		h.protocolError(w, v2, errorCode(err))
		return
	}
	rows, err := h.Service.pool.Query(r.Context(), `
		WITH matching AS (
			SELECT id,created_at FROM orders
			WHERE merchant_id=$1 AND purpose='external'
				AND ($2=-1 OR ($2=0 AND status IN ('pending','expired')) OR ($2=1 AND status='paid') OR ($2=2 AND status='refunded'))
		), page AS (
			SELECT id FROM matching ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4
		)
		SELECT totals.count,o.trade_no,o.out_trade_no,COALESCE(o.channel,''),COALESCE(o.merchant_pid_snapshot::text,m.pid::text),
			o.created_at,o.paid_at,o.name,o.original_cents,o.param,o.buyer_hint,o.status,COALESCE(host(o.clientip),''),
			COALESCE((SELECT r.provider_trade_no FROM receipts r WHERE r.order_id=o.id AND o.status='paid' ORDER BY r.id LIMIT 1),'')
		FROM (SELECT count(*) FROM matching) totals LEFT JOIN page ON true
		LEFT JOIN orders o ON o.id=page.id JOIN merchants m ON m.id=$1
		ORDER BY o.created_at DESC,o.id DESC`, id, status, limit, offset)
	if err != nil {
		h.protocolError(w, v2, "INTERNAL_ERROR")
		return
	}
	defer rows.Close()
	var count int64
	items := []map[string]any{}
	for rows.Next() {
		var trade, out, name, param, buyer, state *string
		var channel, pid, ip, provider string
		var created, paid *time.Time
		var cents *int64
		if err := rows.Scan(&count, &trade, &out, &channel, &pid, &created, &paid, &name, &cents, &param, &buyer, &state, &ip, &provider); err != nil {
			h.protocolError(w, v2, "INTERNAL_ERROR")
			return
		}
		if trade == nil {
			break
		}
		var end any
		if paid != nil {
			end = formatProtocolTime(*paid)
		}
		item := map[string]any{
			"trade_no": *trade, "out_trade_no": *out, "type": channel, "pid": pid,
			"addtime": formatProtocolTime(*created), "endtime": end, "name": *name,
			"money": formatMoney(*cents), "param": *param, "buyer": *buyer, "status": protocolOrderStatus(*state),
		}
		if v2 {
			item["api_trade_no"], item["clientip"], item["refundmoney"] = provider, ip, ""
			if *state == "refunded" {
				item["refundmoney"] = formatMoney(*cents)
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		h.protocolError(w, v2, "INTERNAL_ERROR")
		return
	}
	rows.Close()
	data := map[string]any{"code": 1, "msg": "success", "count": count, "data": items}
	if v2 {
		data["code"] = 0
		h.v2JSON(w, http.StatusOK, data)
		return
	}
	v1JSON(w, http.StatusOK, data)
}

func (h *Handler) closeProtocol(w http.ResponseWriter, r *http.Request, id int64, p url.Values) {
	if !h.authorizeProtocolIP(w, r, id, true) {
		return
	}
	trade, out := p.Get("trade_no"), p.Get("out_trade_no")
	if trade == "" && out == "" {
		h.protocolError(w, true, "VALIDATION_ERROR")
		return
	}
	if err := h.Service.closeProtocolOrder(r.Context(), h.requestActor(r, id), trade, out); err != nil {
		code := errorCode(err)
		msg := code
		switch code {
		case "NOT_FOUND":
			msg = "当前订单不存在"
		case "ORDER_NOT_PAYABLE":
			msg = "该订单状态不支持关闭"
		}
		h.v2JSON(w, http.StatusOK, map[string]any{"code": -1, "msg": msg, "error_code": code})
		return
	}
	h.v2JSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "订单关闭成功"})
}

func (h *Handler) refundQueryProtocol(w http.ResponseWriter, r *http.Request, id int64, p url.Values) {
	if !h.authorizeProtocolIP(w, r, id, true) {
		return
	}
	refund, out := p.Get("refund_no"), p.Get("out_refund_no")
	if refund == "" && out == "" {
		h.protocolError(w, true, "VALIDATION_ERROR")
		return
	}
	var trade, outTrade, state string
	var cents int64
	var requested time.Time
	var handled *time.Time
	err := h.Service.pool.QueryRow(r.Context(), `
		SELECT r.public_id,COALESCE(r.out_refund_no,''),o.trade_no,o.out_trade_no,r.requested_cents,r.status,r.requested_at,r.handled_at
		FROM refunds r JOIN orders o ON o.id=r.order_id AND o.merchant_id=r.merchant_id
		WHERE r.merchant_id=$1 AND o.purpose='external' AND ($2='' OR r.public_id=$2) AND ($3='' OR r.out_refund_no=$3)`, id, refund, out).
		Scan(&refund, &out, &trade, &outTrade, &cents, &state, &requested, &handled)
	if errors.Is(err, pgx.ErrNoRows) {
		h.protocolError(w, true, "NOT_FOUND")
		return
	}
	if err != nil {
		h.protocolError(w, true, "INTERNAL_ERROR")
		return
	}
	status := 0
	switch state {
	case "completed":
		status = 1
	case "rejected":
		status = 2
	}
	end := ""
	if handled != nil {
		end = formatProtocolTime(*handled)
	}
	h.v2JSON(w, http.StatusOK, map[string]any{
		"code": 0, "refund_no": refund, "out_refund_no": out, "trade_no": trade, "out_trade_no": outTrade,
		"money": formatMoney(cents), "status": status, "refund_status": state, "addtime": formatProtocolTime(requested), "endtime": end,
	})
}
