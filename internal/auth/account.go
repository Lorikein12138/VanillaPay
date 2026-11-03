package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/platform"
)

func (s *Service) Logout(ctx context.Context, role, token string) error {
	if token == "" || (role != "merchant" && role != "admin") {
		return appError("UNAUTHENTICATED", 401)
	}
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET revoked_at=$1 WHERE token_hash=$2 AND role=$3 AND revoked_at IS NULL`, s.clock.Now(), HashOpaque(token), role)
	return err
}

func (s *Service) UpdatePreferences(ctx context.Context, actor platform.Actor, locale, theme string) (map[string]string, error) {
	if actor.SubjectID <= 0 || (actor.Role != "merchant" && actor.Role != "admin") {
		return nil, appError("UNAUTHENTICATED", 401)
	}
	if (locale != "" && locale != "zh-CN" && locale != "en") || (theme != "" && theme != "system" && theme != "light" && theme != "dark") || (locale == "" && theme == "") {
		return nil, appError("VALIDATION_ERROR", 400)
	}
	if actor.Role != "merchant" && actor.Role != "admin" || actor.SubjectID <= 0 {
		return nil, appError("FORBIDDEN", 403)
	}
	var result struct {
		Locale string `json:"locale"`
		Theme  string `json:"theme"`
	}
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if actor.Role == "merchant" {
			if _, err := tx.Exec(ctx, `UPDATE merchants SET preferences=preferences||jsonb_strip_nulls(jsonb_build_object('locale',NULLIF($1,''),'theme',NULLIF($2,''))),updated_at=$3 WHERE id=$4`, locale, theme, s.clock.Now(), actor.SubjectID); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `UPDATE admins SET preferences=preferences||jsonb_strip_nulls(jsonb_build_object('locale',NULLIF($1,''),'theme',NULLIF($2,''))),updated_at=$3 WHERE id=$4`, locale, theme, s.clock.Now(), actor.SubjectID); err != nil {
				return err
			}
		}
		var merchantID *int64
		if actor.MerchantID > 0 {
			merchantID = &actor.MerchantID
		}
		if err := recordAudit(ctx, tx, actor, merchantID, "account.preferences", "account", fmt.Sprint(actor.SubjectID), "success", nil); err != nil {
			return err
		}
		if actor.Role == "merchant" {
			return tx.QueryRow(ctx, `SELECT preferences->>'locale',preferences->>'theme' FROM merchants WHERE id=$1`, actor.SubjectID).Scan(&result.Locale, &result.Theme)
		}
		return tx.QueryRow(ctx, `SELECT preferences->>'locale',preferences->>'theme' FROM admins WHERE id=$1`, actor.SubjectID).Scan(&result.Locale, &result.Theme)
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"locale": result.Locale, "theme": result.Theme}, nil
}

func (s *Service) BannedInfo(ctx context.Context, actor platform.Actor) (map[string]any, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return nil, appError("UNAUTHENTICATED", 401)
	}
	var bannedAt *time.Time
	var reason *string
	if err := s.pool.QueryRow(ctx, `SELECT banned_at,ban_reason FROM merchants WHERE id=$1`, actor.MerchantID).Scan(&bannedAt, &reason); err != nil {
		return nil, appError("NOT_FOUND", 404)
	}
	var security []byte
	if err := s.pool.QueryRow(ctx, `SELECT security FROM site_settings WHERE singleton=true`).Scan(&security); err != nil {
		return nil, err
	}
	contact := map[string]string{"zh_CN": "", "en": ""}
	var value struct {
		BanContact map[string]string `json:"ban_contact"`
	}
	if json.Unmarshal(security, &value) == nil && value.BanContact != nil {
		contact = value.BanContact
	}
	return map[string]any{"banned": bannedAt != nil, "banned_at": bannedAt, "reason": reason, "contact": contact}, nil
}

func (s *Service) ChangeAdminPassword(ctx context.Context, actor platform.Actor, current, password, confirmation, totpCode string) error {
	if actor.Role != "admin" || actor.SubjectID <= 0 {
		return appError("UNAUTHENTICATED", 401)
	}
	if err := ValidatePassword(password, confirmation); err != nil {
		return appError("VALIDATION_ERROR", 400)
	}
	hash, err := HashPassword(password, s.random)
	if err != nil {
		return err
	}
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var account Account
		if err := tx.QueryRow(ctx, `SELECT id,'admin',username,username_normalized,'','', '',password_hash,NULL,0,NULL,NULL,totp_cipher,last_totp_step,preferences,created_at FROM admins WHERE id=$1 FOR UPDATE`, actor.SubjectID).Scan(
			&account.ID, &account.Role, &account.Username, &account.UsernameNorm, &account.Email, &account.EmailNorm, &account.PID, &account.PasswordHash, &account.KeyCipher, &account.KeyVersion, &account.BannedAt, &account.BanReason, &account.TOTPCipher, &account.LastTOTPStep, &account.Preferences, &account.CreatedAt); err != nil {
			return appError("UNAUTHENTICATED", 401)
		}
		if !VerifyPassword(account.PasswordHash, current) {
			return appError("INVALID_CREDENTIALS", 401)
		}
		if len(account.TOTPCipher) > 0 {
			secret, err := platform.Decrypt(s.encryptionKey[:], account.TOTPCipher)
			if err != nil {
				return appError("INTERNAL_ERROR", 500)
			}
			step, ok := verifyTOTP(totpCode, string(secret), s.clock.Now())
			if !ok || step <= account.LastTOTPStep {
				return appError("INVALID_2FA", 401)
			}
			if _, err := tx.Exec(ctx, `UPDATE admins SET last_totp_step=$1 WHERE id=$2`, step, actor.SubjectID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE admins SET password_hash=$1,updated_at=$2 WHERE id=$3`, hash, s.clock.Now(), actor.SubjectID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$1 WHERE role='admin' AND subject_id=$2 AND revoked_at IS NULL`, s.clock.Now(), actor.SubjectID); err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, nil, "auth.admin_password_change", "admin", fmt.Sprint(actor.SubjectID), "success", nil)
	})
}
