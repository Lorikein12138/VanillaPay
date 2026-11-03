package auth

import (
	"bytes"
	"crypto/rand"
	"regexp"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

func TestUnit_NormalizeCredentials(t *testing.T) {
	username, err := NormalizeUsername("  Alice_01 ")
	if err != nil || username != "alice_01" {
		t.Fatalf("NormalizeUsername() = %q, %v", username, err)
	}
	for _, invalid := range []string{"ab", "has space", "mail@example.com", "name/role"} {
		if _, err := NormalizeUsername(invalid); err == nil {
			t.Errorf("NormalizeUsername(%q) unexpectedly succeeded", invalid)
		}
	}
	email, err := NormalizeEmail(" Test@BÜCHER.de ")
	if err != nil || email != "test@xn--bcher-kva.de" {
		t.Fatalf("NormalizeEmail() = %q, %v", email, err)
	}
	if NormalizeEmailValue, err := NormalizeEmail("not an email"); err == nil || NormalizeEmailValue != "" {
		t.Fatalf("invalid email normalized to %q", NormalizeEmailValue)
	}
}

func TestUnit_Argon2idHashAndPasswordRules(t *testing.T) {
	if err := ValidatePassword("short", "short"); err == nil {
		t.Fatal("short password was accepted")
	}
	if err := ValidatePassword("twelve chars", "different value"); err == nil {
		t.Fatal("password mismatch was accepted")
	}
	password := "Aa1!" + rand.Text()
	encoded, err := HashPassword(password, bytes.NewReader(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\$argon2id\$v=19\$m=65536,t=3,p=1\$`).MatchString(encoded) {
		t.Fatalf("hash parameters differ from the design: %q", encoded)
	}
	if !VerifyPassword(encoded, password) || VerifyPassword(encoded, "wrong password") {
		t.Fatal("Argon2id verification accepted the wrong password or rejected the right one")
	}
}

func TestUnit_GeneratedMerchantIdentifiers(t *testing.T) {
	pid, err := GeneratePID(bytes.NewReader(bytes.Repeat([]byte{0x45}, 64)))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[1-9][0-9]{15}$`).MatchString(pid) {
		t.Fatalf("PID does not match the public contract: %q", pid)
	}
	key, err := GenerateMerchantKey(bytes.NewReader(bytes.Repeat([]byte{0x11}, 32)))
	if err != nil || len(key) != 64 {
		t.Fatalf("generated key length = %d, err=%v", len(key), err)
	}
}

func TestUnit_EmailDomainAndCodeValidation(t *testing.T) {
	if !ValidateEmailDomain("merchant@example.com", []string{"example.com"}) {
		t.Fatal("exact domain was rejected")
	}
	if ValidateEmailDomain("merchant@sub.example.com", []string{"example.com"}) {
		t.Fatal("subdomain was accepted by exact domain policy")
	}
	if !ValidSixDigitCode("012345") || ValidSixDigitCode("12345") || ValidSixDigitCode("12a456") {
		t.Fatal("six digit code validation is incorrect")
	}
}

func TestUnit_TOTPWindowAndReplayStep(t *testing.T) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "VanillaPay", AccountName: "unit", Period: 30, Digits: otp.DigitsSix, SecretSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	code, err := totp.GenerateCodeCustom(key.Secret(), now, totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	step, ok := verifyTOTP(code, key.Secret(), now)
	if !ok || step != now.Unix()/30 {
		t.Fatalf("TOTP step = %d, valid=%t", step, ok)
	}
	if _, ok := verifyTOTP("not-a-code", key.Secret(), now); ok {
		t.Fatal("malformed TOTP was accepted")
	}
}
