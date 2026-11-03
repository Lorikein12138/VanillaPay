package merchant

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
)

type ConsoleMerchantQuery struct {
	Query              string
	Banned             *bool
	SubscriptionActive *bool
	Cursor             string
	Limit              int
	MerchantID         *int64
}

type ConsoleMerchantPage struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"next_cursor"`
	Total      int64            `json:"total"`
}

func (s *Service) ListConsole(ctx context.Context, actor platform.Actor, q ConsoleMerchantQuery) (ConsoleMerchantPage, error) {
	if actor.Role != "admin" {
		return ConsoleMerchantPage{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	if len([]rune(q.Query)) > 100 {
		return ConsoleMerchantPage{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.Limit < 1 || q.Limit > 100 {
		return ConsoleMerchantPage{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	var cursorTime *time.Time
	var cursorID *int64
	if q.Cursor != "" {
		at, id, err := s.decodeConsoleCursor(q.Cursor, q)
		if err != nil {
			return ConsoleMerchantPage{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
		}
		cursorTime, cursorID = &at, &id
	}
	var banned, active any
	if q.Banned != nil {
		banned = *q.Banned
	}
	if q.SubscriptionActive != nil {
		active = *q.SubscriptionActive
	}
	baseWhere := `($1='' OR position(lower($1) in lower(m.username))>0 OR position(lower($1) in lower(m.email_normalized))>0 OR position(lower($1) in lower(m.pid))>0) AND ($2::boolean IS NULL OR (m.banned_at IS NOT NULL)=$2) AND ($3::boolean IS NULL OR COALESCE(s.expires_at>now(),false)=$3) AND ($4::bigint IS NULL OR m.id=$4)`
	var page ConsoleMerchantPage
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM merchants m LEFT JOIN subscriptions s ON s.merchant_id=m.id WHERE `+baseWhere, q.Query, banned, active, q.MerchantID).Scan(&page.Total); err != nil {
		return page, err
	}
	rows, err := s.pool.Query(ctx, `SELECT m.id,m.username,m.email_normalized,m.pid,m.banned_at,m.ban_reason,m.totp_cipher,m.preferences,m.created_at,s.snapshot,s.expires_at,s.revision,s.source,d.public_id,d.name,d.app_version,d.last_seen_at FROM merchants m LEFT JOIN subscriptions s ON s.merchant_id=m.id LEFT JOIN devices d ON d.merchant_id=m.id AND d.revoked_at IS NULL WHERE `+baseWhere+` AND ($5::timestamptz IS NULL OR (m.created_at,m.id)<($5,$6)) ORDER BY m.created_at DESC,m.id DESC LIMIT $7`, q.Query, banned, active, q.MerchantID, cursorTime, cursorID, q.Limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	page.Items = make([]map[string]any, 0, q.Limit)
	var lastAt time.Time
	var lastID int64
	more := false
	for rows.Next() {
		var row consoleMerchantRow
		if err := rows.Scan(&row.ID, &row.Username, &row.Email, &row.PID, &row.BannedAt, &row.BanReason, &row.TOTPCipher, &row.Preferences, &row.CreatedAt, &row.SubscriptionSnapshot, &row.SubscriptionExpires, &row.SubscriptionRevision, &row.SubscriptionSource, &row.DeviceID, &row.DeviceName, &row.DeviceVersion, &row.DeviceLastSeen); err != nil {
			return page, err
		}
		if len(page.Items) == q.Limit {
			more = true
			break
		}
		page.Items = append(page.Items, row.dto(s.clock.Now()))
		lastAt, lastID = row.CreatedAt, row.ID
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if more && len(page.Items) > 0 {
		cursor := s.encodeConsoleCursor(lastAt, lastID, q)
		page.NextCursor = &cursor
	}
	return page, nil
}

func (s *Service) ConsoleDetail(ctx context.Context, actor platform.Actor, merchantID int64) (map[string]any, error) {
	if actor.Role != "admin" {
		return nil, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	if merchantID <= 0 {
		return nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
	}
	page, err := s.ListConsole(ctx, actor, ConsoleMerchantQuery{Limit: 1, MerchantID: &merchantID})
	if err != nil {
		return nil, err
	}
	if len(page.Items) == 0 {
		return nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
	}
	item := page.Items[0]
	user := make(map[string]any, 12)
	for key, value := range item {
		if key != "subscription" && key != "device" {
			user[key] = value
		}
	}
	collection, err := s.consoleCollection(ctx, merchantID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"user": user, "subscription": item["subscription"], "device": item["device"], "collection": collection}, nil
}

type consoleMerchantRow struct {
	ID                   int64
	Username, Email, PID string
	BannedAt             *time.Time
	BanReason            *string
	TOTPCipher           []byte
	Preferences          []byte
	CreatedAt            time.Time
	SubscriptionSnapshot []byte
	SubscriptionExpires  pgtype.Timestamptz
	SubscriptionRevision pgtype.Int8
	SubscriptionSource   pgtype.Text
	DeviceID             pgtype.Text
	DeviceName           pgtype.Text
	DeviceVersion        pgtype.Text
	DeviceLastSeen       pgtype.Timestamptz
}

func (r consoleMerchantRow) dto(now time.Time) map[string]any {
	user := auth.Account{ID: r.ID, Role: "merchant", Username: r.Username, Email: r.Email, PID: r.PID, BannedAt: r.BannedAt, BanReason: r.BanReason, TOTPCipher: r.TOTPCipher, Preferences: r.Preferences, CreatedAt: r.CreatedAt}.DTO()
	var plan any
	var channels []string
	if len(r.SubscriptionSnapshot) > 0 {
		var snapshot map[string]any
		if json.Unmarshal(r.SubscriptionSnapshot, &snapshot) == nil {
			plan = snapshot
			if rawChannels, ok := snapshot["channels"].([]any); ok {
				for _, item := range rawChannels {
					if channel, ok := item.(string); ok {
						channels = append(channels, channel)
					}
				}
			}
		}
	}
	var expires any
	active := false
	if r.SubscriptionExpires.Valid {
		expires = r.SubscriptionExpires.Time
		active = r.SubscriptionExpires.Time.After(now)
	}
	source := "none"
	if r.SubscriptionSource.Valid {
		source = r.SubscriptionSource.String
	}
	revision := int64(0)
	if r.SubscriptionRevision.Valid {
		revision = r.SubscriptionRevision.Int64
	}
	subscription := map[string]any{"plan": plan, "channels": channels, "expires_at": expires, "active": active, "source": source, "revision": revision}
	var device any
	if r.DeviceID.Valid {
		var lastSeen, offlineAt any
		online := false
		if r.DeviceLastSeen.Valid {
			lastSeen = r.DeviceLastSeen.Time
			offlineAt = r.DeviceLastSeen.Time.Add(90 * time.Second)
			online = r.DeviceLastSeen.Time.After(now.Add(-90*time.Second)) && !r.DeviceLastSeen.Time.After(now)
		}
		device = map[string]any{"id": r.DeviceID.String, "name": r.DeviceName.String, "app_version": r.DeviceVersion.String, "bound": true, "online": online, "last_seen_at": lastSeen, "offline_at": offlineAt, "heartbeat_interval_seconds": 30, "offline_threshold_seconds": 90}
	} else {
		device = map[string]any{"id": nil, "name": nil, "app_version": nil, "bound": false, "online": false, "last_seen_at": nil, "offline_at": nil, "heartbeat_interval_seconds": 30, "offline_threshold_seconds": 90}
	}
	data, _ := json.Marshal(user)
	var item map[string]any
	_ = json.Unmarshal(data, &item)
	item["subscription"] = subscription
	item["device"] = device
	return item
}

func (s *Service) consoleCollection(ctx context.Context, merchantID int64) (map[string]any, error) {
	var row struct {
		TTL       *int
		Limit     *int
		Direction *string
		Version   int64
	}
	err := s.pool.QueryRow(ctx, `SELECT order_ttl_seconds,float_limit_cents,float_direction,version FROM merchant_collection_settings WHERE merchant_id=$1`, merchantID).Scan(&row.TTL, &row.Limit, &row.Direction, &row.Version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	var global struct {
		DefaultTTL int    `json:"default_ttl_seconds"`
		MaxTTL     int    `json:"max_ttl_seconds"`
		FloatMin   int    `json:"float_min_cents"`
		FloatMax   int    `json:"float_max_cents"`
		FloatDef   int    `json:"float_default_cents"`
		FloatDir   string `json:"float_direction"`
	}
	var payment []byte
	if err := s.pool.QueryRow(ctx, `SELECT payment FROM site_settings WHERE singleton=true`).Scan(&payment); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(payment, &global); err != nil {
		return nil, err
	}
	effectiveTTL, effectiveLimit, effectiveDirection := global.DefaultTTL, global.FloatDef, global.FloatDir
	if row.TTL != nil {
		effectiveTTL = *row.TTL
	}
	if row.Limit != nil {
		effectiveLimit = *row.Limit
	}
	if row.Direction != nil {
		effectiveDirection = *row.Direction
	}
	return map[string]any{"order_ttl_seconds": row.TTL, "float_limit_cents": row.Limit, "float_direction": row.Direction, "effective": map[string]any{"order_ttl_seconds": effectiveTTL, "float_limit_cents": effectiveLimit, "float_direction": effectiveDirection}, "bounds": map[string]any{"ttl_max_seconds": global.MaxTTL, "float_min_cents": global.FloatMin, "float_max_cents": global.FloatMax}, "version": row.Version}, nil
}

func consoleFilterFingerprint(q ConsoleMerchantQuery) string {
	banned, active, merchant := "*", "*", "*"
	if q.Banned != nil {
		banned = strconv.FormatBool(*q.Banned)
	}
	if q.SubscriptionActive != nil {
		active = strconv.FormatBool(*q.SubscriptionActive)
	}
	if q.MerchantID != nil {
		merchant = strconv.FormatInt(*q.MerchantID, 10)
	}
	query := strings.Join([]string{strings.ToLower(q.Query), banned, active, merchant}, "|")
	digest := sha256.Sum256([]byte(query))
	return hex.EncodeToString(digest[:])
}

func (s *Service) encodeConsoleCursor(at time.Time, id int64, q ConsoleMerchantQuery) string {
	payload := at.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(id, 10) + "|" + consoleFilterFingerprint(q)
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + hex.EncodeToString(mac.Sum(nil))))
}

func (s *Service) decodeConsoleCursor(cursor string, q ConsoleMerchantQuery) (time.Time, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, err
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 || parts[2] != consoleFilterFingerprint(q) {
		return time.Time{}, 0, fmt.Errorf("invalid cursor")
	}
	payload := strings.Join(parts[:3], "|")
	provided, err := hex.DecodeString(parts[3])
	if err != nil {
		return time.Time{}, 0, err
	}
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(payload))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return time.Time{}, 0, fmt.Errorf("invalid cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, 0, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	return at, id, err
}
