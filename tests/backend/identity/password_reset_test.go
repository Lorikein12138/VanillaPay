package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"vanillapay/internal/auth"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_PasswordResetInvalidProofDoesNotRevealAccount(t *testing.T) {
	for _, state := range []string{"missing", "incorrect", "expired", "consumed", "exhausted"} {
		t.Run(state, func(t *testing.T) {
			fixture, merchantID, email := newPasswordResetFixture(t)
			db := fixture.DB
			var originalHash string
			if err := db.Pool.QueryRow(t.Context(), `SELECT password_hash FROM merchants WHERE id=$1`, merchantID).Scan(&originalHash); err != nil {
				t.Fatal(err)
			}
			code := "000000"
			if state != "missing" {
				code = sendPasswordResetCode(t, fixture, email)
			}
			switch state {
			case "incorrect":
				if code == "000000" {
					code = "000001"
				} else {
					code = "000000"
				}
			case "expired":
				db.Clock.Set(db.Clock.Now().Add(11 * time.Minute))
			case "consumed":
				if _, err := db.Pool.Exec(t.Context(), `UPDATE email_codes SET consumed_at=$1 WHERE email_normalized=$2`, db.Clock.Now(), email); err != nil {
					t.Fatal(err)
				}
			case "exhausted":
				if _, err := db.Pool.Exec(t.Context(), `UPDATE email_codes SET failed_attempts=5 WHERE email_normalized=$1`, email); err != nil {
					t.Fatal(err)
				}
			}
			var knownError json.RawMessage
			for _, address := range []string{email, "absent-reset@example.test"} {
				response := fixture.Request(t, http.MethodPost, "/api/v1/auth/reset-password", auth.ResetPasswordRequest{
					Email: address, EmailCode: code, Password: fixtureChangedPassword, PasswordConfirmation: fixtureChangedPassword, TurnstileToken: "ok:reset_password",
				}, "guest")
				if response.Code != http.StatusBadRequest {
					t.Fatalf("reset status=%d want=%d", response.Code, http.StatusBadRequest)
				}
				assertIdentityError(t, response, "INVALID_CODE", "invalid reset proof")
				var envelope struct {
					Error json.RawMessage `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if address == email {
					knownError = envelope.Error
				} else if !bytes.Equal(knownError, envelope.Error) {
					t.Fatal("reset error payload reveals whether the account exists")
				}
			}
			var currentHash string
			var activeSessions int
			if err := db.Pool.QueryRow(t.Context(), `SELECT password_hash,(SELECT count(*) FROM sessions WHERE role='merchant' AND subject_id=$1 AND revoked_at IS NULL) FROM merchants WHERE id=$1`, merchantID).Scan(&currentHash, &activeSessions); err != nil {
				t.Fatal(err)
			}
			if currentHash != originalHash || activeSessions != 1 {
				t.Fatal("rejected reset changed the password or revoked a session")
			}
		})
	}
}

func TestHTTP_PasswordResetValidCodeChangesPasswordAndRevokesSessions(t *testing.T) {
	fixture, merchantID, email := newPasswordResetFixture(t)
	code := sendPasswordResetCode(t, fixture, email)
	request := auth.ResetPasswordRequest{Email: email, EmailCode: code, Password: fixtureChangedPassword, PasswordConfirmation: fixtureChangedPassword, TurnstileToken: "ok:reset_password"}
	response := fixture.Request(t, http.MethodPost, "/api/v1/auth/reset-password", request, "guest")
	if response.Code != http.StatusOK {
		t.Fatalf("valid password reset status=%d", response.Code)
	}
	var hash string
	var activeSessions, consumedCodes, audits int
	if err := fixture.DB.Pool.QueryRow(t.Context(), `SELECT password_hash,(SELECT count(*) FROM sessions WHERE role='merchant' AND subject_id=$1 AND revoked_at IS NULL),(SELECT count(*) FROM email_codes WHERE email_normalized=$2 AND purpose='reset_password' AND consumed_at IS NOT NULL),(SELECT count(*) FROM audit_logs WHERE merchant_id=$1 AND action='auth.password_reset' AND result='success') FROM merchants WHERE id=$1`, merchantID, email).Scan(&hash, &activeSessions, &consumedCodes, &audits); err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPassword(hash, fixtureChangedPassword) || auth.VerifyPassword(hash, fixturePassword) || activeSessions != 0 || consumedCodes != 1 || audits != 1 {
		t.Fatal("successful reset did not atomically change password, consume proof, revoke sessions and record audit")
	}
	replay := fixture.Request(t, http.MethodPost, "/api/v1/auth/reset-password", request, "guest")
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("reset code replay status=%d", replay.Code)
	}
	assertIdentityError(t, replay, "INVALID_CODE", "reset code replay")
}

func TestDB_PasswordResetPreservesAccountQueryError(t *testing.T) {
	fixture, _, email := newPasswordResetFixture(t)
	if _, err := fixture.DB.Pool.Exec(t.Context(), `ALTER TABLE merchants RENAME TO reset_password_query_failure`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := fixture.DB.Pool.Exec(ctx, `ALTER TABLE reset_password_query_failure RENAME TO merchants`); err != nil {
			t.Errorf("restore merchant table: %v", err)
		}
	})
	err := fixture.App.Services.Auth.ResetPassword(t.Context(), auth.ResetPasswordRequest{
		Email: email, EmailCode: "000000", Password: fixtureChangedPassword, PasswordConfirmation: fixtureChangedPassword, TurnstileToken: "ok:reset_password",
	})
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "42P01" {
		t.Fatalf("reset must preserve original database error, got %v", err)
	}
}

func TestHTTP_PasswordResetMailQueueFailureDoesNotRevealAccount(t *testing.T) {
	fixture, _, email := newPasswordResetFixture(t)
	fixture.Mailer.SetError(errors.New("unavailable mail queue"))
	var logs bytes.Buffer
	logger := slog.Default()
	logWriter, logFlags := log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(logger)
		log.SetOutput(logWriter)
		log.SetFlags(logFlags)
	})
	var known []byte
	for _, address := range []string{email, "absent-reset@example.test"} {
		response := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", auth.EmailCodeRequest{Purpose: "reset_password", Email: address, TurnstileToken: "ok:reset_password"}, "guest")
		assertIdentityResponse(t, response, http.StatusOK, "unavailable reset mail queue")
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		delete(envelope, "request_id")
		public, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		if address == email {
			known = public
		} else if !bytes.Equal(known, public) {
			t.Fatal("unavailable reset mail queue reveals whether the account exists")
		}
	}
	if !strings.Contains(logs.String(), "reset_password_email_unavailable") || strings.Contains(logs.String(), email) || strings.Contains(logs.String(), fixture.Mailer.LastCode()) {
		t.Fatal("reset mail preparation failure must be logged without the mailbox or code")
	}
	var activeCodes int
	if err := fixture.DB.Pool.QueryRow(t.Context(), `SELECT count(*) FROM email_codes WHERE purpose='reset_password' AND consumed_at IS NULL`).Scan(&activeCodes); err != nil || activeCodes != 0 {
		t.Fatalf("failed queue submission left a usable reset code: active=%d err=%v", activeCodes, err)
	}
	// Other email-code purposes must still report mail failures.
	registration := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", auth.EmailCodeRequest{Purpose: "register", Email: "register-mail-failure@example.test", TargetUsername: "register_mail_failure", TurnstileToken: "ok:register"}, "guest")
	if registration.Code != http.StatusServiceUnavailable {
		t.Fatalf("registration mail failure status=%d, want 503", registration.Code)
	}
	assertIdentityError(t, registration, "MAIL_UNAVAILABLE", "registration mail failure remains visible")
}

func TestHTTP_PasswordResetEmailValidationPrecedesUniformResponse(t *testing.T) {
	fixture, _, email := newPasswordResetFixture(t)
	for _, test := range []struct {
		name, email, mode, code string
		status                  int
	}{
		{"invalid_email", "invalid-address", "accept", "VALIDATION_ERROR", http.StatusBadRequest},
		{"invalid_turnstile", email, "wrong-action", "TURNSTILE_FAILED", http.StatusBadRequest},
		{"turnstile_unavailable", email, "unavailable", "TURNSTILE_UNAVAILABLE", http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture.Siteverify.SetMode(test.mode)
			response := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", auth.EmailCodeRequest{Purpose: "reset_password", Email: test.email, TurnstileToken: "ok:reset_password"}, "guest")
			if response.Code != test.status {
				t.Fatalf("reset email validation status=%d want=%d", response.Code, test.status)
			}
			assertIdentityError(t, response, test.code, "reset email public validation")
		})
	}
	if len(fixture.Mailer.Messages()) != 0 {
		t.Fatal("failed public validation sent a reset email")
	}
}

func newPasswordResetFixture(t *testing.T) (*testsupport.HTTPFixture, int64, string) {
	t.Helper()
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	if err := configureTOTPTestSiteverify(t, fixture, t.Context()); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword(fixturePassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	var email string
	if err := db.Pool.QueryRow(t.Context(), `UPDATE merchants SET password_hash=$1 WHERE id=$2 RETURNING email_normalized`, hash, merchantID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	login := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", auth.LoginRequest{Identifier: email, Password: fixturePassword, TurnstileToken: "ok:login"}, "merchant:password-reset")
	assertIdentityResponse(t, login, http.StatusOK, "session before password reset")
	return fixture, merchantID, email
}

func sendPasswordResetCode(t *testing.T, fixture *testsupport.HTTPFixture, email string) string {
	t.Helper()
	response := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", auth.EmailCodeRequest{Purpose: "reset_password", Email: email, TurnstileToken: "ok:reset_password"}, "guest")
	assertIdentityResponse(t, response, http.StatusOK, "issue password reset code")
	code := fixture.Mailer.LastCode()
	if len(code) != 6 {
		t.Fatal("reset mail did not contain a verification code")
	}
	return code
}
