package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/mailtmpl"
	"vanillapay/internal/platform"
)

type TurnstileVerifier interface {
	Verify(context.Context, string, string, string, string) (bool, error)
}

type emailCodeRateLimitError struct {
	RetryAfterSeconds int
}

func (e *emailCodeRateLimitError) Error() string { return "EMAIL_SEND_TOO_SOON" }

func (e *emailCodeRateLimitError) Unwrap() error {
	return appError("EMAIL_SEND_TOO_SOON", http.StatusTooManyRequests)
}

type Siteverify struct {
	Client *http.Client
}

func (v Siteverify) Verify(ctx context.Context, secret, token, action, hostname string) (bool, error) {
	if secret == "" || token == "" {
		return false, nil
	}
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	form := url.Values{"secret": {secret}, "response": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return false, errors.New("turnstile request could not be created")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(req)
	if err != nil {
		return false, errors.New("turnstile verification unavailable")
	}
	defer func() { _ = response.Body.Close() }() // Cleanup must not replace the operation result.
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, errors.New("turnstile verification unavailable")
	}
	var result struct {
		Success  bool   `json:"success"`
		Action   string `json:"action"`
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(&result); err != nil {
		return false, errors.New("turnstile response invalid")
	}
	return result.Success && result.Action == action && strings.EqualFold(result.Hostname, hostname), nil
}

type Service struct {
	pool           *pgxpool.Pool
	attempts       *pgxpool.Pool
	clock          platform.Clock
	random         io.Reader
	encryptionKey  [32]byte
	mailer         platform.Mailer
	turnstile      TurnstileVerifier
	publicHostname string
}

func NewService(pool *pgxpool.Pool, key [32]byte, clock platform.Clock, random io.Reader, mailer platform.Mailer, turnstile TurnstileVerifier, publicBaseURL string) *Service {
	if clock == nil {
		clock = platform.SystemClock{}
	}
	if random == nil {
		random = rand.Reader
	}
	if mailer == nil {
		mailer = platform.DisabledMailer{}
	}
	publicURL, _ := url.Parse(publicBaseURL)
	var hostname string
	if publicURL != nil {
		hostname = publicURL.Hostname()
	}
	return &Service{publicHostname: hostname, pool: pool, clock: clock, random: random, encryptionKey: key, mailer: mailer, turnstile: turnstile}
}

// SetAttemptPool gives the service a pool reserved for counting verification
// code attempts; see consumeCode.
func (s *Service) SetAttemptPool(pool *pgxpool.Pool) {
	s.attempts = pool
}

func (s *Service) RotateBootstrapCode(ctx context.Context) (string, error) {
	if s.pool == nil {
		return "", appError("INTERNAL_ERROR", 500)
	}
	code, err := platform.RandomToken(s.random, 32)
	if err != nil {
		return "", err
	}
	var issued string
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(741928103)`); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE bootstrap SET code_hash=$1,expires_at=NULL,generated_at=$2,consumed_at=NULL WHERE singleton=true`, HashOpaque(code), s.clock.Now()); err != nil {
			return err
		}
		issued = code
		return nil
	})
	return issued, err
}

// IssueConsoleRecoveryCode creates a one-use code for the server-terminal
// recovery command. Issuing a new code invalidates any earlier code/session.
func (s *Service) IssueConsoleRecoveryCode(ctx context.Context) (string, error) {
	code, err := platform.RandomToken(s.random, 32)
	if err != nil {
		return "", err
	}
	var issued string
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(741928104)`); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admins WHERE id=1)`).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return appError("SETUP_REQUIRED", 409)
		}
		now := s.clock.Now()
		if _, err := tx.Exec(ctx, `UPDATE auth_challenges SET consumed_at=$1 WHERE role='admin' AND subject_id=1 AND purpose='recovery' AND consumed_at IS NULL`, now); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO auth_challenges(challenge_hash,role,subject_id,purpose,expires_at) VALUES($1,'admin',1,'recovery',$2) RETURNING id`, HashOpaque(code), time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)).Scan(new(int64)); err != nil {
			return err
		}
		if err := recordAudit(ctx, tx, platform.Actor{Role: "system"}, nil, "auth.console_recovery_issue", "admin", "1", "success", nil); err != nil {
			return err
		}
		issued = code
		return nil
	})
	return issued, err
}

// RecoverConsole consumes the terminal code and creates a ten-minute,
// repair-only browser session in the same transaction.
func (s *Service) RecoverConsole(ctx context.Context, code string) (ConsoleRecoverySession, error) {
	var result ConsoleRecoverySession
	if len(code) == 0 || len(code) > 128 {
		err := appError("INVALID_CODE", 400)
		if auditErr := s.recordFailureAudit(ctx, loginFailureEvent("admin", "auth.console_recovery", "INVALID_CODE", platform.Actor{Role: "admin"})); auditErr != nil {
			return result, auditErr
		}
		return result, err
	}
	token, err := platform.RandomToken(s.random, 32)
	if err != nil {
		return result, err
	}
	csrf, err := platform.RandomToken(s.random, 32)
	if err != nil {
		return result, err
	}
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var codeID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM auth_challenges WHERE challenge_hash=$1 AND role='admin' AND subject_id=1 AND purpose='recovery' AND secret_cipher IS NULL AND consumed_at IS NULL AND expires_at>$2 FOR UPDATE`, HashOpaque(code), s.clock.Now()).Scan(&codeID); err != nil {
			return appError("INVALID_CODE", 401)
		}
		tag, err := tx.Exec(ctx, `UPDATE auth_challenges SET consumed_at=$1 WHERE id=$2 AND consumed_at IS NULL`, s.clock.Now(), codeID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return appError("INVALID_CODE", 401)
		}
		expires := s.clock.Now().Add(10 * time.Minute)
		if err := tx.QueryRow(ctx, `INSERT INTO auth_challenges(challenge_hash,role,subject_id,purpose,secret_cipher,expires_at) VALUES($1,'admin',1,'recovery',$2,$3) RETURNING id`, HashOpaque(token), HashOpaque(csrf), expires).Scan(&result.ChallengeID); err != nil {
			return err
		}
		result.Token = token
		result.CSRFToken = csrf
		result.ExpiresAt = expires
		if err := tx.QueryRow(ctx, `SELECT version,security->>'turnstile_site_key',(security->>'turnstile_enabled')::boolean,EXISTS(SELECT 1 FROM secret_settings WHERE name='turnstile_secret' AND encrypted_value IS NOT NULL) FROM site_settings WHERE singleton=true`).Scan(&result.SettingsVersion, &result.Security.TurnstileSiteKey, &result.Security.TurnstileEnabled, &result.Security.TurnstileSecretConfigured); err != nil {
			return err
		}
		return recordAudit(ctx, tx, platform.Actor{Role: "admin", SubjectID: 1}, nil, "auth.console_recovery", "admin", "1", "success", nil)
	})
	if err != nil && authErrorCode(err) == "INVALID_CODE" {
		if auditErr := s.recordFailureAudit(ctx, loginFailureEvent("admin", "auth.console_recovery", "INVALID_CODE", platform.Actor{Role: "admin"})); auditErr != nil {
			return ConsoleRecoverySession{}, auditErr
		}
	}
	return result, err
}

