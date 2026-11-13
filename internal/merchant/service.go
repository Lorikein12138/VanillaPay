package merchant

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"vanillapay/internal/auth"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/platform"
)

type BanPaymentPort interface {
	ExpirePendingForBannedTx(context.Context, pgx.Tx, int64) (int64, error)
}

type SecurityProofPort interface {
	VerifySecurityProofTx(context.Context, pgx.Tx, platform.Actor, int64, string, string, string) error
}

type PlatformKeyPort interface {
	PlatformPublicKeys(context.Context) ([]map[string]string, error)
}

type Service struct {
	pool         *pgxpool.Pool
	clock        platform.Clock
	random       io.Reader
	key          [32]byte
	banPayments  BanPaymentPort
	proofs       SecurityProofPort
	platformKeys PlatformKeyPort
	purchases    interface {
		CancelPendingPurchasesForBannedTx(context.Context, pgx.Tx, int64) error
	}
}

func (s *Service) SetPurchasePort(port interface {
	CancelPendingPurchasesForBannedTx(context.Context, pgx.Tx, int64) error
}) {
	s.purchases = port
}

func (s *Service) SetPlatformKeyPort(port PlatformKeyPort) { s.platformKeys = port }

func NewService(pool *pgxpool.Pool, key [32]byte, clock platform.Clock, random io.Reader, banPayments BanPaymentPort, proofs SecurityProofPort) *Service {
	if clock == nil {
		clock = platform.SystemClock{}
	}
	if random == nil {
		random = rand.Reader
	}
	return &Service{pool: pool, key: key, clock: clock, random: random, banPayments: banPayments, proofs: proofs}
}

