package device

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/mailtmpl"
	"vanillapay/internal/platform"
)

type PaymentPort interface {
	ApplyPaidReceiptTx(context.Context, pgx.Tx, int64, int64, string, string, string) error
}
type Service struct {
	pool          *pgxpool.Pool
	repository    Repository
	key           [32]byte
	publicBaseURL string
	clock         platform.Clock
	random        io.Reader
	payments      PaymentPort
}

func NewService(pool *pgxpool.Pool, key [32]byte, publicBaseURL string, clock platform.Clock, random io.Reader, payments PaymentPort) *Service {
	if clock == nil {
		clock = platform.SystemClock{}
	}
	if random == nil {
		random = rand.Reader
	}
	return &Service{pool: pool, repository: NewRepository(pool), key: key, publicBaseURL: strings.TrimRight(publicBaseURL, "/"), clock: clock, random: random, payments: payments}
}

func (s *Service) Authenticate(ctx context.Context, token string) (DeviceIdentity, error) {
	if token == "" {
		return DeviceIdentity{}, deviceErr("DEVICE_UNAUTHORIZED", 401)
	}
	id, err := s.repository.DeviceIdentityByTokenHash(ctx, hash(token))
	if err != nil {
		return DeviceIdentity{}, deviceErr("DEVICE_UNAUTHORIZED", 401)
	}
	return id, nil
}

func (s *Service) Heartbeat(ctx context.Context, device DeviceIdentity, req HeartbeatRequest) (DeviceStatus, error) {
	if device.DeviceID <= 0 || device.MerchantID <= 0 {
		return DeviceStatus{}, deviceErr("DEVICE_UNAUTHORIZED", 401)
	}
	if len(req.AppVersion) > 64 {
		return DeviceStatus{}, deviceErr("VALIDATION_ERROR", 400)
	}
	var result DeviceStatus
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{device.MerchantID}, false); err != nil {
			return err
		}
		state, err := lockHeartbeatState(ctx, tx, device.DeviceID, device.MerchantID)
		if errors.Is(err, pgx.ErrNoRows) {
			return deviceErr("DEVICE_UNAUTHORIZED", 401)
		}
		if err != nil {
			return err
		}
		now := s.clock.Now().Truncate(time.Microsecond)
		if state.lastSeen == nil || !now.Equal(*state.lastSeen) {
			if state.armed && state.everOnline && state.isOffline(now) {
				claimed, err := s.claimOfflineTx(ctx, tx, device.DeviceID, device.MerchantID, now)
				if err != nil {
					return err
				}
				if claimed {
					state.armed = false
				}
			}
			state.record(now)
			if _, err := tx.Exec(ctx, `UPDATE devices SET last_seen_at=$1,ever_online=true,app_version=$2,
				updated_at=GREATEST(updated_at,$1),online_since=$3,offline_alert_armed=$4,
				recovery_count=$5,last_counted_heartbeat_at=$6 WHERE id=$7`,
				now, strings.TrimSpace(req.AppVersion), state.onlineSince, state.armed,
				state.count, state.lastCounted, device.DeviceID); err != nil {
				return err
			}
		}
		var banned bool
		if err := tx.QueryRow(ctx, `SELECT banned_at IS NOT NULL FROM merchants WHERE id=$1`, device.MerchantID).Scan(&banned); err != nil {
			return err
		}
		result = DeviceStatus{ServerTime: now, Online: true, HealthStatus: healthStatus(now, true, state.lastSeen, state.count), MerchantBanned: banned, HeartbeatIntervalSeconds: heartbeatIntervalSeconds}
		return nil
	})
	return result, err
}

const offlineEmailEnabledSQL = "(m.preferences->'device_offline_email_enabled') IS DISTINCT FROM 'false'::jsonb"

func lockHeartbeatState(ctx context.Context, tx pgx.Tx, deviceID, merchantID int64) (heartbeatState, error) {
	var state heartbeatState
	err := tx.QueryRow(ctx, `
		SELECT last_seen_at,last_counted_heartbeat_at,online_since,ever_online,offline_alert_armed,recovery_count
		FROM devices WHERE id=$1 AND merchant_id=$2 AND revoked_at IS NULL FOR UPDATE`, deviceID, merchantID).
		Scan(&state.lastSeen, &state.lastCounted, &state.onlineSince, &state.everOnline, &state.armed, &state.count)
	return state, err
}

