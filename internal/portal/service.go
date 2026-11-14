package portal

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/platform"
)

type Service struct {
	pool       *pgxpool.Pool
	repository Repository
	key        [32]byte
	clock      platform.Clock
	random     io.Reader
}

func NewService(pool *pgxpool.Pool, key [32]byte, clock platform.Clock, random io.Reader) *Service {
	if clock == nil {
		clock = platform.SystemClock{}
	}
	if random == nil {
		random = rand.Reader
	}
	return &Service{pool: pool, repository: NewRepository(pool), key: key, clock: clock, random: random}
}

func (s *Service) SaveAnnouncement(ctx context.Context, actor platform.Actor, in AnnouncementCommand) (AnnouncementDTO, error) {
	if actor.Role != "admin" {
		return AnnouncementDTO{}, apiError("FORBIDDEN", 403)
	}
	if len(strings.TrimSpace(in.Title.ZhCN)) == 0 || len(in.Title.ZhCN) > 300 || len(in.Title.En) > 300 || len(strings.TrimSpace(in.Body.ZhCN)) == 0 || len(in.Body.ZhCN) > 100000 || len(in.Body.En) > 100000 {
		return AnnouncementDTO{}, apiError("VALIDATION_ERROR", 400)
	}
	var id string
	var result AnnouncementDTO
	now := s.clock.Now()
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if in.ID == "" {
			var err error
			id, err = platform.RandomToken(s.random, 18)
			if err != nil {
				return err
			}
			title, _ := json.Marshal(in.Title)
			body, _ := json.Marshal(in.Body)
			if err := tx.QueryRow(ctx, `INSERT INTO announcements(public_id,title,body,popup,published_at) VALUES($1,$2,$3,$4,$5) RETURNING revision`, id, title, body, in.Popup, now).Scan(&result.Revision); err != nil {
				return err
			}
		} else {
			id = in.ID
			title, _ := json.Marshal(in.Title)
			body, _ := json.Marshal(in.Body)
			if in.Revision <= 0 {
				return apiError("VALIDATION_ERROR", 400)
			}
			err := tx.QueryRow(ctx, `UPDATE announcements SET revision=revision+1,title=$1,body=$2,popup=$3,updated_at=$4 WHERE public_id=$5 AND revision=$6 AND deleted_at IS NULL RETURNING revision,published_at`, title, body, in.Popup, now, id, in.Revision).Scan(&result.Revision, &result.PublishedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return apiError("VERSION_CONFLICT", 409)
			}
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM announcement_reads WHERE announcement_id=(SELECT id FROM announcements WHERE public_id=$1)`, id); err != nil {
				return err
			}
		}
		if result.PublishedAt.IsZero() {
			result.PublishedAt = now
		}
		metadata, _ := json.Marshal(map[string]any{"revision": result.Revision})
		if err := platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, Action: "announcement.save", TargetType: "announcement", TargetID: id, Result: "success", SafeMetadata: metadata}); err != nil {
			return err
		}
		result.ID = id
		result.Title = in.Title
		result.Body = in.Body
		result.Popup = in.Popup
		return nil
	})
	return result, err
}

func (s *Service) DeleteAnnouncement(ctx context.Context, actor platform.Actor, id string, revision int64) error {
	if actor.Role != "admin" {
		return apiError("FORBIDDEN", 403)
	}
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE announcements SET deleted_at=$1,updated_at=$1 WHERE public_id=$2 AND revision=$3 AND deleted_at IS NULL`, s.clock.Now(), id, revision)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return apiError("VERSION_CONFLICT", 409)
		}
		return platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, Action: "announcement.delete", TargetType: "announcement", TargetID: id, Result: "success", SafeMetadata: json.RawMessage(`{}`)})
	})
}

