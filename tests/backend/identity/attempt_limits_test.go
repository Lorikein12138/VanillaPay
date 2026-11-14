package identity

import (
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_EmailCodeAttemptLimitSurvivesRollback(t *testing.T) {
	for _, wrongAttempts := range []int{4, 5} {
		t.Run(fmt.Sprintf("wrong_attempts_%d", wrongAttempts), func(t *testing.T) {
			db := testsupport.NewDatabase(t)
			fixture := testsupport.NewHTTPFixture(t, db)
			ctx := t.Context()
			if err := configureTOTPTestSiteverify(t, fixture, ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Pool.Exec(ctx, `INSERT INTO legal_versions(kind,revision,content) VALUES('terms',1,'{"zh_CN":"terms","en":"terms"}'),('privacy',1,'{"zh_CN":"privacy","en":"privacy"}')`); err != nil {
				t.Fatal(err)
			}
			service := fixture.App.Services.Auth
			const email = "attempt-limit@example.test"
			const username = "attempt_limit"
			if _, err := service.SendEmailCode(ctx, platform.Actor{}, auth.EmailCodeRequest{Purpose: "register", Email: email, TargetUsername: username, TurnstileToken: "ok:register"}, "zh-CN"); err != nil {
				t.Fatal(err)
			}
			correct := fixture.Mailer.LastCode()
			if len(correct) != 6 {
				t.Fatal("registration mail did not contain a verification code")
			}
			wrong := "000000"
			if correct == wrong {
				wrong = "000001"
			}
			password := "Aa1!" + rand.Text()
			request := auth.RegisterRequest{Username: username, Email: email, EmailCode: wrong, Password: password, PasswordConfirmation: password, AcceptTerms: true, AcceptPrivacy: true, TermsRevision: 1, PrivacyRevision: 1, TurnstileToken: "ok:register"}
			for attempt := 1; attempt <= wrongAttempts; attempt++ {
				_, _, err := service.Register(ctx, request)
				assertAttemptError(t, err, "INVALID_CODE")
				var failures, merchants int
				var consumed *time.Time
				if err := db.Pool.QueryRow(ctx, `SELECT failed_attempts,consumed_at,(SELECT count(*) FROM merchants) FROM email_codes WHERE email_normalized=$1`, email).Scan(&failures, &consumed, &merchants); err != nil {
					t.Fatal(err)
				}
				if failures != attempt || consumed != nil || merchants != 0 {
					t.Fatalf("failed registration rollback: attempts=%d want=%d consumed=%t merchants=%d", failures, attempt, consumed != nil, merchants)
				}
			}
			request.EmailCode = correct
			_, _, err := service.Register(ctx, request)
			if wrongAttempts == 4 {
				if err != nil {
					t.Fatalf("correct fifth attempt must succeed: %v", err)
				}
			} else {
				assertAttemptError(t, err, "INVALID_CODE")
				var failures int
				if err := db.Pool.QueryRow(ctx, `SELECT failed_attempts FROM email_codes WHERE email_normalized=$1`, email).Scan(&failures); err != nil || failures != 5 {
					t.Fatalf("exhausted email code attempts=%d want=5 err=%v", failures, err)
				}
			}
		})
	}
}

func TestDB_LoginChallengeAttemptLimitSurvivesRollback(t *testing.T) {
	for _, wrongAttempts := range []int{4, 5} {
		t.Run(fmt.Sprintf("wrong_attempts_%d", wrongAttempts), func(t *testing.T) {
			db := testsupport.NewDatabase(t)
			merchantID, _ := db.SeedMerchant(t)
			fixture := testsupport.NewHTTPFixture(t, db)
			ctx := t.Context()
			if err := configureTOTPTestSiteverify(t, fixture, ctx); err != nil {
				t.Fatal(err)
			}
			password := "Aa1!" + rand.Text()
			hash, err := auth.HashPassword(password, nil)
			if err != nil {
				t.Fatal(err)
			}
			var username string
			if err := db.Pool.QueryRow(ctx, `UPDATE merchants SET password_hash=$1 WHERE id=$2 RETURNING username`, hash, merchantID).Scan(&username); err != nil {
				t.Fatal(err)
			}
			service := fixture.App.Services.Auth
			actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
			setup, err := service.SetupTOTP(ctx, actor, password)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.ConfirmTOTP(ctx, actor, auth.TOTPConfirm{SetupID: setup.SetupID, TOTPCode: totpCode(t, setup.Secret, db.Clock.Now())}); err != nil {
				t.Fatal(err)
			}
			db.Clock.Set(db.Clock.Now().Add(time.Minute))
			login, err := service.Login(ctx, "merchant", auth.LoginRequest{Identifier: username, Password: password, TurnstileToken: "ok:login"})
			if err != nil || login.ChallengeID == "" {
				t.Fatalf("create login challenge: %v", err)
			}
			for attempt := 1; attempt <= wrongAttempts; attempt++ {
				_, err := service.VerifySecondFactor(ctx, auth.FactorRequest{ChallengeID: login.ChallengeID, TOTPCode: "invalid"})
				assertAttemptError(t, err, "INVALID_2FA")
				var failures, sessions int
				var consumed *time.Time
				if err := db.Pool.QueryRow(ctx, `SELECT failed_attempts,consumed_at,(SELECT count(*) FROM sessions) FROM auth_challenges WHERE challenge_hash=$1`, auth.HashOpaque(login.ChallengeID)).Scan(&failures, &consumed, &sessions); err != nil {
					t.Fatal(err)
				}
				if failures != attempt || consumed != nil || sessions != 0 {
					t.Fatalf("failed challenge rollback: attempts=%d want=%d consumed=%t sessions=%d", failures, attempt, consumed != nil, sessions)
				}
			}
			_, err = service.VerifySecondFactor(ctx, auth.FactorRequest{ChallengeID: login.ChallengeID, TOTPCode: totpCode(t, setup.Secret, db.Clock.Now())})
			if wrongAttempts == 4 {
				if err != nil {
					t.Fatalf("correct fifth attempt must succeed: %v", err)
				}
			} else {
				assertAttemptError(t, err, "CHALLENGE_EXPIRED")
				var failures int
				if err := db.Pool.QueryRow(ctx, `SELECT failed_attempts FROM auth_challenges WHERE challenge_hash=$1`, auth.HashOpaque(login.ChallengeID)).Scan(&failures); err != nil || failures != 5 {
					t.Fatalf("exhausted challenge attempts=%d want=5 err=%v", failures, err)
				}
			}
		})
	}
}

func assertAttemptError(t *testing.T, err error, want string) {
	t.Helper()
	var apiError *platform.Error
	if !errors.As(err, &apiError) || apiError.Code != want {
		t.Fatalf("error=%v want=%s", err, want)
	}
}
