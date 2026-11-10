package identity

import (
	"net/http"
	"testing"

	"vanillapay/internal/auth"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_TurnstileDisabledAllowsAnonymousAuthFlows(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	if _, err := db.Pool.Exec(t.Context(), `INSERT INTO legal_versions(kind,revision,content) VALUES('terms',1,'{"zh_CN":"terms","en":"terms"}'),('privacy',1,'{"zh_CN":"privacy","en":"privacy"}')`); err != nil {
		t.Fatal(err)
	}
	const email = "without-widget@example.test"
	const username = "without_widget"
	codePath := "/api/v1/auth/email-codes"
	registerPath := "/api/v1/auth/register"
	resetPath := "/api/v1/auth/reset-password"
	loginPath := "/api/v1/auth/login"
	code := auth.EmailCodeRequest{Purpose: "register", Email: email, TargetUsername: username}
	assertIdentityResponse(t, fixture.Request(t, http.MethodPost, codePath, code, "guest"), http.StatusOK, "send registration code without token")
	register := registrationBody(username, email, fixture.Mailer.LastCode())
	delete(register, "turnstile_token")
	assertIdentityResponse(t, fixture.Request(t, http.MethodPost, registerPath, register, "guest"), http.StatusCreated, "register without token")
	login := auth.LoginRequest{Identifier: username, Password: fixtureChangedPassword}
	assertIdentityResponse(t, fixture.Request(t, http.MethodPost, loginPath, login, "guest"), http.StatusOK, "login without token")
	code = auth.EmailCodeRequest{Purpose: "reset_password", Email: email}
	assertIdentityResponse(t, fixture.Request(t, http.MethodPost, codePath, code, "guest"), http.StatusOK, "send reset code without token")
	reset := auth.ResetPasswordRequest{Email: email, EmailCode: fixture.Mailer.LastCode(), Password: fixturePassword, PasswordConfirmation: fixturePassword}
	if response := fixture.Request(t, http.MethodPost, resetPath, reset, "guest"); response.Code != http.StatusOK {
		t.Fatalf("reset password without token: status=%d", response.Code)
	}
	login.Password = fixturePassword
	login.TurnstileToken = "ignored-invalid-token"
	assertIdentityResponse(t, fixture.Request(t, http.MethodPost, loginPath, login, "guest"), http.StatusOK, "disabled widget ignores supplied token")
	if len(fixture.Siteverify.Requests()) != 0 {
		t.Fatal("disabled Turnstile called Siteverify")
	}
	if err := configureTOTPTestSiteverify(t, fixture, t.Context()); err != nil {
		t.Fatal(err)
	}
	login.TurnstileToken = ""
	for _, attempt := range []struct {
		path string
		body any
	}{{loginPath, login}, {codePath, code}, {registerPath, register}, {resetPath, reset}} {
		assertIdentityError(t, fixture.Request(t, http.MethodPost, attempt.path, attempt.body, "guest"), "TURNSTILE_FAILED", "enabled Turnstile requires proof")
	}
	fixture.Siteverify.SetMode("wrong-host")
	login.TurnstileToken = "ok:login"
	assertIdentityError(t, fixture.Request(t, http.MethodPost, loginPath, login, "guest"), "TURNSTILE_FAILED", "reject proof with mismatched hostname")
}