func (s *Service) ListAnnouncements(ctx context.Context, actor platform.Actor, q PageQuery) (Page[AnnouncementDTO], error) {
	if actor.Role != "admin" && actor.Role != "merchant" {
		return Page[AnnouncementDTO]{}, apiError("UNAUTHENTICATED", 401)
	}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return Page[AnnouncementDTO]{}, err
	}
	var cursor time.Time
	var after int64
	if q.Cursor != "" {
		cursor, after, err = s.decodeCursor(q.Cursor)
		if err != nil {
			return Page[AnnouncementDTO]{}, apiError("VALIDATION_ERROR", 400)
		}
	}
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM announcements WHERE deleted_at IS NULL`).Scan(&total); err != nil {
		return Page[AnnouncementDTO]{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT a.id,a.public_id,a.revision,a.title,a.body,a.popup,a.published_at,CASE WHEN $1::bigint=0 THEN false ELSE EXISTS(SELECT 1 FROM announcement_reads r WHERE r.merchant_id=$1 AND r.announcement_id=a.id AND r.revision=a.revision) END FROM announcements a WHERE a.deleted_at IS NULL AND ($2::timestamptz IS NULL OR (a.published_at,a.id)<($2,$3)) ORDER BY a.published_at DESC,a.id DESC LIMIT $4`, actor.MerchantID, nullTime(cursor), after, limit+1)
	if err != nil {
		return Page[AnnouncementDTO]{}, err
	}
	defer rows.Close()
	page := Page[AnnouncementDTO]{Items: []AnnouncementDTO{}, Total: total}
	for rows.Next() {
		var dto AnnouncementDTO
		var title, body []byte
		var internal int64
		if err := rows.Scan(&internal, &dto.ID, &dto.Revision, &title, &body, &dto.Popup, &dto.PublishedAt, &dto.Read); err != nil {
			return page, err
		}
		if err := json.Unmarshal(title, &dto.Title); err != nil {
			return page, err
		}
		if err := json.Unmarshal(body, &dto.Body); err != nil {
			return page, err
		}
		page.Items = append(page.Items, dto)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		page.Items = page.Items[:limit]
		var rowID int64
		if err := s.pool.QueryRow(ctx, `SELECT id FROM announcements WHERE public_id=$1`, last.ID).Scan(&rowID); err != nil {
			return page, err
		}
		token := s.encodeCursor(last.PublishedAt, rowID)
		page.NextCursor = &token
	}
	return page, nil
}

func (s *Service) ListNotifications(ctx context.Context, actor platform.Actor, q PageQuery) (NotificationPage, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return NotificationPage{}, apiError("UNAUTHENTICATED", 401)
	}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return NotificationPage{}, err
	}
	cursor := time.Time{}
	after := int64(0)
	if q.Cursor != "" {
		cursor, after, err = s.decodeCursor(q.Cursor)
		if err != nil {
			return NotificationPage{}, apiError("VALIDATION_ERROR", 400)
		}
	}
	var page NotificationPage
	page.Items = []NotificationDTO{}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE merchant_id=$1 AND ($2::boolean=false OR read_at IS NULL)`, actor.MerchantID, q.UnreadOnly).Scan(&page.Total); err != nil {
		return page, err
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE merchant_id=$1 AND read_at IS NULL`, actor.MerchantID).Scan(&page.UnreadCount); err != nil {
		return page, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id,public_id,kind,source_id,safe_payload,created_at,read_at FROM notifications WHERE merchant_id=$1 AND ($2::boolean=false OR read_at IS NULL) AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4)) ORDER BY created_at DESC,id DESC LIMIT $5`, actor.MerchantID, q.UnreadOnly, nullTime(cursor), after, limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var internal int64
		var dto NotificationDTO
		var raw []byte
		if err := rows.Scan(&internal, &dto.ID, &dto.Kind, &dto.SourceID, &raw, &dto.CreatedAt, &dto.ReadAt); err != nil {
			return page, err
		}
		var payload struct {
			Title   LocalizedText       `json:"title"`
			Summary LocalizedText       `json:"summary"`
			Action  *NotificationAction `json:"action"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return page, err
		}
		dto.Title = payload.Title
		dto.Summary = payload.Summary
		dto.Action = payload.Action
		page.Items = append(page.Items, dto)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		page.Items = page.Items[:limit]
		var internal int64
		if err := s.pool.QueryRow(ctx, `SELECT id FROM notifications WHERE public_id=$1`, last.ID).Scan(&internal); err != nil {
			return page, err
		}
		token := s.encodeCursor(last.CreatedAt, internal)
		page.NextCursor = &token
	}
	popups, err := s.pool.Query(ctx, `SELECT a.id,a.public_id,a.revision,a.title,a.body,a.popup,a.published_at,false FROM announcements a WHERE a.deleted_at IS NULL AND a.popup=true AND NOT EXISTS(SELECT 1 FROM announcement_reads r WHERE r.announcement_id=a.id AND r.revision=a.revision AND r.merchant_id=$1) ORDER BY a.published_at,a.id`, actor.MerchantID)
	if err != nil {
		return page, err
	}
	defer popups.Close()
	for popups.Next() {
		var internal int64
		var item AnnouncementDTO
		var title, body []byte
		if err := popups.Scan(&internal, &item.ID, &item.Revision, &title, &body, &item.Popup, &item.PublishedAt, &item.Read); err != nil {
			return page, err
		}
		_ = json.Unmarshal(title, &item.Title)
		_ = json.Unmarshal(body, &item.Body)
		page.PopupAnnouncements = append(page.PopupAnnouncements, item)
	}
	if err := popups.Err(); err != nil {
		return page, err
	}
	if page.PopupAnnouncements == nil {
		page.PopupAnnouncements = []AnnouncementDTO{}
	}
	return page, nil
}

