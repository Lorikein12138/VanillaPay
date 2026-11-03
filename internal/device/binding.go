package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/platform"
)

func (s *Service) NewBinding(ctx context.Context, actor platform.Actor, replace bool) (BindingDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return BindingDTO{}, deviceErr("UNAUTHENTICATED", 401)
	}
	code, err := newBindingCode(s.random)
	if err != nil {
		return BindingDTO{}, err
	}
	id, err := platform.RandomToken(s.random, 18)
	if err != nil {
		return BindingDTO{}, err
	}
	expires := s.clock.Now().Add(5 * time.Minute)
	var result BindingDTO
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		var existing bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL)`, actor.MerchantID).Scan(&existing); err != nil {
			return err
		}
		if existing && !replace {
			return deviceErr("DEVICE_ALREADY_BOUND", 409)
		}
		_, err := tx.Exec(ctx, `INSERT INTO device_bindings(public_id,code_hash,merchant_id,replace_confirmed,expires_at) VALUES($1,$2,$3,$4,$5)`, id, hash(code), actor.MerchantID, replace, expires)
		if err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"replace": replace})
		if err := platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: &actor.MerchantID, Action: "device.binding_issue", TargetType: "device_binding", TargetID: id, Result: "success", SafeMetadata: metadata}); err != nil {
			return err
		}
		qr := "vanillapay://bind?v=1&base=" + url.QueryEscape(s.publicBaseURL) + "&code=" + url.QueryEscape(code)
		result = BindingDTO{BindingID: id, Code: code, QRContent: qr, ExpiresAt: expires}
		return nil
	})
	return result, err
}

func (s *Service) Binding(ctx context.Context, actor platform.Actor, id string) (BindingStatus, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return BindingStatus{}, deviceErr("UNAUTHENTICATED", 401)
	}
	var status BindingStatus
	var merchantID, deviceID *int64
	var consumed *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT merchant_id,device_id,expires_at,consumed_at FROM device_bindings WHERE public_id=$1`, id).Scan(&merchantID, &deviceID, &status.ExpiresAt, &consumed); err != nil || merchantID == nil || *merchantID != actor.MerchantID {
		return status, deviceErr("NOT_FOUND", 404)
	}
	if consumed != nil {
		status.State = "consumed"
		if deviceID != nil {
			device, err := s.Device(ctx, actor)
			if err == nil {
				status.Device = &device
			}
		}
	} else if !status.ExpiresAt.After(s.clock.Now()) {
		status.State = "expired"
	} else {
		status.State = "pending"
	}
	return status, nil
}

func (s *Service) Redeem(ctx context.Context, req RedeemRequest) (CredentialDTO, error) {
	code := normalizeBindingCode(req.Code)
	if len(code) != 12 || len([]rune(req.DeviceName)) < 1 || len([]rune(req.DeviceName)) > 80 || len(req.AppVersion) > 64 {
		return CredentialDTO{}, deviceErr("INVALID_CODE", 401)
	}
	var merchantID, bindingID int64
	if err := s.pool.QueryRow(ctx, `SELECT id,merchant_id FROM device_bindings WHERE code_hash=$1`, hash(code)).Scan(&bindingID, &merchantID); err != nil {
		return CredentialDTO{}, deviceErr("INVALID_CODE", 401)
	}
	deviceToken, err := platform.RandomToken(s.random, 32)
	if err != nil {
		return CredentialDTO{}, err
	}
	devicePublicID, err := platform.RandomToken(s.random, 16)
	if err != nil {
		return CredentialDTO{}, err
	}
	var result CredentialDTO
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		var confirmed bool
		var expires time.Time
		var consumed *time.Time
		if err := tx.QueryRow(ctx, `SELECT replace_confirmed,expires_at,consumed_at FROM device_bindings WHERE id=$1 AND code_hash=$2 FOR UPDATE`, bindingID, hash(code)).Scan(&confirmed, &expires, &consumed); err != nil {
			return deviceErr("INVALID_CODE", 401)
		}
		if consumed != nil {
			return deviceErr("BINDING_USED", 409)
		}
		if !expires.After(s.clock.Now()) {
			return deviceErr("BINDING_EXPIRED", 410)
		}
		var banned bool
		if err := tx.QueryRow(ctx, `SELECT banned_at IS NOT NULL FROM merchants WHERE id=$1 FOR UPDATE`, merchantID).Scan(&banned); err != nil {
			return err
		}
		if banned {
			return deviceErr("ACCOUNT_BANNED", 403)
		}
		var active int64
		var activePublicID string
		err := tx.QueryRow(ctx, `SELECT id,public_id FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL FOR UPDATE`, merchantID).Scan(&active, &activePublicID)
		if err == nil {
			if !confirmed {
				return deviceErr("DEVICE_ALREADY_BOUND", 409)
			}
			if _, err := tx.Exec(ctx, `UPDATE devices SET revoked_at=$1,updated_at=$1 WHERE id=$2`, s.clock.Now(), active); err != nil {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var newID int64
		if err := tx.QueryRow(ctx, `INSERT INTO devices(public_id,merchant_id,token_hash,name,app_version) VALUES($1,$2,$3,$4,$5) RETURNING id`, devicePublicID, merchantID, hash(deviceToken), strings.TrimSpace(req.DeviceName), strings.TrimSpace(req.AppVersion)).Scan(&newID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE device_bindings SET consumed_at=$1,device_name=$2,app_version=$3,device_id=$4 WHERE id=$5 AND consumed_at IS NULL`, s.clock.Now(), strings.TrimSpace(req.DeviceName), strings.TrimSpace(req.AppVersion), newID, bindingID); err != nil {
			return err
		}
		replace := activePublicID != ""
		action := "device.bind"
		metadata, _ := json.Marshal(map[string]any{"replace": replace, "previous_device_id": activePublicID})
		if replace {
			action = "device.replace"
		}
		actor := platform.Actor{Role: "device", SubjectID: newID, MerchantID: merchantID}
		if request, ok := platform.RequestMetadataFromContext(ctx); ok {
			actor.IP = request.IP
			actor.RequestID = request.RequestID
		}
		if err := platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: &merchantID, Action: action, TargetType: "device", TargetID: devicePublicID, Result: "success", SafeMetadata: metadata}); err != nil {
			return err
		}
		result = CredentialDTO{DeviceID: devicePublicID, DeviceToken: deviceToken, MerchantID: fmt.Sprint(merchantID), ServerTime: s.clock.Now(), HeartbeatIntervalSeconds: 30, OfflineThresholdSeconds: 90}
		return nil
	})
	return result, err
}

func newBindingCode(reader io.Reader) (string, error) {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	raw := make([]byte, 12)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, v := range raw {
		b.WriteByte(alphabet[int(v)%len(alphabet)])
	}
	return b.String(), nil
}

func normalizeBindingCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "")
	return value
}