func (s *Service) Device(ctx context.Context, actor platform.Actor) (DeviceDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return DeviceDTO{}, deviceErr("UNAUTHENTICATED", 401)
	}
	var dto DeviceDTO
	var recoveryCount int16
	if err := s.pool.QueryRow(ctx, `
		SELECT d.public_id,d.name,d.app_version,d.last_seen_at,COALESCE(d.recovery_count,0::smallint),`+offlineEmailEnabledSQL+`
		FROM merchants m LEFT JOIN devices d ON d.merchant_id=m.id AND d.revoked_at IS NULL
		WHERE m.id=$1`, actor.MerchantID).
		Scan(&dto.ID, &dto.Name, &dto.AppVersion, &dto.LastSeenAt, &recoveryCount, &dto.DeviceOfflineEmailEnabled); err != nil {
		return dto, err
	}
	dto.SetHealth(s.clock.Now(), recoveryCount)
	return dto, nil
}

func (s *Service) Revoke(ctx context.Context, actor platform.Actor) error {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return deviceErr("UNAUTHENTICATED", 401)
	}
	return s.revoke(ctx, actor, 0)
}

func (s *Service) Unbind(ctx context.Context, actor platform.Actor) error {
	if actor.Role != "device" || actor.SubjectID <= 0 || actor.MerchantID <= 0 {
		return deviceErr("DEVICE_UNAUTHORIZED", 401)
	}
	return s.revoke(ctx, actor, actor.SubjectID)
}

func (s *Service) revoke(ctx context.Context, actor platform.Actor, deviceID int64) error {
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `UPDATE devices SET revoked_at=$1,updated_at=$1 WHERE merchant_id=$2 AND revoked_at IS NULL AND ($3::bigint=0 OR id=$3) RETURNING public_id`, s.clock.Now(), actor.MerchantID, deviceID)
		if err != nil {
			return err
		}
		var publicIDs []string
		for rows.Next() {
			var publicID string
			if err := rows.Scan(&publicID); err != nil {
				rows.Close()
				return err
			}
			publicIDs = append(publicIDs, publicID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if deviceID != 0 && len(publicIDs) == 0 {
			return deviceErr("DEVICE_UNAUTHORIZED", 401)
		}
		for _, publicID := range publicIDs {
			metadata, _ := json.Marshal(map[string]any{"device_id": publicID})
			if err := platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: &actor.MerchantID, Action: "device.revoke", TargetType: "device", TargetID: publicID, Result: "success", SafeMetadata: metadata}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) DetectOffline(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	list, err := sqlc.New(s.pool).ListOfflineDevices(ctx, sqlc.ListOfflineDevicesParams{
		Cutoff:    pgtype.Timestamptz{Time: s.clock.Now().Add(-offlineThreshold), Valid: true},
		BatchSize: int32(limit),
	})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, c := range list {
		queued := false
		err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			queued = false
			if err := sqlc.LockBusinessRows(ctx, tx, []int64{c.MerchantID}, false); err != nil {
				return err
			}
			state, err := lockHeartbeatState(ctx, tx, c.ID, c.MerchantID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			now := s.clock.Now().Truncate(time.Microsecond)
			if state.armed && state.everOnline && state.isOffline(now) {
				queued, err = s.claimOfflineTx(ctx, tx, c.ID, c.MerchantID, now)
			}
			return err
		})
		if err != nil {
			return count, err
		}
		if queued {
			count++
		}
	}
	return count, nil
}