func (s *Service) MarkRead(ctx context.Context, actor platform.Actor, req ReadRequest) (ReadResult, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return ReadResult{}, apiError("UNAUTHENTICATED", 401)
	}
	if req.All == (len(req.IDs) > 0) {
		return ReadResult{}, apiError("VALIDATION_ERROR", 400)
	}
	if req.All {
		_, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at=$1 WHERE merchant_id=$2 AND read_at IS NULL`, s.clock.Now(), actor.MerchantID)
		if err != nil {
			return ReadResult{}, err
		}
	} else {
		if len(req.IDs) > 100 {
			return ReadResult{}, apiError("VALIDATION_ERROR", 400)
		}
		_, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at=$1 WHERE merchant_id=$2 AND public_id=ANY($3::text[]) AND read_at IS NULL`, s.clock.Now(), actor.MerchantID, req.IDs)
		if err != nil {
			return ReadResult{}, err
		}
	}
	var count int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE merchant_id=$1 AND read_at IS NULL`, actor.MerchantID).Scan(&count)
	return ReadResult{UnreadCount: count}, err
}

func (s *Service) MarkAnnouncementRead(ctx context.Context, actor platform.Actor, id string, revision int64) error {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return apiError("UNAUTHENTICATED", 401)
	}
	if revision <= 0 {
		return apiError("VALIDATION_ERROR", 400)
	}
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var internal int64
		var current int64
		if err := tx.QueryRow(ctx, `SELECT id,revision FROM announcements WHERE public_id=$1 AND deleted_at IS NULL`, id).Scan(&internal, &current); err != nil {
			return apiError("NOT_FOUND", 404)
		}
		if current != revision {
			return apiError("VERSION_CONFLICT", 409)
		}
		_, err := tx.Exec(ctx, `INSERT INTO announcement_reads(merchant_id,announcement_id,revision) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, actor.MerchantID, internal, revision)
		return err
	})
}

