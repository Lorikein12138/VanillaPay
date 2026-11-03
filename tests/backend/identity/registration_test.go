package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_RegistrationLoginAndEmailFlows(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO legal_versions(kind,revision,content) VALUES('terms',1,'{"zh_CN":"terms","en":"terms"}'),('privacy',1,'{"zh_CN":"privacy","en":"privacy"}')`); err != nil {
		t.Fatalf("seed current legal revisions: %v", err)
	}
	if err := configureTOTPTestSiteverify(t, fixture, ctx); err != nil {
		t.Fatalf("configure local Siteverify: %v", err)
	}
	firstEmail := "first-user@personal.example.test"
	firstUsername := "first_user"
	codeResponse := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{
		"purpose": "register", "email": firstEmail, "target_username": firstUsername, "turnstile_token": "ok:register",
	}, "guest")
	assertIdentityResponse(t, codeResponse, http.StatusOK, "registration email code")
	firstCode := fixture.Mailer.LastCode()
	if len(firstCode) != 6 {
		t.Fatalf("registration mail code = %q", firstCode)
	}
	var data json.RawMessage
	t.Run("V07.B.r01/v01", func(t *testing.T) {
		registered := fixture.Request(t, http.MethodPost, "/api/v1/auth/register", map[string]any{
			"username": firstUsername, "email": firstEmail, "email_code": firstCode,
			"password": fixturePassword, "password_confirmation": fixturePassword,
			"accept_terms": true, "accept_privacy": true, "terms_revision": 1, "privacy_revision": 1, "turnstile_token": "ok:register",
		}, "guest")
		data = assertIdentityResponse(t, registered, http.StatusCreated, "registration with verified email and current legal revisions")
	})
	var registeredEnvelope struct {
		NextPath string `json:"next_path"`
		User     struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Email    string `json:"email"`
			PID      string `json:"pid"`
		} `json:"user"`
	}
	t.Run("V07.B.r01/v02", func(t *testing.T) {
		if err := json.Unmarshal(data, &registeredEnvelope); err != nil || registeredEnvelope.NextPath != "/login" || registeredEnvelope.User.ID != "1" || registeredEnvelope.User.Username != firstUsername || registeredEnvelope.User.Email != firstEmail || len(registeredEnvelope.User.PID) != 16 {
			t.Fatalf("registered first account = %s err=%v", data, err)
		}
		var subscriptions int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE merchant_id=1 AND active_chain_id IS NOT NULL`).Scan(&subscriptions); err != nil || subscriptions != 0 {
			t.Fatalf("new registration unexpectedly gained a subscription: count=%d err=%v", subscriptions, err)
		}
		var accepted int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM legal_acceptances WHERE merchant_id=1 AND revision=1 AND accepted_at IS NOT NULL`).Scan(&accepted); err != nil || accepted != 2 {
			t.Fatalf("registered merchant legal acceptances=%d err=%v, want current terms and privacy revisions", accepted, err)
		}
	})
	t.Run("V09.B.r01/v01", func(t *testing.T) {
		byUsername := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"identifier": firstUsername, "password": fixturePassword, "turnstile_token": "ok:login",
		}, "guest")
		assertIdentityResponse(t, byUsername, http.StatusOK, "username login")
		byEmail := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"identifier": firstEmail, "password": fixturePassword, "turnstile_token": "ok:login",
		}, "guest")
		assertIdentityResponse(t, byEmail, http.StatusOK, "email login")
	})

	t.Run("V07.B.r02/v01", func(t *testing.T) {
		db.Clock.Set(db.Clock.Now().Add(time.Minute))
		usernameEmail := "username-conflict@example.test"
		usernameCode := sendRegistrationCode(t, fixture, usernameEmail, "FIRST_USER")
		usernameConflict := fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("FIRST_USER", usernameEmail, usernameCode), "guest")
		assertIdentityError(t, usernameConflict, "USERNAME_TAKEN", "case-insensitive registration username conflict")

		db.Clock.Set(db.Clock.Now().Add(time.Minute))
		emailCode := sendRegistrationCode(t, fixture, strings.ToUpper(firstEmail), "another_unique_user")
		emailConflict := fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("another_unique_user", strings.ToUpper(firstEmail), emailCode), "guest")
		assertIdentityError(t, emailConflict, "EMAIL_TAKEN", "case-insensitive registration email conflict")
	})
	t.Run("V07.B.r03/v01", func(t *testing.T) {
		codeEmail := "invalid-terms@example.test"
		code := sendRegistrationCode(t, fixture, codeEmail, "invalid_terms_user")
		mismatch := registrationBody("invalid_terms_user", codeEmail, code)
		mismatch["password_confirmation"] = "different password"
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", mismatch, "guest"), "VALIDATION_ERROR", "mismatched registration password")
		missingAgreement := registrationBody("invalid_terms_user", codeEmail, code)
		missingAgreement["accept_terms"] = false
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", missingAgreement, "guest"), "AGREEMENT_REQUIRED", "registration without terms acceptance")
		oldRevision := registrationBody("invalid_terms_user", codeEmail, code)
		oldRevision["terms_revision"] = 0
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", oldRevision, "guest"), "LEGAL_VERSION_CHANGED", "registration with stale legal revision")
	})
	t.Run("V07.B.r04/v01", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			email := fmt.Sprintf("same-ip-%d@example.test", i)
			username := fmt.Sprintf("same_ip_user_%d", i)
			code := sendRegistrationCode(t, fixture, email, username)
			response := registrationRequestAt(t, fixture, "/api/v1/auth/register", registrationBody(username, email, code), "192.0.2.90:41500")
			assertIdentityResponse(t, response, http.StatusCreated, "multiple registrations from the same source IP")
		}
	})
	t.Run("V07.B.r04/v02", func(t *testing.T) {
		var count int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM merchants WHERE username LIKE 'same_ip_user_%'`).Scan(&count); err != nil || count != 3 {
			t.Fatalf("same-IP registrations=%d want=3 err=%v", count, err)
		}
	})
	t.Run("V08.B.r02/v01", func(t *testing.T) {
		email := "target-bound@example.test"
		code := sendRegistrationCode(t, fixture, email, "target_bound_user")
		wrongTarget := registrationBody("another_target_user", email, code)
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", wrongTarget, "guest"), "INVALID_CODE", "registration code bound to requested username")
		wrongEmail := registrationBody("target_bound_user", "different-target@example.test", code)
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", wrongEmail, "guest"), "INVALID_CODE", "registration code bound to target email")
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("target_bound_user", email, code), "guest"), http.StatusCreated, "registration with matching code target")
	})
	t.Run("V08.B.r02/v02", func(t *testing.T) {
		email := "code-state@example.test"
		code := sendRegistrationCode(t, fixture, email, "code_state_user")
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("code_state_user", "wrong-code-target@example.test", code), "guest"), "INVALID_CODE", "registration code for a different email")
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("code_state_user", email, code), "guest"), http.StatusCreated, "registration consumes the bound code")
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("code_state_user", email, code), "guest"), "INVALID_CODE", "registration code replay")
		wrongPurpose := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{"purpose": "reset_password", "email": firstEmail, "turnstile_token": "ok:reset_password"}, "guest")
		assertIdentityResponse(t, wrongPurpose, http.StatusOK, "issue password-reset code for purpose-isolation fixture")
		wrongPurposeCode := fixture.Mailer.LastCode()
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("wrong_purpose_user", firstEmail, wrongPurposeCode), "guest"), "INVALID_CODE", "password-reset code rejected for registration")
		expiredEmail := "expired-register-code@example.test"
		expiredCode := sendRegistrationCode(t, fixture, expiredEmail, "expired_register_code")
		db.Clock.Set(db.Clock.Now().Add(11 * time.Minute))
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("expired_register_code", expiredEmail, expiredCode), "guest"), "INVALID_CODE", "expired registration code")
	})
	t.Run("V08.B.r02/v03", func(t *testing.T) {
		email := "resend-too-soon@example.test"
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{"purpose": "register", "email": email, "target_username": "resend_too_soon", "turnstile_token": "ok:register"}, "guest"), http.StatusOK, "first email-code request")
		var sentAt time.Time
		if err := db.Pool.QueryRow(t.Context(), `SELECT sent_at FROM email_codes WHERE purpose='register' AND email_normalized=$1 ORDER BY id DESC LIMIT 1`, email).Scan(&sentAt); err != nil {
			t.Fatalf("read first email-code send time: %v", err)
		}
		db.Clock.Set(sentAt.Add(59 * time.Second))
		tooSoon := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{"purpose": "register", "email": email, "target_username": "resend_too_soon", "turnstile_token": "ok:register"}, "guest")
		assertIdentityError(t, tooSoon, "EMAIL_SEND_TOO_SOON", "email-code resend before sixty seconds")
	})
	t.Run("V08.B.r02/v04", func(t *testing.T) {
		email := "resend-replaces@example.test"
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{"purpose": "register", "email": email, "target_username": "resend_replaces", "turnstile_token": "ok:register"}, "guest"), http.StatusOK, "initial email-code request")
		oldCode := fixture.Mailer.LastCode()
		var sentAt time.Time
		if err := db.Pool.QueryRow(t.Context(), `SELECT sent_at FROM email_codes WHERE purpose='register' AND email_normalized=$1 ORDER BY id DESC LIMIT 1`, email).Scan(&sentAt); err != nil {
			t.Fatalf("read initial resend send time: %v", err)
		}
		db.Clock.Set(sentAt.Add(61 * time.Second))
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{"purpose": "register", "email": email, "target_username": "resend_replaces", "turnstile_token": "ok:register"}, "guest"), http.StatusOK, "resend after sixty seconds")
		newCode := fixture.Mailer.LastCode()
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("resend_replaces", email, oldCode), "guest"), "INVALID_CODE", "previous code invalidated by resend")
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("resend_replaces", email, newCode), "guest"), http.StatusCreated, "registration with latest email code")
	})
	t.Run("V08.B.r03/v01", func(t *testing.T) {
		email := "transaction-rollback@example.test"
		username := "registration_tx_rollback"
		code := sendRegistrationCode(t, fixture, email, username)
		if _, err := db.Pool.Exec(ctx, `CREATE FUNCTION reject_registration_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.username='registration_tx_rollback' THEN RAISE EXCEPTION 'registration fixture rejection'; END IF; RETURN NEW; END $$`); err != nil {
			t.Fatal("install local registration failure function")
		}
		if _, err := db.Pool.Exec(ctx, `CREATE TRIGGER reject_registration_fixture BEFORE INSERT ON merchants FOR EACH ROW EXECUTE FUNCTION reject_registration_fixture()`); err != nil {
			t.Fatal("install local registration failure trigger")
		}
		failed := fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody(username, email, code), "guest")
		if failed.Code == http.StatusCreated {
			t.Fatal("registration succeeded while its database transaction was fault-injected")
		}
		if _, err := db.Pool.Exec(ctx, `DROP TRIGGER reject_registration_fixture ON merchants`); err != nil {
			t.Fatal("remove local registration failure trigger")
		}
		if _, err := db.Pool.Exec(ctx, `DROP FUNCTION reject_registration_fixture()`); err != nil {
			t.Fatal("remove local registration failure trigger")
		}
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody(username, email, code), "guest"), http.StatusCreated, "retry with code preserved by rolled-back registration")
	})
	t.Run("V09.B.r02/v01", func(t *testing.T) {
		for _, tokenMode := range []struct{ mode, token string }{{"accept", "fake-login-token"}, {"replay", "ok:login"}, {"wrong-action", "wrong-action:register"}, {"wrong-host", "ok:login"}} {
			fixture.Siteverify.SetMode(tokenMode.mode)
			response := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"identifier": firstUsername, "password": fixturePassword, "turnstile_token": tokenMode.token}, "guest")
			assertIdentityError(t, response, "TURNSTILE_FAILED", "rejected login Siteverify mode "+tokenMode.mode)
		}
		fixture.Siteverify.SetMode("accept")
	})
	t.Run("V09.B.r03/v01", func(t *testing.T) {
		fixture.Siteverify.SetMode("unavailable")
		response := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"identifier": firstUsername, "password": fixturePassword, "turnstile_token": "ok:login"}, "guest")
		assertIdentityError(t, response, "TURNSTILE_UNAVAILABLE", "Siteverify transport unavailable")
		fixture.Siteverify.SetMode("accept")
	})
	t.Run("V09.B.r04/v01", func(t *testing.T) {
		for i := 0; i < 30; i++ {
			response := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"identifier": firstUsername, "password": fixtureWrongPassword, "turnstile_token": "ok:login"}, "guest")
			assertIdentityError(t, response, "INVALID_CREDENTIALS", "repeated invalid credentials")
		}
	})
	t.Run("V09.B.r04/v02", func(t *testing.T) {
		response := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"identifier": firstEmail, "password": fixturePassword, "turnstile_token": "ok:login"}, "guest")
		assertIdentityResponse(t, response, http.StatusOK, "correct login after thirty failures")
	})
	t.Run("V09.B.r05/v01", func(t *testing.T) {
		known := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{"purpose": "reset_password", "email": firstEmail, "turnstile_token": "ok:reset_password"}, "guest")
		unknown := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{"purpose": "reset_password", "email": "absent-user@example.test", "turnstile_token": "ok:reset_password"}, "guest")
		knownData := assertIdentityResponse(t, known, http.StatusOK, "password reset request for known account")
		unknownData := assertIdentityResponse(t, unknown, http.StatusOK, "password reset request for unknown account")
		var knownResult, unknownResult map[string]any
		if err := json.Unmarshal(knownData, &knownResult); err != nil {
			t.Fatal("decode known-account reset response")
		}
		if err := json.Unmarshal(unknownData, &unknownResult); err != nil {
			t.Fatal("decode unknown-account reset response")
		}
		if len(knownResult) != len(unknownResult) || knownResult["sent"] != unknownResult["sent"] || knownResult["retry_after_seconds"] != unknownResult["retry_after_seconds"] {
			t.Fatalf("password-reset email existence was disclosed: known=%#v unknown=%#v", knownResult, unknownResult)
		}
	})

	if _, err := db.Pool.Exec(ctx, `UPDATE site_settings SET security=jsonb_set(jsonb_set(security,'{restrict_email_domains}','true'::jsonb),'{email_domains}','["qq.com"]'::jsonb) WHERE singleton=true`); err != nil {
		t.Fatalf("enable exact registration email-domain allowlist: %v", err)
	}
	t.Run("V08.B.r01/v01", func(t *testing.T) {
		if _, err := db.Pool.Exec(ctx, `UPDATE site_settings SET security=jsonb_set(security,'{restrict_email_domains}','false'::jsonb) WHERE singleton=true`); err != nil {
			t.Fatal("disable email-domain restriction for the unrestricted-domain case")
		}
		arbitrary := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{
			"purpose": "register", "email": "allowed-anywhere@unusual-domain.example", "target_username": "allowed_anywhere", "turnstile_token": "ok:register",
		}, "guest")
		assertIdentityResponse(t, arbitrary, http.StatusOK, "arbitrary legal domain while restriction is disabled")
		code := fixture.Mailer.LastCode()
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("allowed_anywhere", "allowed-anywhere@unusual-domain.example", code), "guest"), http.StatusCreated, "register with arbitrary legal domain while restriction is disabled")
		if _, err := db.Pool.Exec(ctx, `UPDATE site_settings SET security=jsonb_set(security,'{restrict_email_domains}','true'::jsonb) WHERE singleton=true`); err != nil {
			t.Fatal("restore exact email-domain restriction")
		}
	})
	t.Run("V08.B.r01/v02", func(t *testing.T) {
		allowed := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{
			"purpose": "register", "email": "exact-domain@qq.com", "target_username": "domain_exact", "turnstile_token": "ok:register",
		}, "guest")
		assertIdentityResponse(t, allowed, http.StatusOK, "exact allowed email domain")
		allowedCode := fixture.Mailer.LastCode()
		accepted := fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody("domain_exact", "exact-domain@qq.com", allowedCode), "guest")
		assertIdentityResponse(t, accepted, http.StatusCreated, "registration with exact allowed qq.com domain")
	})
	t.Run("V08.B.r01/v03", func(t *testing.T) {
		for _, rejectedEmail := range []string{"subdomain@x.qq.com", "suffix@evilqq.com"} {
			rejected := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{
				"purpose": "register", "email": rejectedEmail, "target_username": "domain_rejected", "turnstile_token": "ok:register",
			}, "guest")
			assertIdentityError(t, rejected, "EMAIL_DOMAIN_DENIED", "non-exact registration email domain "+rejectedEmail)
		}
	})
}

func registrationBody(username, email, code string) map[string]any {
	return map[string]any{
		"username": username, "email": email, "email_code": code,
		"password": fixtureChangedPassword, "password_confirmation": fixtureChangedPassword,
		"accept_terms": true, "accept_privacy": true, "terms_revision": 1, "privacy_revision": 1, "turnstile_token": "ok:register",
	}
}

func sendRegistrationCode(t *testing.T, fixture *testsupport.HTTPFixture, email, username string) string {
	t.Helper()
	response := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{
		"purpose": "register", "email": email, "target_username": username, "turnstile_token": "ok:register",
	}, "guest")
	assertIdentityResponse(t, response, http.StatusOK, "registration code for "+email)
	code := fixture.Mailer.LastCode()
	if len(code) != 6 {
		t.Fatalf("registration email code for %s = %q", email, code)
	}
	return code
}

func registrationRequestAt(t *testing.T, fixture *testsupport.HTTPFixture, path string, body any, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal("encode registration HTTP request")
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return fixture.RequestWithRemoteAddr(t, http.MethodPost, path, strings.NewReader(string(encoded)), "guest", headers, remoteAddr)
}
