package settings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"testing"
	"time"

	"vanillapay/internal/auth"
	"vanillapay/internal/httpx"
	"vanillapay/internal/jobs"
	"vanillapay/internal/platform"
	internalsettings "vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_PasswordResetMailDoesNotRevealAccount(t *testing.T) {
	for _, scenario := range []string{"normal", "cooldown", "smtp_failure"} {
		t.Run(scenario, func(t *testing.T) {
			db := testsupport.NewDatabase(t)
			merchantID, _ := db.SeedMerchant(t)
			fixture := testsupport.NewHTTPFixture(t, db)
			// Keep the real HTTP/Turnstile path, using the production mail queue.
			fixture.App.Services.Auth = auth.NewService(db.Pool, db.Key, db.Clock, nil, jobs.QueueMailer{Pool: db.Pool, Key: db.Key}, fixture.Siteverify, fixture.App.PublicBaseURL())
			fixture.Handler = httpx.NewRouter(fixture.App)
			var version int64
			if err := db.Pool.QueryRow(t.Context(), `SELECT version FROM site_settings WHERE singleton=true`).Scan(&version); err != nil {
				t.Fatal(err)
			}
			security := json.RawMessage(`{"turnstile_site_key":"reset-site-key","turnstile_secret":"reset-site-secret","turnstile_enabled":true, "turnstile_token": "ok:settings_security","restrict_email_domains":false,"email_domains":[],"ban_contact":{"zh_CN":"support@example.test","en":"support@example.test"}}`)
			if _, err := fixture.App.Services.Settings.UpdateGroup(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, "security", internalsettings.UpdateGroup{ExpectedVersion: version, Value: security}); err != nil {
				t.Fatal(err)
			}
			fixture.Siteverify.SetMode("accept")
			var email string
			if err := db.Pool.QueryRow(t.Context(), `SELECT email_normalized FROM merchants WHERE id=$1`, merchantID).Scan(&email); err != nil {
				t.Fatal(err)
			}
			var sink *localSMTPSink
			if scenario == "smtp_failure" {
				configureMailWorker(t, db, fixture.App, "127.0.0.1", unusedLocalPort(t), "reset-smtp-user", fixturePassword)
			} else {
				sink = newTrustedLocalSMTPSink(t)
				configureMailWorker(t, db, fixture.App, "127.0.0.1", sink.port, "reset-smtp-user", fixturePassword)
			}
			requestPair := func() {
				t.Helper()
				var known []byte
				for _, address := range []string{email, "absent-reset-mail@example.test"} {
					body, err := json.Marshal(auth.EmailCodeRequest{Purpose: "reset_password", Email: address, TurnstileToken: "ok:reset_password"})
					if err != nil {
						t.Fatal(err)
					}
					response := fixture.RequestWithHeaders(t, http.MethodPost, "/api/v1/auth/email-codes", bytes.NewReader(body), "guest", http.Header{"Content-Type": {"application/json"}, "X-Request-Id": {"reset-mail-parity"}})
					if response.Code != http.StatusOK {
						t.Fatalf("reset email status=%d, want 200", response.Code)
					}
					want := []byte(`{"data":{"sent":true,"retry_after_seconds":60},"error":null,"request_id":"reset-mail-parity"}`)
					if !bytes.Equal(bytes.TrimSpace(response.Body.Bytes()), want) {
						t.Fatal("reset email must return the same complete public envelope in every scenario")
					}
					var envelope struct {
						Data  auth.EmailCodeResult `json:"data"`
						Error json.RawMessage      `json:"error"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
						t.Fatal(err)
					}
					if string(envelope.Error) != "null" || !envelope.Data.Sent || envelope.Data.RetryAfterSeconds != 60 {
						t.Fatal("reset email must return the same successful result without an error code")
					}
					if address == email {
						known = bytes.Clone(response.Body.Bytes())
					} else if !bytes.Equal(known, response.Body.Bytes()) {
						t.Fatal("reset email public response reveals whether the account exists")
					}
				}
			}
			requestPair()
			if scenario == "cooldown" {
				requestPair()
				var codes, deliveries int
				if err := db.Pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM email_codes WHERE purpose='reset_password'),(SELECT count(*) FROM mail_deliveries WHERE template='transactional')`).Scan(&codes, &deliveries); err != nil || codes != 1 || deliveries != 1 {
					t.Fatalf("cooldown created another code or mail: codes=%d deliveries=%d err=%v", codes, deliveries, err)
				}
			}
			var deliveryID string
			if err := db.Pool.QueryRow(t.Context(), `SELECT public_id FROM mail_deliveries WHERE template='transactional'`).Scan(&deliveryID); err != nil {
				t.Fatal(err)
			}
			startMailWorkers(t, fixture.App)
			if scenario == "smtp_failure" {
				waitMailAttempts(t, db, deliveryID, 1, 10*time.Second)
				var errorCode string
				if err := db.Pool.QueryRow(t.Context(), `SELECT last_error FROM mail_deliveries WHERE public_id=$1`, deliveryID).Scan(&errorCode); err != nil || errorCode != "smtp_delivery_failed" {
					t.Fatalf("SMTP failure must be retained for the existing retry worker: code=%q err=%v", errorCode, err)
				}
				return
			}
			var code string
			select {
			case raw := <-sink.message:
				message, err := mail.ReadMessage(strings.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				body, err := readMIMEBody(message)
				if err != nil {
					t.Fatal(err)
				}
				codes := regexp.MustCompile(`verification code is ([0-9]{6})\.`).FindStringSubmatch(body)
				if len(codes) != 2 || !strings.Contains(message.Header.Get("To"), email) {
					t.Fatal("registered mailbox did not receive its reset verification code")
				}
				code = codes[1]
			case <-time.After(10 * time.Second):
				t.Fatal("local SMTP server did not receive the password reset mail")
			}
			// The delivered code remains usable, including after a silent cooldown.
			reset := fixture.Request(t, http.MethodPost, "/api/v1/auth/reset-password", auth.ResetPasswordRequest{Email: email, EmailCode: code, Password: fixtureChangedPassword, PasswordConfirmation: fixtureChangedPassword, TurnstileToken: "ok:reset_password"}, "guest")
			if reset.Code != http.StatusOK {
				t.Fatalf("reset with delivered code status=%d, want 200", reset.Code)
			}
			var hash string
			if err := db.Pool.QueryRow(t.Context(), `SELECT password_hash FROM merchants WHERE id=$1`, merchantID).Scan(&hash); err != nil || !auth.VerifyPassword(hash, fixtureChangedPassword) {
				t.Fatal("reset with SMTP-delivered code did not change the password")
			}
		})
	}
}
