package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_ProfileSecurity(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	ctx := context.Background()
	merchantID, _ := db.SeedMerchant(t)
	otherID, _ := db.SeedMerchant(t)
	password := fixturePassword
	passwordHash, err := auth.HashPassword(password, nil)
	if err != nil {
		t.Fatal("hash profile fixture password")
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE merchants SET password_hash=$1 WHERE id=$2`, passwordHash, merchantID); err != nil {
		t.Fatal("set profile fixture password")
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE merchants SET username='profile_taken',username_normalized='profile_taken' WHERE id=$1`, otherID); err != nil {
		t.Fatal("prepare unique username conflict fixture")
	}
	var username, email string
	if err := db.Pool.QueryRow(ctx, `SELECT username,email FROM merchants WHERE id=$1`, merchantID).Scan(&username, &email); err != nil {
		t.Fatal("read profile fixture identity")
	}
	if err := configureTOTPTestSiteverify(t, fixture, ctx); err != nil {
		t.Fatalf("configure local Siteverify: %v", err)
	}
	client := "merchant:profile-security"
	login := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"identifier": username, "password": password, "turnstile_token": "ok:login",
	}, client)
	assertIdentityResponse(t, login, http.StatusOK, "profile fixture login")

	setup := fixture.Request(t, http.MethodPost, "/api/v1/merchant/2fa/setup", map[string]any{"password": password}, client)
	setupData := assertIdentityResponse(t, setup, http.StatusOK, "profile fixture TOTP setup")
	var setupDTO struct {
		SetupID string `json:"setup_id"`
		Secret  string `json:"secret"`
	}
	if err := json.Unmarshal(setupData, &setupDTO); err != nil || setupDTO.SetupID == "" || setupDTO.Secret == "" {
		t.Fatalf("profile TOTP setup response = %s, err=%v", setupData, err)
	}
	confirm := fixture.Request(t, http.MethodPost, "/api/v1/merchant/2fa/confirm", map[string]any{
		"setup_id": setupDTO.SetupID, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now()),
	}, client)
	assertIdentityResponse(t, confirm, http.StatusOK, "profile fixture TOTP confirmation")

	conflictUsernameCode := sendProfileEmailCode(t, fixture, client, "change_username", email, "profile_taken", "")
	conflictNewEmail := "atomicity-new@example.test"
	conflictOldEmailCode := sendProfileEmailCode(t, fixture, client, "change_email_old", email, "", email)
	conflictNewEmailCode := sendProfileEmailCode(t, fixture, client, "change_email_new", conflictNewEmail, "", conflictNewEmail)
	db.Clock.Set(db.Clock.Now().Add(30 * time.Second))
	t.Run("V10.B.r05/v01", func(t *testing.T) {
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		_, err := fixture.App.Services.Auth.ChangeIdentity(ctx, actor, auth.IdentityChange{
			Username: "profile_taken", Email: conflictNewEmail, EmailCode: conflictUsernameCode,
			OldEmailCode: conflictOldEmailCode, NewEmailCode: conflictNewEmailCode, TOTPCode: totpCode(t, setupDTO.Secret, db.Clock.Now()),
		})
		if identityProfileErrorCode(err) != "USERNAME_TAKEN" {
			t.Fatalf("conflicting multi-field profile update error = %v", err)
		}
		var gotUsername, gotEmail string
		if err := db.Pool.QueryRow(ctx, `SELECT username,email FROM merchants WHERE id=$1`, merchantID).Scan(&gotUsername, &gotEmail); err != nil {
			t.Fatal("read profile after unique conflict")
		}
		if gotUsername != username || gotEmail != email {
			t.Fatalf("unique conflict left a partial profile update: username=%q email=%q", gotUsername, gotEmail)
		}
		for _, code := range []string{conflictUsernameCode, conflictOldEmailCode, conflictNewEmailCode} {
			var available int
			if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM email_codes WHERE code_hash=$1 AND consumed_at IS NULL`, auth.HashOpaque(code)).Scan(&available); err != nil || available != 1 {
				t.Fatalf("profile proof was consumed despite transaction rollback: available=%d err=%v", available, err)
			}
		}
	})

	db.Clock.Set(db.Clock.Now().Add(time.Minute))
	usernameCode := sendProfileEmailCode(t, fixture, client, "change_username", email, "profile_renamed", "")
	db.Clock.Set(db.Clock.Now().Add(30 * time.Second))
	t.Run("V10.B.r04/v01", func(t *testing.T) {
		wrongTarget := fixture.Request(t, http.MethodPatch, "/api/v1/merchant/profile/username", map[string]any{
			"username": "profile_unrequested", "security_proof": map[string]any{"email_code": usernameCode, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now())},
		}, client)
		assertIdentityError(t, wrongTarget, "INVALID_CODE", "username code bound to requested target")
	})
	t.Run("V10.B.r01/v01", func(t *testing.T) {
		changed := fixture.Request(t, http.MethodPatch, "/api/v1/merchant/profile/username", map[string]any{
			"username": "profile_renamed", "security_proof": map[string]any{"email_code": usernameCode, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now())},
		}, client)
		data := assertIdentityResponse(t, changed, http.StatusOK, "username change with current email proof")
		var account struct {
			Username string `json:"username"`
		}
		if err := json.Unmarshal(data, &account); err != nil || account.Username != "profile_renamed" {
			t.Fatalf("changed username response=%s err=%v", data, err)
		}
	})

	newEmail := "profile-renamed@example.test"
	db.Clock.Set(db.Clock.Now().Add(30 * time.Second))
	oldEmailCode := sendProfileEmailCode(t, fixture, client, "change_email_old", email, "", email)
	newEmailCode := sendProfileEmailCode(t, fixture, client, "change_email_new", newEmail, "", newEmail)
	db.Clock.Set(db.Clock.Now().Add(30 * time.Second))
	t.Run("V10.B.r02/v01", func(t *testing.T) {
		missing := fixture.Request(t, http.MethodPatch, "/api/v1/merchant/profile/email", map[string]any{
			"email": newEmail, "old_email_code": oldEmailCode, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now()),
		}, client)
		assertIdentityError(t, missing, "INVALID_CODE", "email change requires both email proofs")
	})
	t.Run("V10.B.r02/v02", func(t *testing.T) {
		swapped := fixture.Request(t, http.MethodPatch, "/api/v1/merchant/profile/email", map[string]any{
			"email": newEmail, "old_email_code": newEmailCode, "new_email_code": oldEmailCode, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now()),
		}, client)
		assertIdentityError(t, swapped, "INVALID_CODE", "email proofs are bound to their original and target addresses")
		changed := fixture.Request(t, http.MethodPatch, "/api/v1/merchant/profile/email", map[string]any{
			"email": newEmail, "old_email_code": oldEmailCode, "new_email_code": newEmailCode, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now()),
		}, client)
		data := assertIdentityResponse(t, changed, http.StatusOK, "email change with both bound proofs")
		var account struct {
			Email string `json:"email"`
		}
		if err := json.Unmarshal(data, &account); err != nil || account.Email != newEmail {
			t.Fatalf("changed email response=%s err=%v", data, err)
		}
	})

	db.Clock.Set(db.Clock.Now().Add(30 * time.Second))
	passwordCode := sendProfileEmailCode(t, fixture, client, "change_password", newEmail, "", newEmail)
	newPassword := fixtureChangedPassword
	t.Run("V10.B.r03/v01", func(t *testing.T) {
		changed := fixture.Request(t, http.MethodPatch, "/api/v1/merchant/profile/password", map[string]any{
			"password": newPassword, "password_confirmation": newPassword,
			"security_proof": map[string]any{"email_code": passwordCode, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now())},
		}, client)
		assertProfileSuccess(t, changed, "password change with proof")
		var liveSessions int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE role='merchant' AND subject_id=$1 AND revoked_at IS NULL`, merchantID).Scan(&liveSessions); err != nil || liveSessions != 0 {
			t.Fatalf("password change left live sessions=%d err=%v", liveSessions, err)
		}
		oldLogin := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"identifier": newEmail, "password": password, "turnstile_token": "ok:login",
		}, "guest")
		assertIdentityError(t, oldLogin, "INVALID_CREDENTIALS", "old password after password change")
		newLogin := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"identifier": newEmail, "password": newPassword, "turnstile_token": "ok:login",
		}, "guest")
		data := assertIdentityResponse(t, newLogin, http.StatusOK, "new password after password change")
		var challenge struct {
			Requires2FA bool `json:"requires_2fa"`
		}
		if err := json.Unmarshal(data, &challenge); err != nil || !challenge.Requires2FA {
			t.Fatalf("new password login did not retain TOTP requirement: %s err=%v", data, err)
		}
	})
	t.Run("V10.B.r03/v02", func(t *testing.T) {
		var enabled bool
		if err := db.Pool.QueryRow(ctx, `SELECT totp_cipher IS NOT NULL FROM merchants WHERE id=$1`, merchantID).Scan(&enabled); err != nil || !enabled {
			t.Fatalf("password change disabled TOTP: enabled=%t err=%v", enabled, err)
		}
	})
}

func identityProfileErrorCode(err error) string {
	var apiErr *platform.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code
	}
	return ""
}

func assertProfileSuccess(t *testing.T, response *httptest.ResponseRecorder, label string) {
	t.Helper()
	var envelope struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusOK || envelope.Error != nil {
		t.Fatalf("%s status=%d body=%s err=%v", label, response.Code, response.Body.String(), err)
	}
}

func sendProfileEmailCode(t *testing.T, fixture *testsupport.HTTPFixture, client, purpose, email, targetUsername, targetEmail string) string {
	t.Helper()
	response := fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{
		"purpose": purpose, "email": email, "target_username": targetUsername, "target_email": targetEmail,
	}, client)
	assertIdentityResponse(t, response, http.StatusOK, "send "+purpose+" email code")
	code := fixture.Mailer.LastCode()
	if len(code) != 6 {
		t.Fatalf("mailer did not capture a six-digit %s code", purpose)
	}
	return code
}
