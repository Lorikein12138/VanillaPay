package settings

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"vanillapay/internal/auth"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/mailtmpl"
	"vanillapay/internal/platform"
)

type Service struct {
	pool          *pgxpool.Pool
	key           [32]byte
	clock         platform.Clock
	random        io.Reader
	publicBaseURL string
	dataDir       string
	turnstile     auth.TurnstileVerifier
	icons         siteIcons
}

func NewService(pool *pgxpool.Pool, key [32]byte, clock platform.Clock, random io.Reader, publicBaseURL, dataDir string, turnstile auth.TurnstileVerifier) *Service {
	if clock == nil {
		clock = platform.SystemClock{}
	}
	if random == nil {
		random = rand.Reader
	}
	return &Service{pool: pool, key: key, clock: clock, random: random, publicBaseURL: publicBaseURL, dataDir: dataDir, turnstile: turnstile}
}

func (s *Service) GetGroup(ctx context.Context, actor platform.Actor) (SettingsRow, error) {
	if actor.Role != "admin" {
		return SettingsRow{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return SettingsRow{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := (Repository{DB: tx}).Get(ctx)
	if err != nil {
		return SettingsRow{}, err
	}
	result := row.Groups
	for name, secretField := range map[string]string{"smtp_password": "password", "turnstile_secret": "turnstile_secret", "billing_key": "merchant_key"} {
		var encrypted []byte
		err := tx.QueryRow(ctx, `SELECT encrypted_value FROM secret_settings WHERE name=$1`, name).Scan(&encrypted)
		configured := err == nil && len(encrypted) > 0
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return SettingsRow{}, err
		}
		group := secretGroup(name)
		object := map[string]any{}
		if err := json.Unmarshal(result[group], &object); err != nil {
			return SettingsRow{}, err
		}
		delete(object, secretField)
		switch name {
		case "smtp_password":
			object["password_configured"] = configured
			if configured {
				object["masked_password"] = "••••••••"
			} else {
				object["masked_password"] = ""
			}
		case "turnstile_secret":
			object["secret_configured"] = configured
		case "billing_key":
			object["key_configured"] = configured
		}
		if name == "billing_key" {
			var billing struct {
				PID string `json:"merchant_pid"`
			}
			_ = json.Unmarshal(result[group], &billing)
			var merchantID *int64
			var id int64
			if billing.PID != "" {
				if err := tx.QueryRow(ctx, `SELECT id FROM merchants WHERE pid=$1`, billing.PID).Scan(&id); err == nil {
					merchantID = &id
				}
			}
			object["merchant_id"] = merchantID
			var valid bool
			if configured && merchantID != nil {
				var cipher []byte
				if tx.QueryRow(ctx, `SELECT key_cipher FROM merchants WHERE id=$1 AND banned_at IS NULL`, *merchantID).Scan(&cipher) == nil {
					plain, decErr := platform.Decrypt(s.key[:], cipher)
					secret, secErr := platform.Decrypt(s.key[:], encrypted)
					valid = secErr == nil && decErr == nil && constantEqual(string(secret), string(plain))
				}
			}
			object["credential_valid"] = valid
		}
		encoded, _ := json.Marshal(object)
		result[group] = encoded
	}
	if err := tx.Commit(ctx); err != nil {
		return SettingsRow{}, err
	}
	return row, nil
}

func secretGroup(name string) string {
	switch name {
	case "smtp_password":
		return "mail"
	case "turnstile_secret":
		return "security"
	default:
		return "billing"
	}
}

func (s *Service) UpdateGroup(ctx context.Context, actor platform.Actor, group string, req UpdateGroup) (GroupResult, error) {
	prepared, err := prepareUpdateGroup(actor, group, req)
	if err != nil {
		return GroupResult{}, err
	}
	return s.updatePreparedGroup(ctx, actor, group, req.ExpectedVersion, prepared)
}

type preparedGroupUpdate struct {
	value          json.RawMessage
	secrets        map[string]json.RawMessage
	recovery       *RecoverySecurityRequest
	recoverySecret json.RawMessage
	turnstileToken string
}

func prepareUpdateGroup(actor platform.Actor, group string, req UpdateGroup) (preparedGroupUpdate, error) {
	if actor.RecoveryOnly {
		if group != "security" {
			return preparedGroupUpdate{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
		}
		return prepareRecoveryUpdate(actor, req)
	}
	if actor.Role != "admin" {
		return preparedGroupUpdate{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	if req.ExpectedVersion < 1 {
		return preparedGroupUpdate{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	clean, secrets, err := ValidateGroup(group, req.Value)
	if err != nil {
		var validation *platform.Error
		if errors.As(err, &validation) {
			return preparedGroupUpdate{}, validation
		}
		return preparedGroupUpdate{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	var token string
	if group == "security" {
		_ = json.Unmarshal(groupField(req.Value, "turnstile_token"), &token) // Already validated by ValidateGroup.
	}
	return preparedGroupUpdate{value: clean, secrets: secrets, turnstileToken: token}, nil
}

func (s *Service) updatePreparedGroup(ctx context.Context, actor platform.Actor, group string, expectedVersion int64, prepared preparedGroupUpdate) (GroupResult, error) {
	if prepared.recovery != nil {
		return s.updateRecoverySecurity(ctx, actor, expectedVersion, *prepared.recovery, prepared.recoverySecret)
	}
	clean, secrets := prepared.value, prepared.secrets
	if group == "security" {
		if err := s.verifySecurityUpdate(ctx, expectedVersion, clean, secrets["turnstile_secret"], prepared.turnstileToken); err != nil {
			return GroupResult{}, err
		}
	}
	ids, err := s.merchantIDs(ctx)
	if err != nil {
		return GroupResult{}, err
	}
	var result GroupResult
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, ids, true); err != nil {
			return err
		}
		var current int64
		if err := tx.QueryRow(ctx, `SELECT version FROM site_settings WHERE singleton=true FOR UPDATE`).Scan(&current); err != nil {
			return err
		}
		if current != expectedVersion {
			return &platform.Error{Code: "VERSION_CONFLICT", HTTPStatus: 409}
		}
		if group == "site" {
			if err := validateFaviconAsset(ctx, tx, clean); err != nil {
				return err
			}
		}
		if group == "billing" {
			if err := validateBillingValueTx(ctx, tx, s.key, clean, secrets); err != nil {
				return err
			}
		}
		var risk platform.PaymentRisk
		if group == "risk" {
			if err := json.Unmarshal(clean, &risk); err != nil {
				return err
			}
			if risk.CheckoutTurnstileEnabled {
				if err := validateCheckoutTurnstile(ctx, tx); err != nil {
					return err
				}
			}
		}
		var affected int64
		var newVersion int64
		if err := updateSettingsGroupTx(ctx, tx, group, clean, s.clock.Now(), current, &newVersion); err != nil {
			return err
		}
		if group == "payment" {
			var err error
			affected, err = s.reconcileCollectionSettings(ctx, tx, actor, clean)
			if err != nil {
				return err
			}
		}
		for name, value := range secrets {
			if string(value) == "null" {
				if _, err := tx.Exec(ctx, `DELETE FROM secret_settings WHERE name=$1`, name); err != nil {
					return err
				}
				continue
			}
			var plain string
			if err := json.Unmarshal(value, &plain); err != nil {
				return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
			}
			cipher, err := platform.Encrypt(s.key[:], []byte(plain), s.random)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO secret_settings(name,encrypted_value,version,updated_at) VALUES($1,$2,1,$3) ON CONFLICT(name) DO UPDATE SET encrypted_value=EXCLUDED.encrypted_value,version=secret_settings.version+1,updated_at=EXCLUDED.updated_at`, name, cipher, s.clock.Now()); err != nil {
				return err
			}
		}
		if group == "footer_legal" {
			if err := saveLegalRevisions(ctx, tx, clean, s.clock.Now()); err != nil {
				return err
			}
		}
		if group == "billing" {
			if _, err := tx.Exec(ctx, `UPDATE site_settings SET billing=jsonb_set(billing,'{credential_valid}','true'::jsonb,true) WHERE singleton=true`); err != nil {
				return err
			}
		}
		metadata := map[string]any{"affected_merchants": affected}
		if group != "payment" {
			metadata = nil
		}
		if group == "risk" {
			metadata = map[string]any{"checkout_turnstile_enabled": risk.CheckoutTurnstileEnabled, "ip_daily_limit_enabled": risk.IPDailyOrderLimit > 0,
				"ip_blacklist_count": len(risk.IPBlacklist), "name_block_words_count": len(risk.NameBlockWords)}
		}
		if err := settingsAudit(ctx, tx, actor, group, newVersion, metadata); err != nil {
			return err
		}
		masked := maskSecrets(clean, group, secrets)
		result = GroupResult{Version: newVersion, Group: group, Value: masked, AffectedMerchants: affected}
		return nil
	})
	return result, err
}

func updateSettingsGroupTx(ctx context.Context, tx pgx.Tx, group string, clean json.RawMessage, now time.Time, version int64, updated *int64) error {
	var query string
	switch group {
	case "site":
		query = `UPDATE site_settings SET site=$1::jsonb,version=version+1,updated_at=$2 WHERE singleton=true AND version=$3 RETURNING version`
	case "home":
		query = `UPDATE site_settings SET home=$1::jsonb,version=version+1,updated_at=$2 WHERE singleton=true AND version=$3 RETURNING version`
	case "footer_legal":
		query = `UPDATE site_settings SET footer_legal=$1::jsonb,version=version+1,updated_at=$2 WHERE singleton=true AND version=$3 RETURNING version`
	case "mail":
		query = `UPDATE site_settings SET mail=$1::jsonb,version=version+1,updated_at=$2 WHERE singleton=true AND version=$3 RETURNING version`
	case "security":
		query = `UPDATE site_settings SET security=$1::jsonb || jsonb_strip_nulls(jsonb_build_object('allowed_hostname',security->'allowed_hostname')),version=version+1,updated_at=$2 WHERE singleton=true AND version=$3 RETURNING version`
	case "payment":
		query = `UPDATE site_settings SET payment=$1::jsonb,version=version+1,updated_at=$2 WHERE singleton=true AND version=$3 RETURNING version`
	case "risk":
		query = `UPDATE site_settings SET risk=$1::jsonb,version=version+1,updated_at=$2 WHERE singleton=true AND version=$3 RETURNING version`
	case "billing":
		query = `UPDATE site_settings SET billing=$1::jsonb,version=version+1,updated_at=$2 WHERE singleton=true AND version=$3 RETURNING version`
	default:
		return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	return tx.QueryRow(ctx, query, clean, now, version).Scan(updated)
}

func prepareRecoveryUpdate(actor platform.Actor, req UpdateGroup) (preparedGroupUpdate, error) {
	if actor.Role != "admin" || actor.SubjectID != 1 || actor.RecoveryChallengeID <= 0 || req.ExpectedVersion < 1 || len(req.Value) == 0 || len(req.Value) > 16<<10 {
		return preparedGroupUpdate{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	var input RecoverySecurityRequest
	if err := decodeGroup(req.Value, &input); err != nil {
		return preparedGroupUpdate{}, err
	}
	if input.Enabled && strings.TrimSpace(input.SiteKey) == "" {
		return preparedGroupUpdate{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	secret := groupField(req.Value, "turnstile_secret")
	if secret != nil && strings.TrimSpace(input.Secret) == "" {
		return preparedGroupUpdate{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	return preparedGroupUpdate{recovery: &input, recoverySecret: secret}, nil
}

func (s *Service) updateRecoverySecurity(ctx context.Context, actor platform.Actor, expectedVersion int64, input RecoverySecurityRequest, secret json.RawMessage) (GroupResult, error) {
	replacesSecret := secret != nil
	ids, err := s.merchantIDs(ctx)
	if err != nil {
		return GroupResult{}, err
	}
	var result GroupResult
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, ids, true); err != nil {
			return err
		}
		var current int64
		var currentSecurity json.RawMessage
		if err := tx.QueryRow(ctx, `SELECT version,security - 'allowed_hostname' FROM site_settings WHERE singleton=true FOR UPDATE`).Scan(&current, &currentSecurity); err != nil {
			return err
		}
		if current != expectedVersion {
			return &platform.Error{Code: "VERSION_CONFLICT", HTTPStatus: 409}
		}
		var merged map[string]json.RawMessage
		if err := json.Unmarshal(currentSecurity, &merged); err != nil || merged == nil {
			return errors.New("stored security settings are invalid")
		}
		merged["turnstile_site_key"] = marshal(input.SiteKey)
		merged["turnstile_enabled"] = marshal(input.Enabled)
		if replacesSecret {
			merged["turnstile_secret"] = append(json.RawMessage(nil), secret...)
		}
		encoded, err := json.Marshal(merged)
		if err != nil {
			return err
		}
		clean, secrets, err := ValidateGroup("security", encoded)
		if err != nil {
			return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
		}
		if input.Enabled && !replacesSecret {
			var configured bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM secret_settings WHERE name='turnstile_secret' AND octet_length(encrypted_value)>0)`).Scan(&configured); err != nil {
				return err
			}
			if !configured {
				return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
			}
		}
		var nextVersion int64
		if err := tx.QueryRow(ctx, `UPDATE site_settings SET security=$1::jsonb || jsonb_strip_nulls(jsonb_build_object('allowed_hostname',security->'allowed_hostname')),version=version+1,updated_at=$2 WHERE singleton=true AND version=$3 RETURNING version`, clean, s.clock.Now(), current).Scan(&nextVersion); err != nil {
			return err
		}
		if raw, ok := secrets["turnstile_secret"]; ok {
			var plain string
			if json.Unmarshal(raw, &plain) != nil || plain == "" {
				return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
			}
			cipher, err := platform.Encrypt(s.key[:], []byte(plain), s.random)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO secret_settings(name,encrypted_value,version,updated_at) VALUES('turnstile_secret',$1,1,$2) ON CONFLICT(name) DO UPDATE SET encrypted_value=EXCLUDED.encrypted_value,version=secret_settings.version+1,updated_at=EXCLUDED.updated_at`, cipher, s.clock.Now()); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE auth_challenges SET consumed_at=$1,secret_cipher=NULL WHERE id=$2 AND role='admin' AND subject_id=1 AND purpose='recovery' AND consumed_at IS NULL AND expires_at>$1`, s.clock.Now(), actor.RecoveryChallengeID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
		}
		if err := settingsAudit(ctx, tx, actor, "security", nextVersion, map[string]any{"scope": "recovery_security"}); err != nil {
			return err
		}
		result = GroupResult{Version: nextVersion, Group: "security", Value: maskSecrets(clean, "security", secrets)}
		return nil
	})
	return result, err
}

func validateBillingValueTx(ctx context.Context, tx pgx.Tx, key [32]byte, clean json.RawMessage, secrets map[string]json.RawMessage) error {
	var value struct {
		MerchantPID string `json:"merchant_pid"`
	}
	if err := json.Unmarshal(clean, &value); err != nil {
		return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	if value.MerchantPID == "" {
		if raw, ok := secrets["billing_key"]; ok && string(raw) != "null" {
			return &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
		}
		return nil
	}
	var provided string
	if raw, ok := secrets["billing_key"]; ok {
		if string(raw) == "null" {
			return &platform.Error{Code: "BILLING_CREDENTIAL_STALE", HTTPStatus: 409}
		}
		_ = json.Unmarshal(raw, &provided)
	} else {
		cipher, err := secretCipherTx(ctx, tx, "billing_key")
		if err != nil {
			return &platform.Error{Code: "BILLING_CREDENTIAL_STALE", HTTPStatus: 409}
		}
		plain, err := platform.Decrypt(key[:], cipher)
		if err != nil {
			return &platform.Error{Code: "BILLING_CREDENTIAL_STALE", HTTPStatus: 409}
		}
		provided = string(plain)
	}
	var id int64
	var cipher []byte
	if err := tx.QueryRow(ctx, `SELECT id,key_cipher FROM merchants WHERE pid=$1 AND banned_at IS NULL FOR UPDATE`, value.MerchantPID).Scan(&id, &cipher); err != nil {
		return &platform.Error{Code: "BILLING_CREDENTIAL_STALE", HTTPStatus: 409}
	}
	plain, err := platform.Decrypt(key[:], cipher)
	if err != nil || len(provided) != 64 || !constantEqual(provided, string(plain)) {
		return &platform.Error{Code: "BILLING_CREDENTIAL_STALE", HTTPStatus: 409}
	}
	return nil
}

func secretCipherTx(ctx context.Context, tx pgx.Tx, name string) ([]byte, error) {
	var cipher []byte
	err := tx.QueryRow(ctx, `SELECT encrypted_value FROM secret_settings WHERE name=$1`, name).Scan(&cipher)
	return cipher, err
}

func saveLegalRevisions(ctx context.Context, tx pgx.Tx, clean json.RawMessage, now time.Time) error {
	var value FooterLegal
	if err := json.Unmarshal(clean, &value); err != nil {
		return err
	}
	for _, item := range []struct {
		kind    string
		content LanguageText
	}{{"terms", value.Terms}, {"privacy", value.Privacy}} {
		var previous []byte
		err := tx.QueryRow(ctx, `SELECT content FROM legal_versions WHERE kind=$1 ORDER BY revision DESC LIMIT 1 FOR UPDATE`, item.kind).Scan(&previous)
		if err == nil {
			var content LanguageText
			if err := json.Unmarshal(previous, &content); err != nil {
				return err
			}
			if content == item.content {
				continue
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		encoded, _ := json.Marshal(item.content)
		if _, err := tx.Exec(ctx, `INSERT INTO legal_versions(kind,revision,content,published_at) VALUES($1,(SELECT COALESCE(max(revision),0)+1 FROM legal_versions WHERE kind=$1),$2,$3)`, item.kind, encoded, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) merchantIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM merchants ORDER BY id`)
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

func (s *Service) reconcileCollectionSettings(ctx context.Context, tx pgx.Tx, actor platform.Actor, clean json.RawMessage) (int64, error) {
	var global GroupBounds
	if err := json.Unmarshal(clean, &global); err != nil {
		return 0, err
	}
	resetFields := make(map[int64]map[string]bool)
	versions := make(map[int64]int64)
	rows, err := tx.Query(ctx, `UPDATE merchant_collection_settings SET order_ttl_seconds=NULL,version=version+1,updated_at=$1 WHERE order_ttl_seconds>$2 RETURNING merchant_id,version`, s.clock.Now(), global.MaxTTLSeconds)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var merchantID, version int64
		if err := rows.Scan(&merchantID, &version); err != nil {
			rows.Close()
			return 0, err
		}
		if resetFields[merchantID] == nil {
			resetFields[merchantID] = make(map[string]bool)
		}
		resetFields[merchantID]["order_ttl_seconds"] = true
		versions[merchantID] = version
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	rows, err = tx.Query(ctx, `UPDATE merchant_collection_settings SET float_limit_cents=NULL,version=version+1,updated_at=$1 WHERE float_limit_cents<$2 OR float_limit_cents>$3 RETURNING merchant_id,version`, s.clock.Now(), global.FloatMinCents, global.FloatMaxCents)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var merchantID, version int64
		if err := rows.Scan(&merchantID, &version); err != nil {
			rows.Close()
			return 0, err
		}
		if resetFields[merchantID] == nil {
			resetFields[merchantID] = make(map[string]bool)
		}
		resetFields[merchantID]["float_limit_cents"] = true
		versions[merchantID] = version
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for merchantID, fieldSet := range resetFields {
		fields := make([]string, 0, len(fieldSet))
		for field := range fieldSet {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		mid := merchantID
		metadata, _ := json.Marshal(map[string]any{"fields": fields, "version": versions[merchantID]})
		if err := platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: &mid, Action: "collection.bounds_reset", TargetType: "merchant_collection_settings", TargetID: strconv.FormatInt(merchantID, 10), Result: "success", SafeMetadata: metadata}); err != nil {
			return 0, err
		}
	}
	return int64(len(resetFields)), nil
}

func (s *Service) SaveCollection(ctx context.Context, actor platform.Actor, req SaveCollectionRequest) (CollectionSettings, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return CollectionSettings{}, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	if req.ExpectedVersion < 0 {
		return CollectionSettings{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	if req.OrderTTLSeconds != nil && (*req.OrderTTLSeconds < 60 || *req.OrderTTLSeconds > 86400) || req.FloatLimitCents != nil && (*req.FloatLimitCents < 0 || *req.FloatLimitCents > 999) || req.FloatDirection != nil && *req.FloatDirection != "up" && *req.FloatDirection != "down" {
		return CollectionSettings{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	var result CollectionSettings
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		// GetCollection exposes version zero until the first settings row exists.
		var currentVersion int64
		err := tx.QueryRow(ctx, `SELECT version FROM merchant_collection_settings WHERE merchant_id=$1`, actor.MerchantID).Scan(&currentVersion)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if currentVersion != req.ExpectedVersion {
			return &platform.Error{Code: "VERSION_CONFLICT", HTTPStatus: 409}
		}
		var global GroupBounds
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT payment FROM site_settings WHERE singleton=true`).Scan(&raw); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &global); err != nil {
			return err
		}
		if req.OrderTTLSeconds != nil && (*req.OrderTTLSeconds < 60 || *req.OrderTTLSeconds > global.MaxTTLSeconds) {
			return &platform.Error{Code: "TTL_OUT_OF_RANGE", HTTPStatus: 400}
		}
		if req.FloatLimitCents != nil && (*req.FloatLimitCents < global.FloatMinCents || *req.FloatLimitCents > global.FloatMaxCents) {
			return &platform.Error{Code: "FLOAT_OUT_OF_RANGE", HTTPStatus: 400}
		}
		var row struct {
			TTL       *int
			Limit     *int
			Direction *string
			Version   int64
		}
		if err := tx.QueryRow(ctx, `INSERT INTO merchant_collection_settings(merchant_id,order_ttl_seconds,float_limit_cents,float_direction) VALUES($1,$2,$3,$4) ON CONFLICT(merchant_id) DO UPDATE SET order_ttl_seconds=EXCLUDED.order_ttl_seconds,float_limit_cents=EXCLUDED.float_limit_cents,float_direction=EXCLUDED.float_direction,version=merchant_collection_settings.version+1,updated_at=$5 RETURNING order_ttl_seconds,float_limit_cents,float_direction,version`, actor.MerchantID, req.OrderTTLSeconds, req.FloatLimitCents, req.FloatDirection, s.clock.Now()).Scan(&row.TTL, &row.Limit, &row.Direction, &row.Version); err != nil {
			return err
		}
		result.OrderTTLSeconds = row.TTL
		result.FloatLimitCents = row.Limit
		result.FloatDirection = row.Direction
		result.Version = row.Version
		ttl := global.DefaultTTLSeconds
		if row.TTL != nil {
			ttl = *row.TTL
		}
		limit := global.FloatDefaultCents
		if row.Limit != nil {
			limit = *row.Limit
		}
		direction := global.FloatDirection
		if row.Direction != nil {
			direction = *row.Direction
		}
		result.Effective.OrderTTLSeconds = ttl
		result.Effective.FloatLimitCents = limit
		result.Effective.FloatDirection = direction
		result.Bounds.TTLMaxSeconds = global.MaxTTLSeconds
		result.Bounds.FloatMinCents = global.FloatMinCents
		result.Bounds.FloatMaxCents = global.FloatMaxCents
		merchantID := actor.MerchantID
		metadata, _ := json.Marshal(map[string]any{"version": row.Version})
		return platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: &merchantID, Action: "collection.save", TargetType: "merchant_collection_settings", TargetID: strconv.FormatInt(merchantID, 10), Result: "success", SafeMetadata: metadata})
	})
	return result, err
}

func (s *Service) GetCollection(ctx context.Context, actor platform.Actor) (CollectionSettings, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return CollectionSettings{}, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	var global GroupBounds
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT payment FROM site_settings WHERE singleton=true`).Scan(&raw); err != nil {
		return CollectionSettings{}, err
	}
	if err := json.Unmarshal(raw, &global); err != nil {
		return CollectionSettings{}, err
	}
	var result CollectionSettings
	err := s.pool.QueryRow(ctx, `SELECT order_ttl_seconds,float_limit_cents,float_direction,version FROM merchant_collection_settings WHERE merchant_id=$1`, actor.MerchantID).Scan(&result.OrderTTLSeconds, &result.FloatLimitCents, &result.FloatDirection, &result.Version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return CollectionSettings{}, err
	}
	ttl := global.DefaultTTLSeconds
	if result.OrderTTLSeconds != nil {
		ttl = *result.OrderTTLSeconds
	}
	floatLimit := global.FloatDefaultCents
	if result.FloatLimitCents != nil {
		floatLimit = *result.FloatLimitCents
	}
	direction := global.FloatDirection
	if result.FloatDirection != nil {
		direction = *result.FloatDirection
	}
	result.Effective.OrderTTLSeconds = ttl
	result.Effective.FloatLimitCents = floatLimit
	result.Effective.FloatDirection = direction
	result.Bounds.TTLMaxSeconds = global.MaxTTLSeconds
	result.Bounds.FloatMinCents = global.FloatMinCents
	result.Bounds.FloatMaxCents = global.FloatMaxCents
	return result, nil
}

func (s *Service) GetPublic(ctx context.Context) (map[string]any, error) {
	var site SiteGroupRequest
	var home, footer, security map[string]any
	var configured bool
	err := s.pool.QueryRow(ctx, `SELECT site,home,footer_legal,security,
		EXISTS(SELECT 1 FROM secret_settings WHERE name='turnstile_secret' AND octet_length(encrypted_value)>0)
		FROM site_settings WHERE singleton`).Scan(&site, &home, &footer, &security, &configured)
	if err != nil {
		return nil, err
	}
	siteKey, _ := security["turnstile_site_key"].(string)
	legal := map[string]int64{}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON(kind) kind,revision FROM legal_versions ORDER BY kind,revision DESC`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind string
		var revision int64
		if err := rows.Scan(&kind, &revision); err != nil {
			rows.Close()
			return nil, err
		}
		legal[kind] = revision
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	logo := ""
	if site.LogoAssetID != nil && *site.LogoAssetID != "" {
		logo = "/media/" + url.PathEscape(*site.LogoAssetID)
	}
	footerItems, _ := footer["items"].([]any)
	visible := make([]any, 0, len(footerItems))
	for _, item := range footerItems {
		if m, ok := item.(map[string]any); ok && m["enabled"] == true {
			visible = append(visible, m)
		}
	}
	sort.SliceStable(visible, func(i, j int) bool {
		a, _ := visible[i].(map[string]any)
		b, _ := visible[j].(map[string]any)
		ai, _ := a["sort"].(float64)
		bi, _ := b["sort"].(float64)
		return ai < bi
	})
	return map[string]any{"site_name": site.SiteName, "logo_url": logo, "favicon_url": faviconURL(site), "turnstile": map[string]any{"site_key": siteKey, "enabled": security["turnstile_enabled"] == true && strings.TrimSpace(siteKey) != "" && configured, "login_action": "login", "register_action": "register"}, "registration_open": true, "email_domain_restriction": map[string]any{"enabled": security["restrict_email_domains"], "domains": security["email_domains"]}, "home": home, "footer": visible, "legal_revisions": map[string]any{"terms": legal["terms"], "privacy": legal["privacy"]}}, nil
}

func (s *Service) GetLegal(ctx context.Context, kind string) (map[string]any, error) {
	if kind != "terms" && kind != "privacy" {
		return nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
	}
	var revision int64
	var content []byte
	var published time.Time
	if err := s.pool.QueryRow(ctx, `SELECT revision,content,published_at FROM legal_versions WHERE kind=$1 ORDER BY revision DESC LIMIT 1`, kind).Scan(&revision, &content, &published); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
		}
		return nil, err
	}
	var localized LanguageText
	if err := json.Unmarshal(content, &localized); err != nil {
		return nil, err
	}
	return map[string]any{"kind": kind, "revision": revision, "content": localized, "updated_at": published.UTC().Format(time.RFC3339Nano)}, nil
}

func (s *Service) QRSlots(ctx context.Context, actor platform.Actor) ([]QRDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return nil, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	rows, err := s.pool.Query(ctx, `SELECT channel,id,active,created_at FROM media_assets WHERE owner_merchant_id=$1 AND kind='qr' ORDER BY created_at DESC`, actor.MerchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	latest := map[string]QRDTO{}
	for rows.Next() {
		var channel, id string
		var active bool
		var updated time.Time
		if err := rows.Scan(&channel, &id, &active, &updated); err != nil {
			return nil, err
		}
		if _, ok := latest[channel]; !ok {
			asset := id
			when := updated.UTC().Format(time.RFC3339Nano)
			url := "/media/" + id
			latest[channel] = QRDTO{Channel: channel, AssetID: &asset, ImageURL: &url, Enabled: active, UpdatedAt: &when}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]QRDTO, 0, 3)
	for _, channel := range platform.PaymentChannels() {
		if value, ok := latest[channel]; ok {
			out = append(out, value)
		} else {
			out = append(out, QRDTO{Channel: channel, Enabled: false})
		}
	}
	return out, nil
}

func (s *Service) SetQREnabled(ctx context.Context, actor platform.Actor, channel string, enabled bool) (QRDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return QRDTO{}, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	if !platform.ValidPaymentChannel(channel) {
		return QRDTO{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	var out QRDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		var id string
		var updated time.Time
		err := tx.QueryRow(ctx, `SELECT id,created_at FROM media_assets WHERE owner_merchant_id=$1 AND kind='qr' AND channel=$2 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, actor.MerchantID, channel).Scan(&id, &updated)
		if err != nil {
			if !enabled && errors.Is(err, pgx.ErrNoRows) {
				out = QRDTO{Channel: channel, Enabled: false}
				return nil
			}
			return &platform.Error{Code: "QR_UNAVAILABLE", HTTPStatus: 422}
		}
		if enabled {
			if _, err := tx.Exec(ctx, `UPDATE media_assets SET active=false WHERE owner_merchant_id=$1 AND kind='qr' AND channel=$2 AND id<>$3 AND active=true`, actor.MerchantID, channel, id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE media_assets SET active=$1 WHERE id=$2`, enabled, id); err != nil {
			return err
		}
		url := "/media/" + id
		out = QRDTO{Channel: channel, AssetID: &id, ImageURL: &url, Enabled: enabled}
		stamp := updated.UTC().Format(time.RFC3339Nano)
		out.UpdatedAt = &stamp
		return nil
	})
	return out, err
}

func (s *Service) Secret(ctx context.Context, name string) (string, error) {
	var cipher []byte
	if err := s.pool.QueryRow(ctx, `SELECT encrypted_value FROM secret_settings WHERE name=$1`, name).Scan(&cipher); err != nil {
		return "", err
	}
	plain, err := platform.Decrypt(s.key[:], cipher)
	return string(plain), err
}

func maskSecrets(value json.RawMessage, group string, secrets map[string]json.RawMessage) json.RawMessage {
	var object map[string]any
	_ = json.Unmarshal(value, &object)
	for name, raw := range secrets {
		configured := string(raw) != "null" && len(raw) > 0
		switch name {
		case "smtp_password":
			object["password_configured"] = configured
			if configured {
				object["masked_password"] = "••••••••"
			} else {
				object["masked_password"] = ""
			}
		case "billing_key":
			object["key_configured"] = configured
		default:
			object["secret_configured"] = configured
		}
	}
	if group == "mail" && object["password_configured"] == nil {
		object["password_configured"] = false
		object["masked_password"] = ""
	}
	b, _ := json.Marshal(object)
	return b
}

func settingsAudit(ctx context.Context, tx pgx.Tx, actor platform.Actor, group string, version int64, details map[string]any) error {
	metadata := make(map[string]any)
	if _, repairScope := details["scope"]; repairScope {
		for key, value := range details {
			metadata[key] = value
		}
	} else {
		metadata["group"] = group
		metadata["version"] = version
		for key, value := range details {
			metadata[key] = value
		}
	}
	raw, _ := json.Marshal(metadata)
	return platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, Action: "settings.update_group", TargetType: "settings", TargetID: group, Result: "success", SafeMetadata: raw})
}
func constantEqual(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func (s *Service) QueueMailTest(ctx context.Context, actor platform.Actor, recipient string) (map[string]any, error) {
	if actor.Role != "admin" {
		return nil, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	recipient = strings.TrimSpace(strings.ToLower(recipient))
	if !validEmail(recipient) {
		return nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}
	}
	id, err := platform.RandomToken(s.random, 18)
	if err != nil {
		return nil, err
	}
	dedupe, err := platform.RandomToken(s.random, 24)
	if err != nil {
		return nil, err
	}
	var locale string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(
		(SELECT preferences->>'locale' FROM merchants WHERE email_normalized=$1),
		(SELECT preferences->>'locale' FROM admins WHERE id=$2), 'zh-CN')`, recipient, actor.SubjectID).Scan(&locale); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(mailtmpl.Payload{Locale: locale})
	if err != nil {
		return nil, err
	}
	toCipher, err := platform.Encrypt(s.key[:], []byte(recipient), s.random)
	if err != nil {
		return nil, err
	}
	payloadCipher, err := platform.Encrypt(s.key[:], payload, s.random)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO mail_deliveries(public_id,dedupe_key,template,recipient_cipher,payload_cipher,state) VALUES($1,$2,$3,$4,$5,'queued')`, id, dedupe, mailtmpl.MailTest, toCipher, payloadCipher)
	if err != nil {
		return nil, err
	}
	return map[string]any{"delivery_id": id, "state": "queued"}, nil
}

func (s *Service) MailTestStatus(ctx context.Context, actor platform.Actor, id string) (map[string]any, error) {
	if actor.Role != "admin" {
		return nil, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	var state string
	var errorCode *string
	if err := s.pool.QueryRow(ctx, `SELECT state,last_error FROM mail_deliveries WHERE public_id=$1 AND template='mail_test'`, id).Scan(&state, &errorCode); err != nil {
		return nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
	}
	return map[string]any{"id": id, "state": state, "error_code": errorCode}, nil
}