// ResolveConsoleRecovery validates the independent repair cookie and its CSRF
// token and returns the database challenge ID for atomic consumption on save.
func (s *Service) ResolveConsoleRecovery(ctx context.Context, token, csrf string) (int64, error) {
	if token == "" || csrf == "" {
		return 0, appError("UNAUTHENTICATED", 401)
	}
	var id int64
	var csrfHash []byte
	var expires time.Time
	if err := s.pool.QueryRow(ctx, `SELECT id,secret_cipher,expires_at FROM auth_challenges WHERE challenge_hash=$1 AND role='admin' AND subject_id=1 AND purpose='recovery' AND consumed_at IS NULL`, HashOpaque(token)).Scan(&id, &csrfHash, &expires); err != nil || !expires.After(s.clock.Now()) || subtle.ConstantTimeCompare(csrfHash, HashOpaque(csrf)) != 1 {
		return 0, appError("UNAUTHENTICATED", 401)
	}
	return id, nil
}

func (s *Service) BootstrapStatus(ctx context.Context) (map[string]bool, error) {
	var initialized, enabled bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admins), (security->>'turnstile_enabled')::boolean AND btrim(security->>'turnstile_site_key') <> '' AND EXISTS(SELECT 1 FROM secret_settings WHERE name='turnstile_secret' AND octet_length(encrypted_value)>0) FROM site_settings WHERE singleton`).Scan(&initialized, &enabled); err != nil {
		return nil, err
	}
	return map[string]bool{"initialized": initialized, "turnstile_enabled": enabled}, nil
}

func (s *Service) Bootstrap(ctx context.Context, req BootstrapRequest) (SessionResult, error) {
	username, normErr := NormalizeUsername(req.Username)
	if normErr != nil {
		return SessionResult{}, appError("VALIDATION_ERROR", 400)
	}
	if err := ValidatePassword(req.Password, req.PasswordConfirmation); err != nil {
		return SessionResult{}, appError("VALIDATION_ERROR", 400)
	}
	passwordHash, err := HashPassword(req.Password, s.random)
	if err != nil {
		return SessionResult{}, err
	}
	var result SessionResult
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return appError("ALREADY_INITIALIZED", 409)
		}
		var stored []byte
		var expires *time.Time
		var consumed *time.Time
		if err := tx.QueryRow(ctx, `SELECT code_hash,expires_at,consumed_at FROM bootstrap WHERE singleton=true FOR UPDATE`).Scan(&stored, &expires, &consumed); err != nil {
			return err
		}
		// A concurrent bootstrap request may have observed the empty admin table
		// before waiting for this row lock. Recheck after the lock so that the
		// loser receives ALREADY_INITIALIZED instead of treating the consumed
		// one-use code as an ordinary invalid code.
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return appError("ALREADY_INITIALIZED", 409)
		}
		if len(stored) != 32 || consumed != nil || (expires != nil && !expires.After(s.clock.Now())) || subtle.ConstantTimeCompare(stored, HashOpaque(req.InitializationCode)) != 1 {
			return appError("INVALID_CODE", 400)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO admins(username,username_normalized,password_hash) VALUES($1,$2,$3)`, req.Username, username, passwordHash); err != nil {
			return mapIdentityConflict(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE bootstrap SET consumed_at=$1,code_hash=NULL WHERE singleton=true`, s.clock.Now()); err != nil {
			return err
		}
		var id int64
		if err := tx.QueryRow(ctx, `SELECT id FROM admins WHERE username_normalized=$1`, username).Scan(&id); err != nil {
			return err
		}
		account, err := (Repository{DB: tx}).ByID(ctx, "admin", id)
		if err != nil {
			return err
		}
		result, err = s.createSession(ctx, tx, account, 12*time.Hour, "/console")
		if err != nil {
			return err
		}
		return recordAudit(ctx, tx, platform.Actor{Role: "admin", SubjectID: id}, nil, "admin.bootstrap", "admin", fmt.Sprint(id), "success", nil)
	})
	return result, err
}

func (s *Service) SendEmailCode(ctx context.Context, actor platform.Actor, req EmailCodeRequest, locale string) (EmailCodeResult, error) {
	allowed := map[string]bool{"register": true, "reset_password": true, "change_username": true, "change_email_old": true, "change_email_new": true, "change_password": true, "reset_key": true}
	if !allowed[req.Purpose] {
		return EmailCodeResult{}, appError("VALIDATION_ERROR", 400)
	}
	email, err := NormalizeEmail(req.Email)
	if err != nil {
		return EmailCodeResult{}, appError("VALIDATION_ERROR", 400)
	}
	role := "public"
	var subject any
	if actor.SubjectID > 0 {
		role = actor.Role
		subject = actor.SubjectID
		if role != "merchant" && role != "admin" {
			return EmailCodeResult{}, appError("FORBIDDEN", 403)
		}
		var current string
		if role == "merchant" {
			var banned bool
			if err := s.pool.QueryRow(ctx, `SELECT email_normalized,banned_at IS NOT NULL,COALESCE(preferences->>'locale','zh-CN') FROM merchants WHERE id=$1`, actor.SubjectID).Scan(&current, &banned, &locale); err != nil {
				return EmailCodeResult{}, err
			}
			if banned {
				return EmailCodeResult{}, appError("ACCOUNT_BANNED", 403)
			}
		} else {
			return EmailCodeResult{}, appError("FORBIDDEN", 403)
		}
		if req.Purpose == "change_email_old" && email != current {
			return EmailCodeResult{}, appError("INVALID_CODE", 400)
		}
		if req.Purpose == "change_email_new" && email == current {
			return EmailCodeResult{}, appError("VALIDATION_ERROR", 400)
		}
	} else if req.Purpose != "register" && req.Purpose != "reset_password" {
		return EmailCodeResult{}, appError("UNAUTHENTICATED", 401)
	}
	if actor.SubjectID == 0 {
		if err := s.requireTurnstile(ctx, req.TurnstileToken, req.Purpose); err != nil {
			return EmailCodeResult{}, err
		}
	}
	if req.Purpose == "register" {
		if req.TargetUsername == "" {
			return EmailCodeResult{}, appError("VALIDATION_ERROR", 400)
		}
		normalizedTarget, err := NormalizeUsername(req.TargetUsername)
		if err != nil {
			return EmailCodeResult{}, appError("VALIDATION_ERROR", 400)
		}
		req.TargetUsername = normalizedTarget
		var enabled bool
		var domains []string
		if err := s.pool.QueryRow(ctx, `SELECT COALESCE((security->>'restrict_email_domains')::boolean,false),ARRAY(SELECT jsonb_array_elements_text(COALESCE(security->'email_domains','[]'::jsonb))) FROM site_settings WHERE singleton=true`).Scan(&enabled, &domains); err != nil {
			return EmailCodeResult{}, err
		}
		if enabled && !ValidateEmailDomain(email, domains) {
			return EmailCodeResult{}, appError("EMAIL_DOMAIN_DENIED", 403)
		}
	}
	result, err := s.issueEmailCode(ctx, req, email, role, subject, locale)
	if req.Purpose != "reset_password" {
		return result, err
	}
	// After public validation, account lookup, cooldown and delivery outcomes
	// must not reveal whether this mailbox belongs to a merchant.
	if err != nil {
		metadata, _ := platform.RequestMetadataFromContext(ctx)
		slog.ErrorContext(ctx, "reset_password_email_unavailable", "request_id", metadata.RequestID)
	}
	return EmailCodeResult{Sent: true, RetryAfterSeconds: 60}, nil
}

func (s *Service) issueEmailCode(ctx context.Context, req EmailCodeRequest, email, role string, subject any, locale string) (EmailCodeResult, error) {
	if role == "public" {
		var accountLocale *string
		if err := s.pool.QueryRow(ctx, `SELECT (SELECT COALESCE(preferences->>'locale','zh-CN') FROM merchants WHERE email_normalized=$1)`, email).Scan(&accountLocale); err != nil {
			return EmailCodeResult{}, err
		}
		if accountLocale != nil {
			locale = *accountLocale
		} else if req.Purpose == "reset_password" {
			return EmailCodeResult{Sent: true, RetryAfterSeconds: 60}, nil
		}
	}
	target := targetDigestValue(req.Purpose, req.TargetUsername, req.TargetEmail, email)
	var last *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT max(sent_at) FROM email_codes WHERE purpose=$1 AND email_normalized=$2 AND subject_role=$3 AND subject_id IS NOT DISTINCT FROM $4::bigint`, req.Purpose, email, role, subject).Scan(&last); err != nil {
		return EmailCodeResult{}, err
	}
	if last != nil && s.clock.Now().Before(last.Add(60*time.Second)) {
		if req.Purpose == "reset_password" {
			return EmailCodeResult{Sent: true, RetryAfterSeconds: 60}, nil
		}
		remaining := last.Add(60 * time.Second).Sub(s.clock.Now())
		retryAfter := int((remaining + time.Second - 1) / time.Second)
		if retryAfter < 1 {
			retryAfter = 1
		}
		return EmailCodeResult{}, &emailCodeRateLimitError{RetryAfterSeconds: retryAfter}
	}
	code, err := newNumericCode(s.random)
	if err != nil {
		return EmailCodeResult{}, err
	}
	var targetHash [32]byte
	copy(targetHash[:], target)
	var subjectID = subject
	var rowID int64
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO email_codes(purpose,subject_role,subject_id,email_normalized,target_digest,code_hash,sent_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, req.Purpose, role, subjectID, email, targetHash[:], HashOpaque(code), s.clock.Now(), s.clock.Now().Add(10*time.Minute)).Scan(&rowID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE email_codes SET consumed_at=$1 WHERE purpose=$2 AND email_normalized=$3 AND subject_role=$4 AND subject_id IS NOT DISTINCT FROM $5::bigint AND id<>$6 AND consumed_at IS NULL`, s.clock.Now(), req.Purpose, email, role, subjectID, rowID)
		return err
	})
	if err != nil {
		return EmailCodeResult{}, err
	}
	if err := s.mailer.Send(ctx, platform.MailMessage{
		To: email, Template: mailtmpl.Verification,
		Payload: mailtmpl.Payload{Locale: locale, Purpose: req.Purpose, Code: code},
	}); err != nil {
		_, _ = s.pool.Exec(ctx, `UPDATE email_codes SET consumed_at=$1 WHERE id=$2 AND consumed_at IS NULL`, s.clock.Now(), rowID)
		return EmailCodeResult{}, appError("MAIL_UNAVAILABLE", 503)
	}
	return EmailCodeResult{Sent: true, RetryAfterSeconds: 60}, nil
}

func targetDigestValue(purpose, username, targetEmail, email string) []byte {
	value := purpose + "\x00" + strings.ToLower(strings.TrimSpace(username)) + "\x00" + strings.ToLower(strings.TrimSpace(targetEmail)) + "\x00" + email
	return HashOpaque(value)
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (UserDTO, error) {
	username, err := NormalizeUsername(req.Username)
	if err != nil {
		return UserDTO{}, appError("VALIDATION_ERROR", 400)
	}
	email, err := NormalizeEmail(req.Email)
	if err != nil {
		return UserDTO{}, appError("VALIDATION_ERROR", 400)
	}
	if err := ValidatePassword(req.Password, req.PasswordConfirmation); err != nil {
		return UserDTO{}, appError("VALIDATION_ERROR", 400)
	}
	if !req.AcceptTerms || !req.AcceptPrivacy {
		return UserDTO{}, appError("AGREEMENT_REQUIRED", 400)
	}
	if err := s.requireTurnstile(ctx, req.TurnstileToken, "register"); err != nil {
		return UserDTO{}, err
	}
	passwordHash, err := HashPassword(req.Password, s.random)
	if err != nil {
		return UserDTO{}, err
	}
	var result UserDTO
	for attempt := 0; attempt < 3; attempt++ {
		pid, err := GeneratePID(s.random)
		if err != nil {
			return UserDTO{}, err
		}
		merchantKey, err := GenerateMerchantKey(s.random)
		if err != nil {
			return UserDTO{}, err
		}
		keyCipher, err := platform.Encrypt(s.encryptionKey[:], []byte(merchantKey), s.random)
		if err != nil {
			return UserDTO{}, err
		}
		err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			if err := sqlc.LockBusinessRows(ctx, tx, nil, false); err != nil {
				return err
			}
			var enabled bool
			var domains []string
			if err := tx.QueryRow(ctx, `SELECT COALESCE((security->>'restrict_email_domains')::boolean,false),ARRAY(SELECT jsonb_array_elements_text(COALESCE(security->'email_domains','[]'::jsonb))) FROM site_settings WHERE singleton=true`).Scan(&enabled, &domains); err != nil {
				return err
			}
			if enabled && !ValidateEmailDomain(email, domains) {
				return appError("EMAIL_DOMAIN_DENIED", 403)
			}
			if err := s.consumeCode(ctx, tx, "register", "public", nil, email, targetDigestValue("register", username, "", email), req.EmailCode, s.clock.Now()); err != nil {
				return err
			}
			var termsRev, privacyRev int64
			if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision) FILTER(WHERE kind='terms'),0),COALESCE(max(revision) FILTER(WHERE kind='privacy'),0) FROM legal_versions`).Scan(&termsRev, &privacyRev); err != nil {
				return err
			}
			if req.TermsRevision != termsRev || req.PrivacyRevision != privacyRev {
				return appError("LEGAL_VERSION_CHANGED", 400)
			}
			var id int64
			var created time.Time
			if err := tx.QueryRow(ctx, `INSERT INTO merchants(username,username_normalized,email,email_normalized,pid,password_hash,key_cipher) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,created_at`, req.Username, username, email, email, pid, passwordHash, keyCipher).Scan(&id, &created); err != nil {
				return mapIdentityConflict(err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO legal_acceptances(merchant_id,kind,revision) VALUES($1,'terms',$2),($1,'privacy',$3)`, id, termsRev, privacyRev); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO merchant_collection_settings(merchant_id) VALUES($1)`, id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO merchant_ip_rules(merchant_id) VALUES($1)`, id); err != nil {
				return err
			}
			result = UserDTO{ID: fmt.Sprint(id), Role: "merchant", Username: req.Username, Email: email, PID: pid, Banned: false, Locale: "zh-CN", Theme: "system", CreatedAt: created}
			merchantID := id
			return recordAudit(ctx, tx, platform.Actor{Role: "system", SubjectID: id, MerchantID: id}, &merchantID, "merchant.register", "merchant", fmt.Sprint(id), "success", nil)
		})
		if err == nil {
			return result, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "pid") {
			continue
		}
		return UserDTO{}, err
	}
	return UserDTO{}, appError("VERSION_CONFLICT", 409)
}

func (s *Service) Login(ctx context.Context, role string, req LoginRequest) (LoginResult, error) {
	if role != "merchant" && role != "admin" {
		return LoginResult{}, appError("VALIDATION_ERROR", 400)
	}
	if err := s.requireTurnstile(ctx, req.TurnstileToken, "login"); err != nil {
		if auditErr := s.recordFailureAudit(ctx, loginFailureEvent(role, "auth.login", authErrorCode(err), platform.Actor{Role: role})); auditErr != nil {
			return LoginResult{}, auditErr
		}
		return LoginResult{}, err
	}
	account, err := (Repository{DB: s.pool}).ByIdentifier(ctx, role, req.Identifier)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, err
	}
	if errors.Is(err, pgx.ErrNoRows) || !VerifyPassword(account.PasswordHash, req.Password) {
		actor := platform.Actor{Role: role}
		if account.ID > 0 {
			actor.SubjectID = account.ID
			if role == "merchant" {
				actor.MerchantID = account.ID
			}
		}
		failure := appError("INVALID_CREDENTIALS", 401)
		if auditErr := s.recordFailureAudit(ctx, loginFailureEvent(role, "auth.login", "INVALID_CREDENTIALS", actor)); auditErr != nil {
			return LoginResult{}, auditErr
		}
		return LoginResult{}, failure
	}
	if account.TOTPCipher != nil {
		challenge, err := platform.RandomToken(s.random, 32)
		if err != nil {
			return LoginResult{}, err
		}
		expires := s.clock.Now().Add(5 * time.Minute)
		err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO auth_challenges(challenge_hash,role,subject_id,purpose,expires_at) VALUES($1,$2,$3,'login',$4)`, HashOpaque(challenge), role, account.ID, expires); err != nil {
				return err
			}
			merchantID := int64(0)
			if role == "merchant" {
				merchantID = account.ID
			}
			actor := platform.Actor{Role: role, SubjectID: account.ID, MerchantID: merchantID}
			return recordAudit(ctx, tx, actor, int64PtrPositive(merchantID), "auth.login_challenge", "account", fmt.Sprint(account.ID), "success", nil)
		})
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{ChallengeID: challenge, ExpiresAt: expires, Requires2FA: true}, nil
	}
	var result SessionResult
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		account, err = (Repository{DB: tx}).ByID(ctx, role, account.ID)
		if err != nil {
			return err
		}
		result, err = s.createSession(ctx, tx, account, sessionTTL(role), nextPath(account))
		if err != nil {
			return err
		}
		merchantID := account.ID
		if role == "admin" {
			merchantID = 0
		}
		var mid *int64
		if merchantID > 0 {
			mid = &merchantID
		}
		return recordAudit(ctx, tx, platform.Actor{Role: role, SubjectID: account.ID, MerchantID: merchantID}, mid, "auth.login", "account", fmt.Sprint(account.ID), "success", nil)
	})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Session: &result}, nil
}

