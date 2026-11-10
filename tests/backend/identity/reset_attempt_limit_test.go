package identity

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/auth"
	"vanillapay/tests/backend/testsupport"
)

// With Turnstile off nothing but the attempt counter stands between a guesser
// and a reset code, so wrong guesses must stay counted, concurrent guesses
// must never exceed the limit and an exhausted code must stay dead.
func TestHTTP_PasswordResetAttemptLimitWithoutTurnstile(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	hash, err := auth.HashPassword(fixturePassword, nil)
	if err != nil {
		t.Fatal(err)
	}
	var email string
	if err := db.Pool.QueryRow(t.Context(), `UPDATE merchants SET password_hash=$1 WHERE id=$2 RETURNING email_normalized`, hash, merchantID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	issue := func(t *testing.T) string {
		t.Helper()
		response := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", auth.EmailCodeRequest{Purpose: "reset_password", Email: email}, "guest")
		assertIdentityResponse(t, response, http.StatusOK, "issue reset code without Turnstile")
		code := fixture.Mailer.LastCode()
		if len(code) != 6 {
			t.Fatal("reset mail did not contain a verification code")
		}
		return code
	}
	reset := func(t *testing.T, code string) *httptest.ResponseRecorder {
		t.Helper()
		return fixture.Request(t, http.MethodPost, "/api/v1/auth/reset-password", auth.ResetPasswordRequest{Email: email, EmailCode: code, Password: fixtureChangedPassword, PasswordConfirmation: fixtureChangedPassword}, "guest")
	}
	wrongFor := func(code string) string {
		if code == "000000" {
			return "000001"
		}
		return "000000"
	}
	failures := func(t *testing.T) int {
		t.Helper()
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT failed_attempts FROM email_codes WHERE email_normalized=$1 AND purpose='reset_password' ORDER BY sent_at DESC LIMIT 1`, email).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	passwordChanged := func(t *testing.T) bool {
		t.Helper()
		var stored string
		if err := db.Pool.QueryRow(t.Context(), `SELECT password_hash FROM merchants WHERE id=$1`, merchantID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		return auth.VerifyPassword(stored, fixtureChangedPassword)
	}

	correct := issue(t)
	for attempt := 1; attempt <= 2; attempt++ {
		assertIdentityError(t, reset(t, wrongFor(correct)), "INVALID_CODE", "wrong reset attempt")
		if failures(t) != attempt {
			t.Fatalf("failed attempt %d was not persisted: %d", attempt, failures(t))
		}
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if got := reset(t, wrongFor(correct)); got.Code != http.StatusBadRequest {
				t.Errorf("concurrent wrong reset status=%d", got.Code)
			}
		})
	}
	wg.Wait()
	if got := failures(t); got != 5 {
		t.Fatalf("concurrent wrong attempts counted %d, want exactly the limit 5", got)
	}
	assertIdentityError(t, reset(t, correct), "INVALID_CODE", "exhausted reset code")
	if passwordChanged(t) {
		t.Fatal("exhausted code still reset the password")
	}

	db.Clock.Set(db.Clock.Now().Add(61 * time.Second))
	fresh := issue(t)
	if got := reset(t, fresh); got.Code != http.StatusOK || !passwordChanged(t) {
		t.Fatalf("new reset code after exhaustion = %d %s", got.Code, got.Body.String())
	}
}
