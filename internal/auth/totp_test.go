package auth

import (
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func TestUnit_TOTPAdjacentWindowReturnsAcceptedStep(t *testing.T) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "VanillaPay", AccountName: "totp-unit", Period: 30, Digits: otp.DigitsSix, SecretSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	code, err := totp.GenerateCodeCustom(key.Secret(), now.Add(-30*time.Second), totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	step, ok := verifyTOTP(code, key.Secret(), now)
	if !ok || step != now.Unix()/30-1 {
		t.Fatalf("adjacent-window TOTP step = %d, valid=%t", step, ok)
	}
}
