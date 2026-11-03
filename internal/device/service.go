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
	"github.com/jackc/pgx/v5/pgxpool"
	sqlc "vanillapay/internal/db/sqlc"
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
		var last *time.Time
		var ever bool
		var appVersion string
		if err := tx.QueryRow(ctx, `SELECT last_seen_at,ever_online,app_version FROM devices WHERE id=$1 AND merchant_id=$2 AND revoked_at IS NULL FOR UPDATE`, device.DeviceID, device.MerchantID).Scan(&last, &ever, &appVersion); err != nil {
			return deviceErr("DEVICE_UNAUTHORIZED", 401)
		}
		now := s.clock.Now()
		if ever && last != nil && !last.After(now.Add(-90*time.Second)) {
			if _, err := tx.Exec(ctx, `UPDATE devices SET offline_episode=offline_episode+1 WHERE id=$1`, device.DeviceID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE devices SET last_seen_at=$1,ever_online=true,app_version=$2,updated_at=$1 WHERE id=$3`, now, strings.TrimSpace(req.AppVersion), device.DeviceID); err != nil {
			return err
		}
		var banned bool
		if err := tx.QueryRow(ctx, `SELECT banned_at IS NOT NULL FROM merchants WHERE id=$1`, device.MerchantID).Scan(&banned); err != nil {
			return err
		}
		result = DeviceStatus{ServerTime: now, Online: true, MerchantBanned: banned, HeartbeatIntervalSeconds: 30}
		return nil
	})
	return result, err
}

func (s *Service) Device(ctx context.Context, actor platform.Actor) (DeviceDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return DeviceDTO{}, deviceErr("UNAUTHENTICATED", 401)
	}
	var publicID, name, version string
	var last *time.Time
	err := s.pool.QueryRow(ctx, `SELECT public_id,name,app_version,last_seen_at FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL`, actor.MerchantID).Scan(&publicID, &name, &version, &last)
	dto := DeviceDTO{HeartbeatIntervalSeconds: 30, OfflineThresholdSeconds: 90}
	if errors.Is(err, pgx.ErrNoRows) {
		return dto, nil
	}
	if err != nil {
		return dto, err
	}
	dto.ID = &publicID
	dto.Name = &name
	dto.AppVersion = &version
	dto.Bound = true
	dto.LastSeenAt = last
	dto.Online = last != nil && last.After(s.clock.Now().Add(-90*time.Second))
	if last != nil {
		off := last.Add(90 * time.Second)
		dto.OfflineAt = &off
	}
	return dto, nil
}

func (s *Service) Revoke(ctx context.Context, actor platform.Actor) error {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return deviceErr("UNAUTHENTICATED", 401)
	}
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `UPDATE devices SET revoked_at=$1,updated_at=$1 WHERE merchant_id=$2 AND revoked_at IS NULL RETURNING public_id`, s.clock.Now(), actor.MerchantID)
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
	rows, err := s.pool.Query(ctx, `SELECT id,merchant_id,offline_episode FROM devices WHERE revoked_at IS NULL AND ever_online=true AND last_seen_at<=$1::timestamptz-interval '90 seconds' AND offline_notified_episode<offline_episode+1 ORDER BY last_seen_at,id LIMIT $2`, s.clock.Now(), limit)
	if err != nil {
		return 0, err
	}
	type candidate struct{ id, merchant, episode int64 }
	var list []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.merchant, &c.episode); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	count := 0
	for _, c := range list {
		err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			if err := sqlc.LockBusinessRows(ctx, tx, []int64{c.merchant}, false); err != nil {
				return err
			}
			var email string
			var episode, notified int64
			if err := tx.QueryRow(ctx, `SELECT d.offline_episode,d.offline_notified_episode,m.email_normalized FROM devices d JOIN merchants m ON m.id=d.merchant_id WHERE d.id=$1 AND d.revoked_at IS NULL AND d.last_seen_at<=$2::timestamptz-interval '90 seconds' FOR UPDATE OF d`, c.id, s.clock.Now()).Scan(&episode, &notified, &email); err != nil {
				return err
			}
			if !needsOfflineNotification(episode, notified) {
				return nil
			}
			next := episode + 1
			if _, err := tx.Exec(ctx, `UPDATE devices SET offline_notified_episode=$1 WHERE id=$2`, next, c.id); err != nil {
				return err
			}
			dedupe := fmt.Sprintf("device_offline:%d:%d", c.id, next)
			id, err := platform.RandomToken(s.random, 18)
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{"title": map[string]string{"zh_CN": "设备离线", "en": "Device offline"}, "summary": map[string]string{"zh_CN": "设备已超过 90 秒未心跳", "en": "The device missed heartbeats for over 90 seconds"}, "action": map[string]string{"page": "/dashboard/collection", "entity_id": "device"}})
			if _, err := tx.Exec(ctx, `INSERT INTO notifications(public_id,merchant_id,kind,source_id,dedupe_key,safe_payload) VALUES($1,$2,'device_offline',$3,$4,$5) ON CONFLICT(dedupe_key) DO NOTHING`, id, c.merchant, fmt.Sprint(c.id), dedupe, payload); err != nil {
				return err
			}
			if err := s.queueMailTx(ctx, tx, email, "device_offline", dedupe, map[string]any{"device_id": c.id, "episode": next}); err != nil {
				return err
			}
			count++
			return nil
		})
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

func needsOfflineNotification(episode, notified int64) bool {
	return notified < episode+1
}

func (s *Service) queueMailTx(ctx context.Context, tx pgx.Tx, recipient, template, dedupe string, payload any) error {
	recipientCipher, err := platform.Encrypt(s.key[:], []byte(recipient), s.random)
	if err != nil {
		return err
	}
	data, _ := json.Marshal(payload)
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