func (s *Service) VerifySecondFactor(ctx context.Context, req FactorRequest) (SessionResult, error) {
	var result SessionResult
	var role string
	var subject int64
	// Count the attempt before verifying so failures survive the rollback.
	err := s.pool.QueryRow(ctx, `UPDATE auth_challenges SET failed_attempts=failed_attempts+1 WHERE challenge_hash=$1 AND purpose='login' AND consumed_at IS NULL AND failed_attempts<$2 RETURNING id`, HashOpaque(req.ChallengeID), maxCodeAttempts).Scan(new(int64))
	if errors.Is(err, pgx.ErrNoRows) {
		err = appError("CHALLENGE_EXPIRED", 401)
	}
	if err == nil {
		err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			var challengeID int64
			var expires time.Time
			var consumed *time.Time
			var purpose string
			if err := tx.QueryRow(ctx, `SELECT id,role,subject_id,purpose,expires_at,consumed_at FROM auth_challenges WHERE challenge_hash=$1 FOR UPDATE`, HashOpaque(req.ChallengeID)).Scan(&challengeID, &role, &subject, &purpose, &expires, &consumed); err != nil {
				return appError("CHALLENGE_EXPIRED", 401)
			}
			if purpose != "login" || consumed != nil || !expires.After(s.clock.Now()) {
				return appError("CHALLENGE_EXPIRED", 401)
			}
			account, err := (Repository{DB: tx}).ByID(ctx, role, subject)
			if err != nil {
				return appError("CHALLENGE_EXPIRED", 401)
			}
			valid := false
			if req.TOTPCode != "" && len(account.TOTPCipher) > 0 {
				secret, err := platform.Decrypt(s.encryptionKey[:], account.TOTPCipher)
				if err != nil {
					return appError("INTERNAL_ERROR", 500)
				}
				step, ok := verifyTOTP(req.TOTPCode, string(secret), s.clock.Now())
				if ok && step > account.LastTOTPStep {
					var tag pgconn.CommandTag
					if role == "merchant" {
						tag, err = tx.Exec(ctx, `UPDATE merchants SET last_totp_step=$1 WHERE id=$2 AND last_totp_step<$1`, step, subject)
					} else {
						tag, err = tx.Exec(ctx, `UPDATE admins SET last_totp_step=$1 WHERE id=$2 AND last_totp_step<$1`, step, subject)
					}
					if err != nil {
						return err
					}
					valid = tag.RowsAffected() == 1
				}
			} else if req.RecoveryCode != "" && req.TOTPCode == "" {
				tag, err := tx.Exec(ctx, `UPDATE recovery_codes SET used_at=$1 WHERE role=$2 AND subject_id=$3 AND code_hash=$4 AND used_at IS NULL`, s.clock.Now(), role, subject, HashOpaque(strings.ToUpper(strings.TrimSpace(req.RecoveryCode))))
				if err != nil {
					return err
				}
				valid = tag.RowsAffected() == 1
			}
			if !valid {
				return appError("INVALID_2FA", 401)
			}
			if _, err := tx.Exec(ctx, `UPDATE auth_challenges SET consumed_at=$1 WHERE id=$2`, s.clock.Now(), challengeID); err != nil {
				return err
			}
			account, err = (Repository{DB: tx}).ByID(ctx, role, subject)
			if err != nil {
				return err
			}
			result, err = s.createSession(ctx, tx, account, sessionTTL(role), nextPath(account))
			if err != nil {
				return err
			}
			merchantID := int64(0)
			if role == "merchant" {
				merchantID = subject
			}
			return recordAudit(ctx, tx, platform.Actor{Role: role, SubjectID: subject, MerchantID: merchantID}, int64PtrPositive(merchantID), "auth.login_2fa", "account", fmt.Sprint(subject), "success", nil)
		})
	}
	if err != nil {
		code := authErrorCode(err)
		if code == "CHALLENGE_EXPIRED" || code == "INVALID_2FA" {
			actor := platform.Actor{Role: role, SubjectID: subject}
			if role == "merchant" {
				actor.MerchantID = subject
			}
			if actor.Role == "" {
				actor.Role = "system"
			}
			if auditErr := s.recordFailureAudit(ctx, loginFailureEvent(actor.Role, "auth.login_2fa", code, actor)); auditErr != nil {
				return SessionResult{}, auditErr
			}
		}
	}
	return result, err
}

