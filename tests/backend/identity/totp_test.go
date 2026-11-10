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

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_TOTPStateTransitions(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	ctx := context.Background()
	var totpSecrets, recoveryCodes []string

	bootstrapCode, err := fixture.App.Services.Auth.RotateBootstrapCode(ctx)
	if err != nil {
		t.Fatal("prepare admin bootstrap code")
	}
	adminPassword := fixturePassword
	bootstrap := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", map[string]any{
		"initialization_code": bootstrapCode, "username": "totp_admin", "password": adminPassword, "password_confirmation": adminPassword,
	}, "admin:totp-admin")
	assertIdentityResponse(t, bootstrap, http.StatusCreated, "admin bootstrap")

	merchantID, _ := db.SeedMerchant(t)
	var merchantUsername string
	if err := db.Pool.QueryRow(ctx, `SELECT username FROM merchants WHERE id=$1`, merchantID).Scan(&merchantUsername); err != nil {
		t.Fatal("read merchant fixture username")
	}
	merchantPassword := fixturePassword
	merchantHash, err := auth.HashPassword(merchantPassword, nil)
	if err != nil {
		t.Fatal("hash merchant fixture password")
	}
	if _, err := db.Pool.Exec(ctx, `UPDATE merchants SET password_hash=$1 WHERE id=$2`, merchantHash, merchantID); err != nil {
		t.Fatal("set merchant fixture password")
	}
	if err := configureTOTPTestSiteverify(t, fixture, ctx); err != nil {
		t.Fatal(err)
	}

	merchantLogin := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"identifier": merchantUsername, "password": merchantPassword, "turnstile_token": "ok:login",
	}, "merchant:totp-merchant")
	assertIdentityResponse(t, merchantLogin, http.StatusOK, "merchant login before TOTP setup")

	actors := []struct {
		name, role, path, client, password, identifier string
	}{
		{"admin", "admin", "/api/v1/console/2fa", "admin:totp-admin", adminPassword, "totp_admin"},
		{"merchant", "merchant", "/api/v1/merchant/2fa", "merchant:totp-merchant", merchantPassword, merchantUsername},
	}
	for _, actor := range actors {

		t.Run(actor.name, func(t *testing.T) {
			setup := fixture.Request(t, http.MethodPost, actor.path+"/setup", map[string]any{"password": actor.password}, actor.client)
			setupData := assertIdentityResponse(t, setup, http.StatusOK, actor.name+" TOTP setup")
			var setupDTO struct {
				SetupID string `json:"setup_id"`
				Secret  string `json:"secret"`
			}
			if err := json.Unmarshal(setupData, &setupDTO); err != nil || setupDTO.SetupID == "" || setupDTO.Secret == "" {
				t.Fatalf("%s setup DTO = %s, err=%v", actor.name, setupData, err)
			}
			totpSecrets = append(totpSecrets, setupDTO.Secret)
			t.Run("V11.B.r01/v01", func(t *testing.T) {
				var cipher []byte
				query := `SELECT totp_cipher FROM admins WHERE id=1`
				if actor.role == "merchant" {
					query = `SELECT totp_cipher FROM merchants WHERE id=$1`
					if err := db.Pool.QueryRow(ctx, query, merchantID).Scan(&cipher); err != nil {
						t.Fatal("read pending merchant TOTP state")
					}
				} else if err := db.Pool.QueryRow(ctx, query).Scan(&cipher); err != nil {
					t.Fatal("read pending admin TOTP state")
				}
				if len(cipher) != 0 {
					t.Fatal("TOTP setup enabled the account before confirmation")
				}
			})

			var confirmation struct {
				Enabled       bool     `json:"enabled"`
				RecoveryCodes []string `json:"recovery_codes"`
			}
			t.Run("V11.B.r02/v01", func(t *testing.T) {
				confirmCode := totpCode(t, setupDTO.Secret, db.Clock.Now())
				confirm := fixture.Request(t, http.MethodPost, actor.path+"/confirm", map[string]any{"setup_id": setupDTO.SetupID, "totp_code": confirmCode}, actor.client)
				confirmData := assertIdentityResponse(t, confirm, http.StatusOK, actor.name+" TOTP confirmation")
				if err := json.Unmarshal(confirmData, &confirmation); err != nil || !confirmation.Enabled || len(confirmation.RecoveryCodes) != 10 {
					t.Fatalf("%s TOTP confirmation DTO = %s, err=%v", actor.name, confirmData, err)
				}
				recoveryCodes = append(recoveryCodes, confirmation.RecoveryCodes...)
				for _, code := range confirmation.RecoveryCodes {
					if code == "" {
						t.Fatal("TOTP confirmation returned an empty recovery code")
					}
				}
			})

			var challenge struct {
				ChallengeID string `json:"challenge_id"`
				Requires2FA bool   `json:"requires_2fa"`
			}
			t.Run("V11.B.r03/v01", func(t *testing.T) {
				login := fixture.Request(t, http.MethodPost, loginPath(actor.role), map[string]any{
					"identifier": actor.identifier, "password": actor.password, "turnstile_token": "ok:login",
				}, actor.role+":totp-challenge-"+actor.name)
				loginData := assertIdentityResponse(t, login, http.StatusOK, actor.name+" password challenge")
				if err := json.Unmarshal(loginData, &challenge); err != nil || !challenge.Requires2FA || challenge.ChallengeID == "" {
					t.Fatalf("%s TOTP login challenge = %s, err=%v", actor.name, loginData, err)
				}
				if actor.role == "admin" {
					var active bool
					if err := db.Pool.QueryRow(ctx, `SELECT totp_cipher IS NOT NULL FROM admins WHERE id=1`).Scan(&active); err != nil || !active {
						t.Fatalf("admin TOTP was not persisted before login: active=%t err=%v", active, err)
					}
				} else {
					var active bool
					if err := db.Pool.QueryRow(ctx, `SELECT totp_cipher IS NOT NULL FROM merchants WHERE id=$1`, merchantID).Scan(&active); err != nil || !active {
						t.Fatalf("merchant TOTP was not persisted before login: active=%t err=%v", active, err)
					}
				}
			})

			db.Clock.Set(db.Clock.Now().Add(30 * time.Second))
			t.Run("V11.B.r03/v02", func(t *testing.T) {
				totpLogin := fixture.Request(t, http.MethodPost, secondFactorPath(actor.role), map[string]any{
					"challenge_id": challenge.ChallengeID, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now()),
				}, actor.role+":totp-recovery-"+actor.name)
				assertIdentityResponse(t, totpLogin, http.StatusOK, actor.name+" TOTP login")
			})

			var replayChallenge struct {
				ChallengeID string `json:"challenge_id"`
			}
			t.Run("V11.B.r04/v01", func(t *testing.T) {
				replayChallengeRecorder := fixture.Request(t, http.MethodPost, loginPath(actor.role), map[string]any{
					"identifier": actor.identifier, "password": actor.password, "turnstile_token": "ok:login",
				}, actor.role+":totp-replay-"+actor.name)
				replayChallengeData := assertIdentityResponse(t, replayChallengeRecorder, http.StatusOK, actor.name+" replay challenge")
				if err := json.Unmarshal(replayChallengeData, &replayChallenge); err != nil || replayChallenge.ChallengeID == "" {
					t.Fatalf("%s replay challenge DTO = %s, err=%v", actor.name, replayChallengeData, err)
				}
				replayedTOTP := fixture.Request(t, http.MethodPost, secondFactorPath(actor.role), map[string]any{
					"challenge_id": replayChallenge.ChallengeID, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now()),
				}, "guest")
				assertIdentityError(t, replayedTOTP, "INVALID_2FA", actor.name+" same-step TOTP replay")
			})

			var subjectID int64
			if actor.role == "admin" {
				subjectID = 1
			} else {
				subjectID = merchantID
			}
			t.Run("V11.B.r05/v01", func(t *testing.T) {
				recoveryLogin := fixture.Request(t, http.MethodPost, secondFactorPath(actor.role), map[string]any{
					"challenge_id": replayChallenge.ChallengeID, "recovery_code": confirmation.RecoveryCodes[0],
				}, actor.role+":totp-recovery-"+actor.name)
				assertIdentityResponse(t, recoveryLogin, http.StatusOK, actor.name+" recovery-code login")
				var sessions int
				if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE role=$1 AND subject_id=$2 AND revoked_at IS NULL`, actor.role, func() int64 {
					if actor.role == "admin" {
						return 1
					}
					return merchantID
				}()).Scan(&sessions); err != nil || sessions < 3 {
					t.Fatalf("%s second-factor login did not persist a session: sessions=%d err=%v", actor.name, sessions, err)
				}
				var usedCount int
				if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM recovery_codes WHERE role=$1 AND subject_id=$2 AND code_hash=$3 AND used_at IS NOT NULL`, actor.role, subjectID, auth.HashOpaque(confirmation.RecoveryCodes[0])).Scan(&usedCount); err != nil || usedCount != 1 {
					t.Fatalf("%s recovery code was not consumed once: count=%d err=%v", actor.name, usedCount, err)
				}

				secondChallenge := fixture.Request(t, http.MethodPost, loginPath(actor.role), map[string]any{
					"identifier": actor.identifier, "password": actor.password, "turnstile_token": "ok:login",
				}, actor.role+":totp-replay-"+actor.name)
				secondChallengeData := assertIdentityResponse(t, secondChallenge, http.StatusOK, actor.name+" replay challenge")
				var recoveryReplayChallenge struct {
					ChallengeID string `json:"challenge_id"`
				}
				if err := json.Unmarshal(secondChallengeData, &recoveryReplayChallenge); err != nil || recoveryReplayChallenge.ChallengeID == "" {
					t.Fatalf("%s replay challenge DTO = %s, err=%v", actor.name, secondChallengeData, err)
				}
				replay := fixture.Request(t, http.MethodPost, secondFactorPath(actor.role), map[string]any{"challenge_id": recoveryReplayChallenge.ChallengeID, "recovery_code": confirmation.RecoveryCodes[0]}, "guest")
				assertIdentityError(t, replay, "INVALID_2FA", actor.name+" one-use recovery replay")
			})

			t.Run("V11.B.r04/v02", func(t *testing.T) {
				expiredLogin := fixture.Request(t, http.MethodPost, loginPath(actor.role), map[string]any{
					"identifier": actor.identifier, "password": actor.password, "turnstile_token": "ok:login",
				}, actor.role+":totp-expired-"+actor.name)
				expiredData := assertIdentityResponse(t, expiredLogin, http.StatusOK, actor.name+" expiring challenge")
				var expiredChallenge struct {
					ChallengeID string `json:"challenge_id"`
				}
				if err := json.Unmarshal(expiredData, &expiredChallenge); err != nil || expiredChallenge.ChallengeID == "" {
					t.Fatalf("%s expiring challenge DTO = %s, err=%v", actor.name, expiredData, err)
				}
				db.Clock.Set(db.Clock.Now().Add(6 * time.Minute))
				expired := fixture.Request(t, http.MethodPost, secondFactorPath(actor.role), map[string]any{
					"challenge_id": expiredChallenge.ChallengeID, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now()),
				}, "guest")
				assertIdentityError(t, expired, "CHALLENGE_EXPIRED", actor.name+" expired 2FA challenge")
			})

			t.Run("V11.B.r06/v02", func(t *testing.T) {
				client := actor.role + ":totp-recovery-" + actor.name
				body := map[string]any{"totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now())}
				missing := fixture.Request(t, http.MethodDelete, actor.path, body, client)
				assertIdentityError(t, missing, "VALIDATION_ERROR", actor.name+" disable with missing password field")
				body["password"] = ""
				withoutFreshProof := fixture.Request(t, http.MethodDelete, actor.path, body, client)
				assertIdentityError(t, withoutFreshProof, "INVALID_CREDENTIALS", actor.name+" disable without password proof")
			})

			t.Run("V11.B.r06/v01", func(t *testing.T) {
				disable := fixture.Request(t, http.MethodDelete, actor.path, map[string]any{"password": actor.password, "totp_code": totpCode(t, setupDTO.Secret, db.Clock.Now())}, actor.role+":totp-recovery-"+actor.name)
				assertIdentityResponse(t, disable, http.StatusOK, actor.name+" TOTP disable")
				var enabled bool
				if actor.role == "admin" {
					if err := db.Pool.QueryRow(ctx, `SELECT totp_cipher IS NOT NULL FROM admins WHERE id=1`).Scan(&enabled); err != nil {
						t.Fatal("read disabled admin TOTP state")
					}
				} else {
					if err := db.Pool.QueryRow(ctx, `SELECT totp_cipher IS NOT NULL FROM merchants WHERE id=$1`, merchantID).Scan(&enabled); err != nil {
						t.Fatal("read disabled merchant TOTP state")
					}
				}
				if enabled {
					t.Fatalf("%s TOTP secret remained enabled after authenticated disable", actor.name)
				}
				var recoveryCount int
				if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM recovery_codes WHERE role=$1 AND subject_id=$2`, actor.role, subjectID).Scan(&recoveryCount); err != nil || recoveryCount != 0 {
					t.Fatalf("%s recovery codes remained after TOTP disable: count=%d err=%v", actor.name, recoveryCount, err)
				}
			})
			t.Run("V11.B.r07/v01", func(t *testing.T) {
				var enabled bool
				if actor.role == "admin" {
					if err := db.Pool.QueryRow(ctx, `SELECT totp_cipher IS NOT NULL FROM admins WHERE id=1`).Scan(&enabled); err != nil {
						t.Fatal("read administrator TOTP state")
					}
				} else if err := db.Pool.QueryRow(ctx, `SELECT totp_cipher IS NOT NULL FROM merchants WHERE id=$1`, merchantID).Scan(&enabled); err != nil {
					t.Fatal("read merchant TOTP state")
				}
				if enabled {
					t.Fatalf("%s TOTP flow did not complete authenticated disable", actor.name)
				}
			})
		})
	}
	t.Run("V16.B.r01/v08", func(t *testing.T) {
		rows, err := db.Pool.Query(ctx, `SELECT actor_role,actor_id::text,COALESCE(merchant_id::text,''),action,COALESCE(ip::text,''),request_id,safe_metadata FROM audit_logs WHERE action=ANY($1::text[]) ORDER BY actor_role,actor_id,action`, []string{"auth.totp_setup", "auth.totp_confirm", "auth.totp_disable"})
		if err != nil {
			t.Fatal("query TOTP lifecycle audit rows")
		}
		defer rows.Close()
		counts := map[string]int{}
		for rows.Next() {
			var role, actorID, merchantAuditID, action, ip, requestID string
			var metadata json.RawMessage
			if err := rows.Scan(&role, &actorID, &merchantAuditID, &action, &ip, &requestID, &metadata); err != nil {
				t.Fatal("scan TOTP lifecycle audit row")
			}
			if requestID == "" || ip != "192.0.2.44/32" {
				t.Fatalf("TOTP audit source metadata role=%s action=%s ip=%q request_id=%q", role, action, ip, requestID)
			}
			if role == "admin" && (actorID != "1" || merchantAuditID != "") {
				t.Fatalf("administrator TOTP audit actor/merchant attribution = %q/%q", actorID, merchantAuditID)
			}
			if role == "merchant" && (actorID != fmt.Sprint(merchantID) || merchantAuditID != fmt.Sprint(merchantID)) {
				t.Fatalf("merchant TOTP audit actor/merchant attribution = %q/%q", actorID, merchantAuditID)
			}
			for _, secret := range append(append([]string{}, totpSecrets...), append([]string{adminPassword, merchantPassword}, recoveryCodes...)...) {
				if strings.Contains(string(metadata), secret) {
					t.Fatalf("TOTP audit metadata contains sensitive value %q", secret)
				}
			}
			counts[role+":"+action]++
		}
		if err := rows.Err(); err != nil {
			t.Fatal("iterate TOTP lifecycle audit rows")
		}
		for _, role := range []string{"admin", "merchant"} {
			for _, action := range []string{"auth.totp_setup", "auth.totp_confirm", "auth.totp_disable"} {
				if counts[role+":"+action] != 1 {
					t.Fatalf("TOTP audit count %s/%s=%d want=1", role, action, counts[role+":"+action])
				}
			}
		}
	})
}

func configureTOTPTestSiteverify(t *testing.T, fixture *testsupport.HTTPFixture, ctx context.Context) error {
	t.Helper()
	var version int64
	var current json.RawMessage
	if err := fixture.DB.Pool.QueryRow(ctx, `SELECT version,security FROM site_settings WHERE singleton=true`).Scan(&version, &current); err != nil {
		return err
	}
	var security map[string]json.RawMessage
	if err := json.Unmarshal(current, &security); err != nil {
		return err
	}
	value, err := json.Marshal(map[string]any{
		"turnstile_site_key": "totp-site-key", "turnstile_secret": "totp-site-secret", "turnstile_enabled": true, "turnstile_token": "ok:settings_security",
		"restrict_email_domains": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "support@example.test", "en": "support@example.test"},
	})
	if err != nil {
		return err
	}
	if _, err := fixture.App.Services.Settings.UpdateGroup(ctx, platform.Actor{Role: "admin", SubjectID: 1}, "security", settings.UpdateGroup{ExpectedVersion: version, Value: value}); err != nil {
		return err
	}
	fixture.Siteverify.SetMode("accept")
	return nil
}

func assertIdentityResponse(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, label string) json.RawMessage {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("%s status=%d want=%d body=%s", label, response.Code, wantStatus, response.Body.String())
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		t.Fatalf("%s response envelope=%s err=%v", label, response.Body.String(), err)
	}
	return envelope.Data
}

func assertIdentityError(t *testing.T, response *httptest.ResponseRecorder, code, label string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code == http.StatusOK || envelope.Error.Code != code {
		t.Fatalf("%s status=%d error=%q body=%s err=%v", label, response.Code, envelope.Error.Code, response.Body.String(), err)
	}
}

func totpCode(t *testing.T, secret string, now time.Time) string {
	t.Helper()
	code, err := totp.GenerateCodeCustom(secret, now, totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal("generate independent current TOTP code")
	}
	return code
}

func loginPath(role string) string {
	if role == "admin" {
		return "/api/v1/console/login"
	}
	return "/api/v1/auth/login"
}

func secondFactorPath(role string) string {
	if role == "admin" {
		return "/api/v1/console/second-factor"
	}
	return "/api/v1/auth/second-factor"
}