func (s *Service) Stats(ctx context.Context, actor platform.Actor, q StatsQuery) (StatsDTO, error) {
	if actor.Role != "merchant" && actor.Role != "admin" {
		return StatsDTO{}, apiError("UNAUTHENTICATED", 401)
	}
	if actor.Role == "merchant" && actor.MerchantID <= 0 {
		return StatsDTO{}, apiError("UNAUTHENTICATED", 401)
	}
	q, err := normalizeStatsQuery(q)
	if err != nil {
		return StatsDTO{}, apiError("VALIDATION_ERROR", 400)
	}
	dto := StatsDTO{Start: q.Start.UTC(), End: q.End.UTC(), Timezone: q.Timezone, AmountBasis: "paid_at", OrderBasis: "created_at", StatusCounts: map[string]int64{"paid": 0, "pending": 0, "expired": 0}, Channels: map[string]ChannelStats{}, Trend: []TrendPoint{}}
	for _, ch := range platform.PaymentChannels() {
		dto.Channels[ch] = ChannelStats{}
	}
	merchantID := any(nil)
	if actor.Role == "merchant" {
		merchantID = actor.MerchantID
	}
	rows, err := s.repository.StatsStatusRows(ctx, merchantID, q.Start, q.End)
	if err != nil {
		return dto, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var channel *string
		var count int64
		if err := rows.Scan(&status, &channel, &count); err != nil {
			return dto, err
		}
		dto.StatusCounts[status] += count
		if channel == nil {
			dto.UnassignedCount += count
			continue
		}
		ch, ok := dto.Channels[*channel]
		if !ok {
			continue
		}
		ch.OrderCount += count
		switch status {
		case "paid":
			ch.PaidCount += count
		case "pending":
			ch.PendingCount += count
		case "expired":
			ch.ExpiredCount += count
		}
		dto.Channels[*channel] = ch
	}
	if err := rows.Err(); err != nil {
		return dto, err
	}
	amountRowsByChannel, err := s.repository.StatsChannelAmountRows(ctx, merchantID, q.Start, q.End)
	if err != nil {
		return dto, err
	}
	for amountRowsByChannel.Next() {
		var channel string
		var amount int64
		if err := amountRowsByChannel.Scan(&channel, &amount); err != nil {
			amountRowsByChannel.Close()
			return dto, err
		}
		stats, exists := dto.Channels[channel]
		if exists {
			stats.AmountCents = amount
			dto.Channels[channel] = stats
		}
	}
	if err := amountRowsByChannel.Err(); err != nil {
		amountRowsByChannel.Close()
		return dto, err
	}
	amountRowsByChannel.Close()
	if dto.TotalAmountCents, err = s.repository.StatsTotalAmount(ctx, merchantID, q.Start, q.End); err != nil {
		return dto, err
	}
	if dto.TotalOrderCount, err = s.repository.StatsTotalOrders(ctx, merchantID, q.Start, q.End); err != nil {
		return dto, err
	}
	loc, _ := time.LoadLocation(q.Timezone)
	localStart := q.Start.In(loc)
	startDay := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, loc)
	endDay := q.End.In(loc)
	daily := map[string]*TrendPoint{}
	for day := startDay; day.Before(endDay); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		daily[key] = &TrendPoint{Date: key}
	}
	countRows, err := s.repository.StatsTrendCountRows(ctx, merchantID, q.Start, q.End, q.Timezone)
	if err != nil {
		return dto, err
	}
	defer countRows.Close()
	for countRows.Next() {
		var date, status string
		var n int64
		if err := countRows.Scan(&date, &status, &n); err != nil {
			return dto, err
		}
		if point := daily[date]; point != nil {
			switch status {
			case "paid":
				point.PaidCount += n
			case "pending":
				point.PendingCount += n
			case "expired":
				point.ExpiredCount += n
			}
		}
	}
	if err := countRows.Err(); err != nil {
		return dto, err
	}
	amountRows, err := s.repository.StatsTrendAmountRows(ctx, merchantID, q.Start, q.End, q.Timezone)
	if err != nil {
		return dto, err
	}
	defer amountRows.Close()
	for amountRows.Next() {
		var date string
		var amount int64
		if err := amountRows.Scan(&date, &amount); err != nil {
			return dto, err
		}
		if point := daily[date]; point != nil {
			point.AmountCents = amount
		}
	}
	if err := amountRows.Err(); err != nil {
		return dto, err
	}
	for day := startDay; day.Before(endDay); day = day.AddDate(0, 0, 1) {
		if point := daily[day.Format("2006-01-02")]; point != nil {
			dto.Trend = append(dto.Trend, *point)
		}
	}
	if actor.Role == "admin" {
		merchants, active, revenue, err := s.repository.AdminStats(ctx, q.Start, q.End, s.clock.Now())
		if err != nil {
			return dto, err
		}
		dto.MerchantCount = &merchants
		dto.ActiveSubscriptionCount = &active
		dto.SubscriptionRevenueCents = &revenue
	}
	return dto, nil
}

func normalizeStatsQuery(q StatsQuery) (StatsQuery, error) {
	if q.Timezone == "" {
		q.Timezone = DefaultStatsTimezone
	}
	if _, err := time.LoadLocation(q.Timezone); err != nil {
		return StatsQuery{}, err
	}
	if !q.End.After(q.Start) || q.End.Sub(q.Start) > 366*24*time.Hour {
		return StatsQuery{}, errors.New("invalid statistics range")
	}
	return q, nil
}

func (s *Service) ListOrders(ctx context.Context, actor platform.Actor, q OrderFilter) (Page[OrderDTO], error) {
	if actor.Role != "merchant" && actor.Role != "admin" {
		return Page[OrderDTO]{}, apiError("UNAUTHENTICATED", 401)
	}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return Page[OrderDTO]{}, err
	}
	mid := q.MerchantID
	if actor.Role == "merchant" {
		id := actor.MerchantID
		mid = &id
	} else if mid != nil && *mid <= 0 {
		return Page[OrderDTO]{}, apiError("VALIDATION_ERROR", 400)
	}
	return s.listOrders(ctx, mid, q, limit)
}