func (s *Service) ResetPassword(ctx context.Context, req ResetPasswordRequest) error {
	email, err := NormalizeEmail(req.Email)
	if err != nil {
		return appError("VALIDATION_ERROR", 400)
	}
	if err := ValidatePassword(req.Password, req.PasswordConfirmation); err != nil {
		return appError("VALIDATION_ERROR", 400)
	}
	if err := s.requireTurnstile(ctx, req.TurnstileToken, "reset_password"); err != nil {
		return err
	}
	passwordHash, err := HashPassword(req.Password, s.random)
	if err != nil {
		return err
	}
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var id int64
		if err := tx.QueryRow(ctx, `SELECT id FROM merchants WHERE email_normalized=$1 FOR UPDATE`, email).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return appError("INVALID_CODE", 400)
			}
			return err
		}
		if err := s.consumeCode(ctx, tx, "reset_password", "public", nil, email, targetDigestValue("reset_password", "", "", email), req.EmailCode, s.clock.Now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE merchants SET password_hash=$1,updated_at=$2 WHERE id=$3`, passwordHash, s.clock.Now(), id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$1 WHERE role='merchant' AND subject_id=$2 AND revoked_at IS NULL`, s.clock.Now(), id); err != nil {
			return err
		}
		return recordAudit(ctx, tx, platform.Actor{Role: "merchant", SubjectID: id, MerchantID: id}, &id, "auth.password_reset", "merchant", fmt.Sprint(id), "success", nil)
	})
}