// claimOfflineTx uses the preference and heartbeat under the business and device
// locks, so a late heartbeat and the scanner can handle the same outage once.
func (s *Service) claimOfflineTx(ctx context.Context, tx pgx.Tx, deviceID, merchantID int64, now time.Time) (bool, error) {
	var name, email, locale string
	var last time.Time
	var next int64
	var emailEnabled bool
	err := tx.QueryRow(ctx, `
		UPDATE devices d SET offline_alert_armed=false,online_since=NULL,
			recovery_count=0,last_counted_heartbeat_at=NULL,
			offline_notified_episode=d.offline_notified_episode+1
		FROM merchants m WHERE d.id=$1 AND d.merchant_id=m.id AND m.id=$2 AND d.revoked_at IS NULL
			AND d.ever_online AND d.offline_alert_armed AND d.last_seen_at<=$3
		RETURNING d.offline_notified_episode,d.name,d.last_seen_at,m.email_normalized,COALESCE(m.preferences->>'locale',''),`+offlineEmailEnabledSQL,
		deviceID, merchantID, now.Add(-offlineThreshold)).Scan(&next, &name, &last, &email, &locale, &emailEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	dedupe := fmt.Sprintf("device_offline:%d:%d", deviceID, next)
	id, err := platform.RandomToken(s.random, 18)
	if err != nil {
		return false, err
	}
	payload, _ := json.Marshal(map[string]any{
		"title":   map[string]string{"zh_CN": "设备离线", "en": "Device offline"},
		"summary": map[string]string{"zh_CN": "设备「" + name + "」已至少 5 分钟（300 秒）未收到心跳", "en": "Device “" + name + "” has missed heartbeats for at least 5 minutes (300 seconds)"},
		"action":  map[string]string{"page": "/dashboard/collection", "entity_id": "device"},
	})
	if _, err := tx.Exec(ctx, `INSERT INTO notifications(public_id,merchant_id,kind,source_id,dedupe_key,safe_payload) VALUES($1,$2,'device_offline',$3,$4,$5) ON CONFLICT(dedupe_key) DO NOTHING`, id, merchantID, fmt.Sprint(deviceID), dedupe, payload); err != nil {
		return false, err
	}
	if emailEnabled {
		if err := s.queueMailTx(ctx, tx, email, mailtmpl.DeviceOffline, dedupe, mailtmpl.Payload{Locale: locale, DeviceName: name, LastHeartbeat: &last}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Service) queueMailTx(ctx context.Context, tx pgx.Tx, recipient, template, dedupe string, payload mailtmpl.Payload) error {
	recipientCipher, err := platform.Encrypt(s.key[:], []byte(recipient), s.random)
	if err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	payloadCipher, err := platform.Encrypt(s.key[:], data, s.random)
	if err != nil {
		return err
	}
	id, err := platform.RandomToken(s.random, 18)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO mail_deliveries(public_id,dedupe_key,template,recipient_cipher,payload_cipher,state) VALUES($1,$2,$3,$4,$5,'queued') ON CONFLICT(dedupe_key) DO NOTHING`, id, dedupe, template, recipientCipher, payloadCipher)
	return err
}

func (s *Service) OpenReceipt(ctx context.Context, actor platform.Actor, id string) (map[string]any, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return nil, deviceErr("UNAUTHENTICATED", 401)
	}
	var result struct {
		ID              string
		EventID         string
		DeviceID        string
		Channel         string
		AmountCents     int64
		OccurredAt      time.Time
		ReceivedAt      time.Time
		ProviderTradeNo *string
		PayerHint       *string
		MatchState      string
		UnmatchedReason *string
		TradeNo         *string
		RawTextCipher   []byte
	}
	if err := s.pool.QueryRow(ctx, `SELECT r.public_id,r.event_id::text,d.public_id,r.channel,r.amount_cents,r.occurred_at,r.received_at,r.provider_trade_no,r.payer_hint,r.match_state,r.unmatched_reason,o.trade_no,r.raw_text_cipher FROM receipts r JOIN devices d ON d.id=r.device_id LEFT JOIN orders o ON o.id=r.order_id WHERE r.public_id=$1 AND r.merchant_id=$2`, id, actor.MerchantID).Scan(&result.ID, &result.EventID, &result.DeviceID, &result.Channel, &result.AmountCents, &result.OccurredAt, &result.ReceivedAt, &result.ProviderTradeNo, &result.PayerHint, &result.MatchState, &result.UnmatchedReason, &result.TradeNo, &result.RawTextCipher); err != nil {
		return nil, deviceErr("NOT_FOUND", 404)
	}
	dto := map[string]any{
		"id": result.ID, "event_id": result.EventID, "device_id": result.DeviceID,
		"channel": result.Channel, "amount_cents": result.AmountCents,
		"occurred_at": result.OccurredAt, "received_at": result.ReceivedAt,
		"provider_trade_no": result.ProviderTradeNo, "payer_hint": result.PayerHint,
		"match_state": result.MatchState, "unmatched_reason": result.UnmatchedReason,
		"trade_no": result.TradeNo,
	}
	if result.RawTextCipher != nil {
		plain, err := platform.Decrypt(s.key[:], result.RawTextCipher)
		if err != nil {
			return nil, err
		}
		dto["raw_text"] = string(plain)
	}
	return dto, nil
}

func hash(value string) []byte                { digest := sha256.Sum256([]byte(value)); return digest[:] }
func deviceErr(code string, status int) error { return &platform.Error{Code: code, HTTPStatus: status} }