func (s *Service) OrderDetail(ctx context.Context, actor platform.Actor, tradeNo string) (map[string]any, error) {
	if actor.Role != "merchant" && actor.Role != "admin" {
		return nil, apiError("UNAUTHENTICATED", 401)
	}
	if strings.TrimSpace(tradeNo) == "" {
		return nil, apiError("NOT_FOUND", 404)
	}
	mid := any(nil)
	if actor.Role == "merchant" {
		mid = actor.MerchantID
	}
	var id int64
	var notify, returnURL, ip, keyVersion pgtype.Text
	var receipt pgtype.Text
	if err := s.pool.QueryRow(ctx, `SELECT o.id,o.notify_url,o.return_url,host(o.clientip),o.signing_key_version::text,(SELECT r.public_id FROM receipts r WHERE r.order_id=o.id LIMIT 1) FROM orders o WHERE o.trade_no=$1 AND ($2::bigint IS NULL OR o.merchant_id=$2)`, tradeNo, mid).Scan(&id, &notify, &returnURL, &ip, &keyVersion, &receipt); err != nil {
		return nil, apiError("NOT_FOUND", 404)
	}
	var dto OrderDTO
	var ch, reason, manual pgtype.Text
	var actualCents pgtype.Int8
	var paid, refunded pgtype.Timestamptz
	var channels []string
	var callbackReplaysUsed int
	err := s.pool.QueryRow(ctx, `SELECT o.trade_no,o.out_trade_no,o.merchant_id,o.merchant_username_snapshot,o.purpose,o.protocol,o.channel,o.name,o.product_id,o.param,o.original_cents,o.actual_cents,o.status,o.expire_reason,o.is_manual,o.is_test,o.manual_reason,o.created_at,o.paid_at,o.expires_at,o.refunded_at,
		COALESCE(c.state,CASE WHEN o.purpose='subscription' THEN 'internal_fulfillment' ELSE 'none' END),r.public_id,o.allowed_channels_snapshot,GREATEST(COALESCE(c.generation,1)-1,0)
		FROM orders o LEFT JOIN receipts r ON r.order_id=o.id LEFT JOIN LATERAL (SELECT state,generation FROM callback_events WHERE order_id=o.id ORDER BY generation DESC LIMIT 1) c ON true WHERE o.id=$1`, id).
		Scan(&dto.TradeNo, &dto.OutTradeNo, &dto.MerchantID, &dto.MerchantUsername, &dto.Purpose, &dto.Protocol, &ch, &dto.Name, &dto.ProductID, &dto.Param, &dto.OriginalCents, &actualCents, &dto.Status, &reason, &dto.IsManual, &dto.IsTest, &manual, &dto.CreatedAt, &paid, &dto.ExpiresAt, &refunded, &dto.CallbackStatus, &receipt, &channels, &callbackReplaysUsed)
	if err != nil {
		return nil, err
	}
	if ch.Valid {
		v := ch.String
		dto.Channel = &v
	}
	if actualCents.Valid {
		v := actualCents.Int64
		dto.ActualCents = &v
	}
	if reason.Valid {
		v := reason.String
		dto.ExpireReason = &v
	}
	if manual.Valid {
		v := manual.String
		dto.ManualReason = &v
	}
	if paid.Valid {
		v := paid.Time
		dto.PaidAt = &v
	}
	if refunded.Valid {
		v := refunded.Time
		dto.RefundedAt = &v
	}
	if receipt.Valid {
		v := receipt.String
		dto.ReceiptID = &v
	}
	var refund *RefundDTO
	var rd RefundDTO
	var outNo, rr, handledBy pgtype.Text
	var handled pgtype.Timestamptz
	err = s.pool.QueryRow(ctx, `SELECT r.public_id,o.trade_no,r.out_refund_no,r.requested_cents,r.status,r.reason,r.requested_at,r.handled_at,r.handled_by::text FROM refunds r JOIN orders o ON o.id=r.order_id WHERE r.order_id=$1 ORDER BY r.requested_at DESC LIMIT 1`, id).Scan(&rd.ID, &rd.TradeNo, &outNo, &rd.AmountCents, &rd.Status, &rr, &rd.RequestedAt, &handled, &handledBy)
	if err == nil {
		if outNo.Valid {
			v := outNo.String
			rd.OutRefundNo = &v
		}
		if rr.Valid {
			v := rr.String
			rd.Reason = &v
		}
		if handled.Valid {
			v := handled.Time
			rd.HandledAt = &v
		}
		if handledBy.Valid {
			v := handledBy.String
			rd.HandledBy = &v
		}
		refund = &rd
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT a.public_id,c.public_id,c.generation,a.attempt_no,a.started_at,a.finished_at,a.http_status,a.duration_ms,a.response_excerpt,a.error_code,a.next_attempt_at,a.url_redacted FROM callback_attempts a JOIN callback_events c ON c.id=a.event_id WHERE c.order_id=$1 ORDER BY c.generation,a.attempt_no`, id)
	if err != nil {
		return nil, err
	}
	attempts := []map[string]any{}
	for rows.Next() {
		var aid, eid, urlRedacted, excerpt string
		var generation, number int
		var started time.Time
		var finished, next pgtype.Timestamptz
		var status, duration pgtype.Int8
		var code pgtype.Text
		if err := rows.Scan(&aid, &eid, &generation, &number, &started, &finished, &status, &duration, &excerpt, &code, &next, &urlRedacted); err != nil {
			rows.Close()
			return nil, err
		}
		item := map[string]any{"id": aid, "event_id": eid, "generation": generation, "attempt_no": number, "started_at": started, "finished_at": nil, "http_status": nil, "duration_ms": nil, "response_excerpt": excerpt, "error_code": nil, "next_attempt_at": nil, "url_redacted": urlRedacted}
		if finished.Valid {
			item["finished_at"] = finished.Time
		}
		if status.Valid {
			item["http_status"] = status.Int64
		}
		if duration.Valid {
			item["duration_ms"] = duration.Int64
		}
		if code.Valid {
			item["error_code"] = code.String
		}
		if next.Valid {
			item["next_attempt_at"] = next.Time
		}
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := map[string]any{"order": dto, "notify_url": nullText(notify), "return_url": nullText(returnURL), "key_version": nullText(keyVersion), "clientip": nullText(ip), "receipt_id": dto.ReceiptID, "callback_attempts": attempts, "refund": refund,
		"allowed_channels_snapshot": channels, "callback_replays_used": callbackReplaysUsed, "callback_replay_limit": math.MaxInt32 - 1}
	return out, nil
}

func (s *Service) listOrders(ctx context.Context, mid *int64, q OrderFilter, limit int) (Page[OrderDTO], error) {
	var page Page[OrderDTO]
	page.Items = []OrderDTO{}
	status := q.Status
	channel := q.Channel
	query := strings.TrimSpace(q.Query)
	if len(query) > 100 {
		return page, apiError("VALIDATION_ERROR", 400)
	}
	if status != "" && status != "paid" && status != "pending" && status != "expired" && status != "refunded" {
		return page, apiError("VALIDATION_ERROR", 400)
	}
	if channel != "" && !platform.ValidPaymentChannel(channel) {
		return page, apiError("VALIDATION_ERROR", 400)
	}
	cursorAt := time.Time{}
	cursorID := int64(0)
	if q.Cursor != "" {
		decodedAt, decodedID, decodeErr := s.decodeCursor(q.Cursor)
		if decodeErr != nil || decodedID <= 0 {
			return page, apiError("VALIDATION_ERROR", 400)
		}
		cursorAt, cursorID = decodedAt, decodedID
	}
	total, err := s.repository.CountOrders(ctx, mid, q, status, channel, query)
	if err != nil {
		return page, err
	}
	page.Total = total
	rows, err := s.repository.OrderRows(ctx, mid, q, status, channel, query, cursorAt, cursorID, limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	rowIDs := make([]int64, 0, limit+1)
	for rows.Next() {
		var d OrderDTO
		var rowID int64
		var ch, reason, manual pgtype.Text
		var paid, refunded pgtype.Timestamptz
		var actualCents pgtype.Int8
		var receipt pgtype.Text
		if err := rows.Scan(&rowID, &d.TradeNo, &d.OutTradeNo, &d.MerchantID, &d.MerchantUsername, &d.Purpose, &d.Protocol, &ch, &d.Name, &d.ProductID, &d.Param, &d.OriginalCents, &actualCents, &d.Status, &reason, &d.IsManual, &d.IsTest, &manual, &d.CreatedAt, &paid, &d.ExpiresAt, &refunded, &d.CallbackStatus, &receipt); err != nil {
			return page, err
		}
		if ch.Valid {
			v := ch.String
			d.Channel = &v
		}
		if actualCents.Valid {
			v := actualCents.Int64
			d.ActualCents = &v
		}
		if reason.Valid {
			v := reason.String
			d.ExpireReason = &v
		}
		if manual.Valid {
			v := manual.String
			d.ManualReason = &v
		}
		if paid.Valid {
			v := paid.Time
			d.PaidAt = &v
		}
		if refunded.Valid {
			v := refunded.Time
			d.RefundedAt = &v
		}
		if receipt.Valid {
			v := receipt.String
			d.ReceiptID = &v
		}
		page.Items = append(page.Items, d)
		rowIDs = append(rowIDs, rowID)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		token := s.encodeCursor(last.CreatedAt, rowIDs[limit-1])
		page.NextCursor = &token
		page.Items = page.Items[:limit]
	}
	return page, nil
}

func (s *Service) ListReceipts(ctx context.Context, actor platform.Actor, q ReceiptFilter) (Page[ReceiptDTO], error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return Page[ReceiptDTO]{}, apiError("UNAUTHENTICATED", 401)
	}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return Page[ReceiptDTO]{}, err
	}
	if q.Channel != "" && !platform.ValidPaymentChannel(q.Channel) || q.MatchState != "" && q.MatchState != "matched" && q.MatchState != "unmatched" {
		return Page[ReceiptDTO]{}, apiError("VALIDATION_ERROR", 400)
	}
	cursorAt := time.Time{}
	cursorID := int64(0)
	if q.Cursor != "" {
		cursorAt, cursorID, err = s.decodeCursor(q.Cursor)
		if err != nil || cursorID <= 0 {
			return Page[ReceiptDTO]{}, apiError("VALIDATION_ERROR", 400)
		}
	}
	var page Page[ReceiptDTO]
	page.Items = []ReceiptDTO{}
	if page.Total, err = s.repository.CountReceipts(ctx, actor.MerchantID, q); err != nil {
		return page, err
	}
	rows, err := s.repository.ReceiptRows(ctx, actor.MerchantID, q, cursorAt, cursorID, limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	rowIDs := make([]int64, 0, limit+1)
	for rows.Next() {
		var d ReceiptDTO
		var provider, payer, reason, trade pgtype.Text
		var eventID string
		var deviceID int64
		var rowID int64
		if err := rows.Scan(&rowID, &d.ID, &eventID, &deviceID, &d.Channel, &d.AmountCents, &d.OccurredAt, &d.ReceivedAt, &provider, &payer, &d.MatchState, &reason, &trade); err != nil {
			return page, err
		}
		d.EventID = eventID
		d.DeviceID = strconv.FormatInt(deviceID, 10)
		if provider.Valid {
			v := provider.String
			d.ProviderTradeNo = &v
		}
		if payer.Valid {
			v := payer.String
			d.PayerHint = &v
		}
		if reason.Valid {
			v := reason.String
			d.UnmatchedReason = &v
		}
		if trade.Valid {
			v := trade.String
			d.TradeNo = &v
		}
		page.Items = append(page.Items, d)
		rowIDs = append(rowIDs, rowID)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		token := s.encodeCursor(last.ReceivedAt, rowIDs[limit-1])
		page.NextCursor = &token
		page.Items = page.Items[:limit]
	}
	return page, nil
}

func (s *Service) ListAudits(ctx context.Context, actor platform.Actor, q AuditFilter) (Page[AuditDTO], error) {
	if actor.Role != "merchant" && actor.Role != "admin" {
		return Page[AuditDTO]{}, apiError("UNAUTHENTICATED", 401)
	}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return Page[AuditDTO]{}, err
	}
	mid := q.MerchantID
	if actor.Role == "merchant" {
		id := actor.MerchantID
		mid = &id
		q.ActorRole = ""
	} else if q.ActorRole != "" && q.ActorRole != "merchant" && q.ActorRole != "admin" && q.ActorRole != "system" && q.ActorRole != "device" {
		return Page[AuditDTO]{}, apiError("VALIDATION_ERROR", 400)
	}
	cursorAt := time.Time{}
	cursorID := int64(0)
	if q.Cursor != "" {
		cursorAt, cursorID, err = s.decodeCursor(q.Cursor)
		if err != nil || cursorID <= 0 {
			return Page[AuditDTO]{}, apiError("VALIDATION_ERROR", 400)
		}
	}
	var page Page[AuditDTO]
	page.Items = []AuditDTO{}
	if page.Total, err = s.repository.CountAudits(ctx, mid, q); err != nil {
		return page, err
	}
	rows, err := s.repository.AuditRows(ctx, mid, q, cursorAt, cursorID, limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var d AuditDTO
		var aid, midv pgtype.Int8
		var ip pgtype.Text
		var id int64
		if err := rows.Scan(&id, &d.ActorRole, &aid, &midv, &d.Action, &d.TargetType, &d.TargetID, &d.Result, &ip, &d.CreatedAt, &d.RequestID, &d.SafeMetadata); err != nil {
			return page, err
		}
		d.ID = strconv.FormatInt(id, 10)
		if aid.Valid {
			v := strconv.FormatInt(aid.Int64, 10)
			d.ActorID = &v
		}
		if midv.Valid {
			v := strconv.FormatInt(midv.Int64, 10)
			d.MerchantID = &v
		}
		if ip.Valid {
			v := ip.String
			d.IP = &v
		}
		page.Items = append(page.Items, d)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		lastID, parseErr := strconv.ParseInt(last.ID, 10, 64)
		if parseErr != nil || lastID <= 0 {
			return page, errors.New("invalid audit row ID")
		}
		token := s.encodeCursor(last.CreatedAt, lastID)
		page.NextCursor = &token
		page.Items = page.Items[:limit]
	}
	return page, nil
}

func (s *Service) ExportOrders(ctx context.Context, actor platform.Actor, q OrderFilter, dst io.Writer) error {
	if actor.Role != "merchant" && actor.Role != "admin" {
		return apiError("UNAUTHENTICATED", 401)
	}
	mid := q.MerchantID
	if actor.Role == "merchant" {
		id := actor.MerchantID
		mid = &id
	}
	err := s.exportOrders(ctx, actor, mid, q, dst)
	result := "success"
	code := ""
	if err != nil {
		result = "failure"
		code = "export_failed"
		var apiErr *platform.Error
		if errors.As(err, &apiErr) && apiErr.Code != "" {
			code = apiErr.Code
		}
	}
	meta, _ := json.Marshal(map[string]any{"filter_status": q.Status, "filter_channel": q.Channel, "result_code": code})
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	auditErr := platform.RecordAudit(auditCtx, s.pool, platform.AuditEvent{Actor: actor, MerchantID: mid, Action: "orders.export", TargetType: "order_export", TargetID: "csv", Result: result, SafeMetadata: meta})
	if err != nil {
		return errors.Join(err, auditErr)
	}
	return auditErr
}

func (s *Service) exportOrders(ctx context.Context, actor platform.Actor, mid *int64, q OrderFilter, dst io.Writer) error {
	tx, err := s.repository.BeginOrderExport(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := s.repository.ExportOrderRows(ctx, tx, mid, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	w := csv.NewWriter(dst)
	if _, err := io.WriteString(dst, "\xEF\xBB\xBF"); err != nil {
		return err
	}
	if err := w.Write([]string{"trade_no", "out_trade_no", "merchant_id", "merchant_username", "channel", "name", "product_id", "original_cents", "actual_cents", "status", "is_manual", "created_at", "paid_at", "expires_at", "refunded_at", "callback_status"}); err != nil {
		return err
	}
	for rows.Next() {
		var trade, outTrade, username, name, product, status, callback string
		var merchantID int64
		var channel, actual pgtype.Text
		var original int64
		var manual bool
		var created, expires time.Time
		var paid, refunded pgtype.Timestamptz
		if err := rows.Scan(&trade, &outTrade, &merchantID, &username, &channel, &name, &product, &original, &actual, &status, &manual, &created, &paid, &expires, &refunded, &callback); err != nil {
			return err
		}
		fields := []string{trade, outTrade, strconv.FormatInt(merchantID, 10), csvSafe(username), nullText(channel), csvSafe(name), csvSafe(product), strconv.FormatInt(original, 10), nullText(actual), status, strconv.FormatBool(manual), created.UTC().Format(time.RFC3339Nano), nullTimestamp(paid), expires.UTC().Format(time.RFC3339Nano), nullTimestamp(refunded), callback}
		if err := w.Write(fields); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func pageLimit(n int) (int, error) {
	if n == 0 {
		return 20, nil
	}
	if n < 1 || n > 100 {
		return 0, apiError("VALIDATION_ERROR", 400)
	}
	return n, nil
}
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
func nullTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
func nullText(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}
func nullTimestamp(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339Nano)
}
func csvSafe(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
}
func apiError(code string, status int) error { return &platform.Error{Code: code, HTTPStatus: status} }

func (s *Service) encodeCursor(at time.Time, id int64) string {
	payload := at.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(id, 10)
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + hex.EncodeToString(mac.Sum(nil))))
}
func (s *Service) decodeCursor(cursor string) (time.Time, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, err
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return time.Time{}, 0, fmt.Errorf("invalid cursor")
	}
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(parts[0] + "|" + parts[1]))
	provided, err := hex.DecodeString(parts[2])
	if err != nil || !hmac.Equal(provided, mac.Sum(nil)) {
		return time.Time{}, 0, fmt.Errorf("invalid cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, 0, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	return at, id, err
}