func (s *Service) ChangeIdentity(ctx context.Context, actor platform.Actor, req IdentityChange) (UserDTO, error) {
	if actor.Role != "merchant" || actor.SubjectID <= 0 {
		return UserDTO{}, appError("UNAUTHENTICATED", 401)
	}
	var normalized string
	if req.Username != "" {
		var err error
		normalized, err = NormalizeUsername(req.Username)
		if err != nil {
			return UserDTO{}, appError("VALIDATION_ERROR", 400)
		}
	}
	var email string
	if req.Email != "" {
		var err error
		email, err = NormalizeEmail(req.Email)
		if err != nil {
			return UserDTO{}, appError("VALIDATION_ERROR", 400)
		}
	}
	var passwordHash string
	if req.Password != "" {
		if err := ValidatePassword(req.Password, req.Confirmation); err != nil {
			return UserDTO{}, appError("VALIDATION_ERROR", 400)
		}
		var err error
		passwordHash, err = HashPassword(req.Password, s.random)
		if err != nil {
			return UserDTO{}, err
		}
	}
	var result UserDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		account, err := (Repository{DB: tx}).ByID(ctx, "merchant", actor.SubjectID)
		if err != nil {
			return err
		}
		if len(account.TOTPCipher) > 0 {
			secret, err := platform.Decrypt(s.encryptionKey[:], account.TOTPCipher)
			if err != nil {
				return err
			}
			step, ok := verifyTOTP(req.TOTPCode, string(secret), s.clock.Now())
			if !ok || step <= account.LastTOTPStep {
				return appError("INVALID_2FA", 401)
			}
			if _, err := tx.Exec(ctx, `UPDATE merchants SET last_totp_step=$1 WHERE id=$2`, step, actor.SubjectID); err != nil {
				return err
			}
		}
		if normalized != "" {
			if err := s.consumeCode(ctx, tx, "change_username", "merchant", &actor.SubjectID, account.Email, targetDigestValue("change_username", normalized, "", account.Email), req.EmailCode, s.clock.Now()); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE merchants SET username=$1,username_normalized=$2,updated_at=$3 WHERE id=$4`, req.Username, normalized, s.clock.Now(), actor.SubjectID); err != nil {
				return mapIdentityConflict(err)
			}
		}
		if email != "" {
			if err := s.consumeCode(ctx, tx, "change_email_old", "merchant", &actor.SubjectID, account.Email, targetDigestValue("change_email_old", "", account.Email, account.Email), req.OldEmailCode, s.clock.Now()); err != nil {
				return err
			}
			if err := s.consumeCode(ctx, tx, "change_email_new", "merchant", &actor.SubjectID, email, targetDigestValue("change_email_new", "", email, email), req.NewEmailCode, s.clock.Now()); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE merchants SET email=$1,email_normalized=$2,updated_at=$3 WHERE id=$4`, email, email, s.clock.Now(), actor.SubjectID); err != nil {
				return mapIdentityConflict(err)
			}
		}
		if passwordHash != "" {
			if err := s.consumeCode(ctx, tx, "change_password", "merchant", &actor.SubjectID, account.Email, targetDigestValue("change_password", "", account.Email, account.Email), req.EmailCode, s.clock.Now()); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE merchants SET password_hash=$1,updated_at=$2 WHERE id=$3`, passwordHash, s.clock.Now(), actor.SubjectID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$1 WHERE role='merchant' AND subject_id=$2 AND revoked_at IS NULL`, s.clock.Now(), actor.SubjectID); err != nil {
				return err
			}
		}
		account, err = (Repository{DB: tx}).ByID(ctx, "merchant", actor.SubjectID)
		if err != nil {
			return err
		}
		result = account.DTO()
		return recordAudit(ctx, tx, actor, &actor.MerchantID, "auth.identity_change", "merchant", fmt.Sprint(actor.SubjectID), "success", nil)
	})
	return result, err
}