func (s *Service) Create(ctx context.Context, actor platform.Actor, req CreateMerchant) (MerchantDTO, error) {
	if actor.Role != "admin" {
		return MerchantDTO{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	username, err := auth.NormalizeUsername(req.Username)
	if err != nil {
		return MerchantDTO{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		return MerchantDTO{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	if err := auth.ValidatePassword(req.Password, req.PasswordConfirmation); err != nil {
		return MerchantDTO{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	passwordHash, err := auth.HashPassword(req.Password, s.random)
	if err != nil {
		return MerchantDTO{}, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		pid, err := auth.GeneratePID(s.random)
		if err != nil {
			return MerchantDTO{}, err
		}
		key, err := auth.GenerateMerchantKey(s.random)
		if err != nil {
			return MerchantDTO{}, err
		}
		cipher, err := platform.Encrypt(s.key[:], []byte(key), s.random)
		if err != nil {
			return MerchantDTO{}, err
		}
		var result MerchantDTO
		err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			var id int64
			var created time.Time
			if err := tx.QueryRow(ctx, `INSERT INTO merchants(username,username_normalized,email,email_normalized,pid,password_hash,key_cipher) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,created_at`, req.Username, username, email, email, pid, passwordHash, cipher).Scan(&id, &created); err != nil {
				return mapConflict(err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO merchant_collection_settings(merchant_id) VALUES($1)`, id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO merchant_ip_rules(merchant_id) VALUES($1)`, id); err != nil {
				return err
			}
			mid := id
			if err := recordAudit(ctx, tx, actor, &mid, "merchant.admin_created", "merchant", fmt.Sprint(id), map[string]any{"source": "admin_created"}); err != nil {
				return err
			}
			result = MerchantDTO{ID: fmt.Sprint(id), Role: "merchant", Username: req.Username, Email: email, PID: pid, Locale: "zh-CN", Theme: "system", CreatedAt: created}
			return nil
		})
		if err == nil {
			return result, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "pid") {
			continue
		}
		return MerchantDTO{}, err
	}
	return MerchantDTO{}, &platform.Error{Code: "VERSION_CONFLICT", HTTPStatus: 409}
}

func (s *Service) Ban(ctx context.Context, actor platform.Actor, merchantID int64, reason string) (BanResult, error) {
	if actor.Role != "admin" || merchantID <= 0 {
		return BanResult{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	if err := validateBanReason(reason); err != nil {
		return BanResult{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	if s.banPayments == nil {
		return BanResult{}, errors.New("payment ban transaction port is not connected")
	}
	if s.purchases == nil {
		return BanResult{}, errors.New("subscription ban transaction port is not connected")
	}
	for attempt := 0; attempt < 3; attempt++ {
		lockIDs := []int64{merchantID}
		billingIDs, err := pendingBillingMerchantIDs(ctx, s.pool, merchantID)
		if err != nil {
			return BanResult{}, err
		}
		lockIDs = append(lockIDs, billingIDs...)
		lockSet := make(map[int64]struct{}, len(lockIDs))
		for _, id := range lockIDs {
			lockSet[id] = struct{}{}
		}
		var result BanResult
		err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			if err := sqlc.LockBusinessRows(ctx, tx, lockIDs, false); err != nil {
				return err
			}
			billingIDs, err := pendingBillingMerchantIDs(ctx, tx, merchantID)
			if err != nil {
				return err
			}
			for _, id := range billingIDs {
				if _, ok := lockSet[id]; !ok {
					return errBanLockSetChanged
				}
			}
			var already *time.Time
			if err := tx.QueryRow(ctx, `SELECT banned_at FROM merchants WHERE id=$1 FOR UPDATE`, merchantID).Scan(&already); err != nil {
				return err
			}
			if already != nil {
				return &platform.Error{Code: "VERSION_CONFLICT", HTTPStatus: 409}
			}
			at := s.clock.Now()
			expired, err := s.banPayments.ExpirePendingForBannedTx(ctx, tx, merchantID)
			if err != nil {
				return err
			}
			if err := s.purchases.CancelPendingPurchasesForBannedTx(ctx, tx, merchantID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE merchants SET banned_at=$1,ban_reason=$2,updated_at=$1 WHERE id=$3`, at, strings.TrimSpace(reason), merchantID); err != nil {
				return err
			}
			if err := recordAudit(ctx, tx, actor, &merchantID, "merchant.ban", "merchant", fmt.Sprint(merchantID), map[string]any{"reason": strings.TrimSpace(reason), "expired_orders": expired}); err != nil {
				return err
			}
			result = BanResult{BannedAt: at.UTC().Format(time.RFC3339Nano), Reason: strings.TrimSpace(reason), ExpiredOrders: expired}
			return nil
		})
		if errors.Is(err, errBanLockSetChanged) {
			continue
		}
		return result, err
	}
	return BanResult{}, &platform.Error{Code: "VERSION_CONFLICT", HTTPStatus: 409}
}

var errBanLockSetChanged = errors.New("pending billing lock set changed")

type rowsQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func pendingBillingMerchantIDs(ctx context.Context, db rowsQuerier, buyerID int64) ([]int64, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT billing_merchant_id FROM purchases WHERE buyer_id=$1 AND state='pending' AND billing_merchant_id IS NOT NULL ORDER BY billing_merchant_id`, buyerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) Unban(ctx context.Context, actor platform.Actor, merchantID int64) (MerchantDTO, error) {
	if actor.Role != "admin" || merchantID <= 0 {
		return MerchantDTO{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	var result MerchantDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE merchants SET banned_at=NULL,ban_reason=NULL,updated_at=$1 WHERE id=$2`, s.clock.Now(), merchantID); err != nil {
			return err
		}
		account, err := (auth.Repository{DB: tx}).ByID(ctx, "merchant", merchantID)
		if err != nil {
			return err
		}
		result = account.DTO()
		return recordAudit(ctx, tx, actor, &merchantID, "merchant.unban", "merchant", fmt.Sprint(merchantID), map[string]any{})
	})
	return result, err
}

func (s *Service) ResetKey(ctx context.Context, actor platform.Actor, proof auth.SecurityProof) (KeyDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return KeyDTO{}, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	if s.proofs == nil {
		return KeyDTO{}, errors.New("security proof port is not connected")
	}
	key, err := auth.GenerateMerchantKey(s.random)
	if err != nil {
		return KeyDTO{}, err
	}
	cipher, err := platform.Encrypt(s.key[:], []byte(key), s.random)
	if err != nil {
		return KeyDTO{}, err
	}
	var result KeyDTO
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		if err := s.proofs.VerifySecurityProofTx(ctx, tx, actor, actor.MerchantID, "reset_key", proof.EmailCode, proof.TOTPCode); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE merchants SET key_cipher=$1,key_version=key_version+1,updated_at=$2 WHERE id=$3 RETURNING pid,key_version`, cipher, s.clock.Now(), actor.MerchantID).Scan(&result.PID, &result.KeyVersion); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE site_settings SET billing=jsonb_set(billing,'{credential_valid}','false'::jsonb,true),version=version+1,updated_at=$1 WHERE singleton=true AND billing->>'merchant_pid'=$2`, s.clock.Now(), result.PID); err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, &actor.MerchantID, "merchant.reset_key", "merchant", fmt.Sprint(actor.MerchantID), map[string]any{"key_version": result.KeyVersion})
	})
	if err != nil {
		return KeyDTO{}, err
	}
	result.Key = key
	return result, nil
}

func (s *Service) SetRSAPublicKey(ctx context.Context, actor platform.Actor, req RSAKeyRequest) (map[string]string, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return nil, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	if s.proofs == nil {
		return nil, errors.New("security proof port is not connected")
	}
	_, fingerprint, err := ParseRSAPublicKey(req.PublicKeyPEM)
	if err != nil {
		return nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		if err := s.proofs.VerifySecurityProofTx(ctx, tx, actor, actor.MerchantID, "reset_key", req.Proof.EmailCode, req.Proof.TOTPCode); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE merchants SET rsa_public_key_pem=$1,rsa_key_updated_at=$2,updated_at=$2 WHERE id=$3`, req.PublicKeyPEM, s.clock.Now(), actor.MerchantID); err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, &actor.MerchantID, "merchant.set_rsa_public_key", "merchant", fmt.Sprint(actor.MerchantID), map[string]any{"fingerprint": fingerprint})
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"fingerprint": fingerprint, "updated_at": s.clock.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func (s *Service) ResetPassword(ctx context.Context, actor platform.Actor, merchantID int64, password, confirmation string) error {
	if actor.Role != "admin" || merchantID <= 0 {
		return &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	if err := auth.ValidatePassword(password, confirmation); err != nil {
		return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	hash, err := auth.HashPassword(password, s.random)
	if err != nil {
		return err
	}
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE merchants SET password_hash=$1,updated_at=$2 WHERE id=$3`, hash, s.clock.Now(), merchantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$1 WHERE role='merchant' AND subject_id=$2 AND revoked_at IS NULL`, s.clock.Now(), merchantID); err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, &merchantID, "merchant.reset_password", "merchant", fmt.Sprint(merchantID), map[string]any{})
	})
}

func (s *Service) SetIPWhitelist(ctx context.Context, actor platform.Actor, merchantID int64, value IPWhitelist) error {
	if actor.Role != "merchant" || actor.MerchantID != merchantID {
		return &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	entries, err := NormalizeWhitelist(value.Entries, value.Enabled)
	if err != nil {
		return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		joined := strings.Join(entries, ",")
		if _, err := tx.Exec(ctx, `INSERT INTO merchant_ip_rules(merchant_id,enabled,entries) VALUES($1,$2,CASE WHEN $3='' THEN ARRAY[]::cidr[] ELSE string_to_array($3,',')::cidr[] END) ON CONFLICT(merchant_id) DO UPDATE SET enabled=EXCLUDED.enabled,entries=EXCLUDED.entries,version=merchant_ip_rules.version+1,updated_at=$4`, merchantID, value.Enabled, joined, s.clock.Now()); err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, &merchantID, "merchant.update_ip_whitelist", "merchant", fmt.Sprint(merchantID), map[string]any{"enabled": value.Enabled, "entry_count": len(entries)})
	})
}

func (s *Service) Integration(ctx context.Context, actor platform.Actor, publicBaseURL string) (map[string]any, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return nil, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	var pid, cipher []byte
	var keyVersion int32
	var publicKey *string
	if err := s.pool.QueryRow(ctx, `SELECT pid,key_cipher,key_version,rsa_public_key_pem FROM merchants WHERE id=$1`, actor.MerchantID).Scan(&pid, &cipher, &keyVersion, &publicKey); err != nil {
		return nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
	}
	plain, err := platform.Decrypt(s.key[:], cipher)
	if err != nil {
		return nil, err
	}
	var enabled bool
	var entries []string
	if err := s.pool.QueryRow(ctx, `SELECT enabled,entries::text[] FROM merchant_ip_rules WHERE merchant_id=$1`, actor.MerchantID).Scan(&enabled, &entries); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if s.platformKeys == nil {
		return nil, errors.New("platform public-key port is not connected")
	}
	platformKeys, err := s.platformKeys.PlatformPublicKeys(ctx)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(publicBaseURL, "/")
	var rsaPublic any
	if publicKey != nil {
		rsaPublic = *publicKey
	}
	return map[string]any{"pid": string(pid), "key": string(plain), "key_version": keyVersion, "base_url": base, "v1": map[string]string{"submit": base + "/submit.php", "mapi": base + "/mapi.php", "query": base + "/api.php?act=order", "refund": base + "/api.php?act=refund", "info": base + "/api.php?act=query", "orders": base + "/api.php?act=orders"}, "v2": map[string]string{"submit": base + "/api/pay/submit", "create": base + "/api/pay/create", "query": base + "/api/pay/query", "refund": base + "/api/pay/refund", "close": base + "/api/pay/close", "refundquery": base + "/api/pay/refundquery", "info": base + "/api/merchant/info", "orders": base + "/api/merchant/orders"}, "merchant_public_key": rsaPublic, "platform_public_keys": platformKeys, "ip_whitelist": IPWhitelist{Enabled: enabled, Entries: entries}}, nil
}

func (s *Service) UpdatePreferences(ctx context.Context, actor platform.Actor, locale, theme string) (map[string]string, error) {
	if actor.SubjectID <= 0 || actor.Role != "merchant" && actor.Role != "admin" {
		return nil, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	if locale != "" && locale != "zh-CN" && locale != "en" || theme != "" && theme != "system" && theme != "light" && theme != "dark" {
		return nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	var result struct {
		Locale string `json:"locale"`
		Theme  string `json:"theme"`
	}
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		table := "merchants"
		if actor.Role == "admin" {
			table = "admins"
		}
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET preferences=preferences||jsonb_strip_nulls(jsonb_build_object('locale',NULLIF($1,''),'theme',NULLIF($2,''))),updated_at=$3 WHERE id=$4`, locale, theme, s.clock.Now(), actor.SubjectID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT preferences->>'locale',preferences->>'theme' FROM `+table+` WHERE id=$1`, actor.SubjectID).Scan(&result.Locale, &result.Theme)
	})
	if err != nil {
		return nil, err
	}
	return map[string]string{"locale": result.Locale, "theme": result.Theme}, nil
}

func recordAudit(ctx context.Context, tx pgx.Tx, actor platform.Actor, merchantID *int64, action, targetType, targetID string, metadata map[string]any) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	return platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: merchantID, Action: action, TargetType: targetType, TargetID: targetID, Result: "success", SafeMetadata: data})
}

func mapConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if strings.Contains(pgErr.ConstraintName, "username") {
			return &platform.Error{Code: "USERNAME_TAKEN", HTTPStatus: 409}
		}
		if strings.Contains(pgErr.ConstraintName, "email") {
			return &platform.Error{Code: "EMAIL_TAKEN", HTTPStatus: 409}
		}
	}
	return err
}