func (s *Service) SetupTOTP(ctx context.Context, actor platform.Actor, password string) (TOTPSetup, error) {
	account, err := (Repository{DB: s.pool}).ByID(ctx, actor.Role, actor.SubjectID)
	if err != nil {
		return TOTPSetup{}, err
	}
	if !VerifyPassword(account.PasswordHash, password) {
		return TOTPSetup{}, appError("INVALID_CREDENTIALS", 401)
	}
	if len(account.TOTPCipher) > 0 {
		return TOTPSetup{}, appError("VERSION_CONFLICT", 409)
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "VanillaPay", AccountName: account.Username, Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1, SecretSize: 20})
	if err != nil {
		return TOTPSetup{}, err
	}
	setupID, err := platform.RandomToken(s.random, 32)
	if err != nil {
		return TOTPSetup{}, err
	}
	cipher, err := platform.Encrypt(s.encryptionKey[:], []byte(key.Secret()), s.random)
	if err != nil {
		return TOTPSetup{}, err
	}
	expires := s.clock.Now().Add(5 * time.Minute)
	if err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO auth_challenges(challenge_hash,role,subject_id,purpose,secret_cipher,expires_at) VALUES($1,$2,$3,'totp_setup',$4,$5)`, HashOpaque(setupID), actor.Role, actor.SubjectID, cipher, expires); err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, int64PtrPositive(actor.MerchantID), "auth.totp_setup", "account", fmt.Sprint(actor.SubjectID), "success", nil)
	}); err != nil {
		return TOTPSetup{}, err
	}
	return TOTPSetup{SetupID: setupID, Secret: key.Secret(), OTPAuthURL: key.URL(), ExpiresAt: expires}, nil
}

func (s *Service) ConfirmTOTP(ctx context.Context, actor platform.Actor, req TOTPConfirm) ([]string, error) {
	recovery := make([]string, 10)
	for i := range recovery {
		token, err := platform.RandomToken(s.random, 9)
		if err != nil {
			return nil, err
		}
		recovery[i] = strings.ToUpper(token[:12])
	}
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var id int64
		var cipher []byte
		var expires time.Time
		var consumed *time.Time
		if err := tx.QueryRow(ctx, `SELECT id,secret_cipher,expires_at,consumed_at FROM auth_challenges WHERE challenge_hash=$1 AND role=$2 AND subject_id=$3 AND purpose='totp_setup' FOR UPDATE`, HashOpaque(req.SetupID), actor.Role, actor.SubjectID).Scan(&id, &cipher, &expires, &consumed); err != nil {
			return appError("CHALLENGE_EXPIRED", 401)
		}
		if consumed != nil || !expires.After(s.clock.Now()) {
			return appError("CHALLENGE_EXPIRED", 401)
		}
		secret, err := platform.Decrypt(s.encryptionKey[:], cipher)
		if err != nil {
			return err
		}
		step, valid := verifyTOTP(req.TOTPCode, string(secret), s.clock.Now())
		if !valid {
			return appError("INVALID_2FA", 401)
		}
		if actor.Role == "merchant" {
			_, err = tx.Exec(ctx, `UPDATE merchants SET totp_cipher=$1,last_totp_step=$2,updated_at=$3 WHERE id=$4 AND totp_cipher IS NULL`, cipher, step, s.clock.Now(), actor.SubjectID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE admins SET totp_cipher=$1,last_totp_step=$2,updated_at=$3 WHERE id=$4 AND totp_cipher IS NULL`, cipher, step, s.clock.Now(), actor.SubjectID)
		}
		if err != nil {
			return err
		}
		for _, code := range recovery {
			if _, err := tx.Exec(ctx, `INSERT INTO recovery_codes(role,subject_id,code_hash) VALUES($1,$2,$3)`, actor.Role, actor.SubjectID, HashOpaque(code)); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE auth_challenges SET consumed_at=$1,secret_cipher=NULL WHERE id=$2`, s.clock.Now(), id)
		if err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, int64PtrPositive(actor.MerchantID), "auth.totp_confirm", "account", fmt.Sprint(actor.SubjectID), "success", nil)
	})
	if err != nil {
		return nil, err
	}
	return recovery, nil
}

func (s *Service) DisableTOTP(ctx context.Context, actor platform.Actor, proof FactorProof) error {
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		account, err := (Repository{DB: tx}).ByID(ctx, actor.Role, actor.SubjectID)
		if err != nil {
			return err
		}
		if !VerifyPassword(account.PasswordHash, proof.Password) {
			return appError("INVALID_CREDENTIALS", 401)
		}
		if len(account.TOTPCipher) == 0 {
			return appError("VERSION_CONFLICT", 409)
		}
		secret, err := platform.Decrypt(s.encryptionKey[:], account.TOTPCipher)
		if err != nil {
			return err
		}
		step, valid := verifyTOTP(proof.TOTPCode, string(secret), s.clock.Now())
		if !valid || step <= account.LastTOTPStep {
			return appError("INVALID_2FA", 401)
		}
		if actor.Role == "merchant" {
			_, err = tx.Exec(ctx, `UPDATE merchants SET totp_cipher=NULL,last_totp_step=$1,updated_at=$2 WHERE id=$3`, step, s.clock.Now(), actor.SubjectID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE admins SET totp_cipher=NULL,last_totp_step=$1,updated_at=$2 WHERE id=$3`, step, s.clock.Now(), actor.SubjectID)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM recovery_codes WHERE role=$1 AND subject_id=$2`, actor.Role, actor.SubjectID)
		if err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, int64PtrPositive(actor.MerchantID), "auth.totp_disable", "account", fmt.Sprint(actor.SubjectID), "success", nil)
	})
}

func (s *Service) VerifySecurityProofTx(ctx context.Context, tx pgx.Tx, actor platform.Actor, subjectID int64, purpose, emailCode, totpCode string) error {
	if actor.Role != "merchant" || actor.SubjectID != subjectID || subjectID <= 0 {
		return appError("UNAUTHENTICATED", 401)
	}
	account, err := (Repository{DB: tx}).ByID(ctx, "merchant", subjectID)
	if err != nil {
		return err
	}
	if err := s.consumeCode(ctx, tx, purpose, "merchant", &subjectID, account.Email, targetDigestValue(purpose, "", "", account.Email), emailCode, s.clock.Now()); err != nil {
		return err
	}
	if len(account.TOTPCipher) == 0 {
		return nil
	}
	secret, err := platform.Decrypt(s.encryptionKey[:], account.TOTPCipher)
	if err != nil {
		return appError("INTERNAL_ERROR", 500)
	}
	step, valid := verifyTOTP(totpCode, string(secret), s.clock.Now())
	if !valid || step <= account.LastTOTPStep {
		return appError("INVALID_2FA", 401)
	}
	tag, err := tx.Exec(ctx, `UPDATE merchants SET last_totp_step=$1 WHERE id=$2 AND last_totp_step<$1`, step, subjectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return appError("INVALID_2FA", 401)
	}
	return nil
}

type SessionIdentity struct {
	Actor     platform.Actor
	User      UserDTO
	CSRF      string
	ExpiresAt time.Time
}

func (s *Service) ResolveSession(ctx context.Context, token, role, csrfToken string, write bool) (SessionIdentity, error) {
	if token == "" || (role != "merchant" && role != "admin") {
		return SessionIdentity{}, appError("UNAUTHENTICATED", 401)
	}
	var subject int64
	var csrfHash []byte
	var expires time.Time
	if err := s.pool.QueryRow(ctx, `SELECT subject_id,csrf_hash,expires_at FROM sessions WHERE token_hash=$1 AND role=$2 AND revoked_at IS NULL`, HashOpaque(token), role).Scan(&subject, &csrfHash, &expires); err != nil || !expires.After(s.clock.Now()) {
		return SessionIdentity{}, appError("UNAUTHENTICATED", 401)
	}
	if write && (csrfToken == "" || subtle.ConstantTimeCompare(csrfHash, HashOpaque(csrfToken)) != 1) {
		return SessionIdentity{}, appError("CSRF_FAILED", 403)
	}
	account, err := (Repository{DB: s.pool}).ByID(ctx, role, subject)
	if err != nil {
		return SessionIdentity{}, appError("UNAUTHENTICATED", 401)
	}
	merchantID := int64(0)
	if role == "merchant" {
		merchantID = subject
	}
	return SessionIdentity{Actor: platform.Actor{Role: role, SubjectID: subject, MerchantID: merchantID}, User: account.DTO(), CSRF: csrfToken, ExpiresAt: expires}, nil
}

func (s *Service) createSession(ctx context.Context, tx pgx.Tx, account Account, ttl time.Duration, path string) (SessionResult, error) {
	token, err := platform.RandomToken(s.random, 32)
	if err != nil {
		return SessionResult{}, err
	}
	csrf, err := platform.RandomToken(s.random, 32)
	if err != nil {
		return SessionResult{}, err
	}
	expires := s.clock.Now().Add(ttl)
	if _, err := tx.Exec(ctx, `INSERT INTO sessions(token_hash,csrf_hash,role,subject_id,expires_at) VALUES($1,$2,$3,$4,$5)`, HashOpaque(token), HashOpaque(csrf), account.Role, account.ID, expires); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{User: account.DTO(), CSRFToken: csrf, ExpiresAt: expires, NextPath: path, Token: token}, nil
}

func sessionTTL(role string) time.Duration {
	if role == "admin" {
		return 12 * time.Hour
	}
	return 7 * 24 * time.Hour
}
func nextPath(account Account) string {
	if account.Role == "admin" {
		return "/console"
	}
	if account.BannedAt != nil {
		return "/banned"
	}
	return "/dashboard"
}

func (s *Service) requireTurnstile(ctx context.Context, token, action string) error {
	var enabled bool
	var siteKey string
	var encrypted []byte
	if err := s.pool.QueryRow(ctx, `SELECT (security->>'turnstile_enabled')::boolean, security->>'turnstile_site_key', (SELECT encrypted_value FROM secret_settings WHERE name='turnstile_secret') FROM site_settings WHERE singleton`).Scan(&enabled, &siteKey, &encrypted); err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	if s.turnstile == nil || s.publicHostname == "" || strings.TrimSpace(siteKey) == "" || len(encrypted) == 0 {
		return appError("SETUP_REQUIRED", 503)
	}
	secret, err := platform.Decrypt(s.encryptionKey[:], encrypted)
	if err != nil {
		return appError("TURNSTILE_UNAVAILABLE", 503)
	}
	return VerifyTurnstile(ctx, s.turnstile, string(secret), token, action, s.publicHostname)
}

func VerifyTurnstile(ctx context.Context, verifier TurnstileVerifier, secret, token, action, hostname string) error {
	if token == "" {
		return appError("TURNSTILE_FAILED", 400)
	}
	valid, err := verifier.Verify(ctx, secret, token, action, hostname)
	if err != nil {
		return appError("TURNSTILE_UNAVAILABLE", 503)
	}
	if !valid {
		return appError("TURNSTILE_FAILED", 400)
	}
	return nil
}

// maxCodeAttempts bounds guesses against one email code or login challenge.
const maxCodeAttempts = 5

// consumeCode counts the attempt outside tx before checking it inside tx, so
// a wrong guess stays counted when the caller's transaction rolls back. The
// count uses the reserved attempt pool: tx already holds a main-pool
// connection and row locks that concurrent requests wait on while holding
// their own connections, so taking another main-pool connection here can
// starve forever.
func (s *Service) consumeCode(ctx context.Context, tx pgx.Tx, purpose, role string, subject *int64, email string, target []byte, code string, now time.Time) error {
	counter := s.attempts
	if counter == nil {
		counter = s.pool
	}
	countCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var id int64
	if err := counter.QueryRow(countCtx, `UPDATE email_codes SET failed_attempts=failed_attempts+1 WHERE id=(SELECT id FROM email_codes WHERE purpose=$1 AND subject_role=$2 AND subject_id IS NOT DISTINCT FROM $3::bigint AND email_normalized=$4 AND target_digest=$5 AND consumed_at IS NULL ORDER BY sent_at DESC LIMIT 1) AND failed_attempts<$6 RETURNING id`, purpose, role, subject, email, target, maxCodeAttempts).Scan(&id); err != nil {
		return appError("INVALID_CODE", 400)
	}
	var stored []byte
	var expires time.Time
	if err := tx.QueryRow(ctx, `SELECT code_hash,expires_at FROM email_codes WHERE id=$1 AND consumed_at IS NULL FOR UPDATE`, id).Scan(&stored, &expires); err != nil {
		return appError("INVALID_CODE", 400)
	}
	if !expires.After(now) || !ValidSixDigitCode(code) || subtle.ConstantTimeCompare(stored, HashOpaque(code)) != 1 {
		return appError("INVALID_CODE", 400)
	}
	_, err := tx.Exec(ctx, `UPDATE email_codes SET consumed_at=$1 WHERE id=$2 AND consumed_at IS NULL`, now, id)
	return err
}

func recordAudit(ctx context.Context, tx pgx.Tx, actor platform.Actor, merchantID *int64, action, targetType, targetID, result string, metadata []byte) error {
	if metadata == nil {
		metadata = []byte(`{}`)
	}
	return platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: merchantID, Action: action, TargetType: targetType, TargetID: targetID, Result: result, SafeMetadata: json.RawMessage(metadata)})
}

func loginFailureEvent(role, action, code string, actor platform.Actor) platform.AuditEvent {
	if role != "merchant" && role != "admin" && role != "system" && role != "device" {
		role = "system"
	}
	actor.Role = role
	if actor.SubjectID <= 0 {
		actor.SubjectID = 0
		actor.MerchantID = 0
	}
	var merchantID *int64
	if role == "merchant" && actor.MerchantID > 0 {
		merchantID = int64PtrPositive(actor.MerchantID)
	}
	targetID := "unknown"
	if actor.SubjectID > 0 {
		targetID = fmt.Sprint(actor.SubjectID)
	}
	metadata, _ := json.Marshal(map[string]string{"error_code": code})
	return platform.AuditEvent{Actor: actor, MerchantID: merchantID, Action: action, TargetType: "account", TargetID: targetID, Result: "failure", SafeMetadata: metadata}
}

func (s *Service) recordFailureAudit(ctx context.Context, event platform.AuditEvent) error {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := platform.RecordAudit(auditCtx, s.pool, event); err != nil {
		return appError("INTERNAL_ERROR", 500)
	}
	return nil
}

func authErrorCode(err error) string {
	var apiErr *platform.Error
	if errors.As(err, &apiErr) && apiErr.Code != "" {
		return apiErr.Code
	}
	return "INTERNAL_ERROR"
}

func int64PtrPositive(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func mapIdentityConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if strings.Contains(pgErr.ConstraintName, "username") {
			return appError("USERNAME_TAKEN", 409)
		}
		if strings.Contains(pgErr.ConstraintName, "email") {
			return appError("EMAIL_TAKEN", 409)
		}
	}
	return err
}

func appError(code string, status int) error { return &platform.Error{Code: code, HTTPStatus: status} }
