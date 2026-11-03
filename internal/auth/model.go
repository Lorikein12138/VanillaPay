package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/argon2"
	"golang.org/x/net/idna"
)

var (
	usernamePattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
	ErrInvalidUsername = errors.New("VALIDATION_ERROR")
	ErrInvalidEmail    = errors.New("VALIDATION_ERROR")
	ErrInvalidPassword = errors.New("VALIDATION_ERROR")
)

type UserDTO struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	Username  string     `json:"username"`
	Email     string     `json:"email,omitempty"`
	PID       string     `json:"pid,omitempty"`
	Banned    bool       `json:"banned"`
	BannedAt  *time.Time `json:"banned_at"`
	BanReason *string    `json:"ban_reason"`
	TOTP      bool       `json:"totp_enabled"`
	Locale    string     `json:"locale"`
	Theme     string     `json:"theme"`
	CreatedAt time.Time  `json:"created_at"`
}

type SessionResult struct {
	User      UserDTO   `json:"user"`
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
	NextPath  string    `json:"next_path"`
	Token     string    `json:"-"`
}

// ConsoleRecoverySession is an in-memory result for the one-use repair flow.
// The token is only placed in an HttpOnly cookie and is never serialized.
type ConsoleRecoverySession struct {
	Token       string
	CSRFToken   string
	ExpiresAt   time.Time
	ChallengeID int64
}

type RegisterRequest struct {
	Username             string `json:"username"`
	Email                string `json:"email"`
	EmailCode            string `json:"email_code"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
	AcceptTerms          bool   `json:"accept_terms"`
	AcceptPrivacy        bool   `json:"accept_privacy"`
	TermsRevision        int64  `json:"terms_revision"`
	PrivacyRevision      int64  `json:"privacy_revision"`
	TurnstileToken       string `json:"turnstile_token"`
}

type BootstrapRequest struct {
	InitializationCode   string `json:"initialization_code"`
	Username             string `json:"username"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
}

type LoginRequest struct {
	Identifier     string `json:"identifier"`
	Password       string `json:"password"`
	TurnstileToken string `json:"turnstile_token"`
}

type LoginResult struct {
	Session     *SessionResult `json:"-"`
	ChallengeID string         `json:"challenge_id,omitempty"`
	ExpiresAt   time.Time      `json:"expires_at,omitempty"`
	Requires2FA bool           `json:"requires_2fa,omitempty"`
}

type FactorRequest struct {
	ChallengeID  string `json:"challenge_id"`
	TOTPCode     string `json:"totp_code,omitempty"`
	RecoveryCode string `json:"recovery_code,omitempty"`
}

type EmailCodeRequest struct {
	Purpose        string `json:"purpose"`
	Email          string `json:"email"`
	TargetUsername string `json:"target_username,omitempty"`
	TargetEmail    string `json:"target_email,omitempty"`
	TurnstileToken string `json:"turnstile_token,omitempty"`
}

type IdentityChange struct {
	Username     string `json:"username,omitempty"`
	Email        string `json:"email,omitempty"`
	OldEmailCode string `json:"old_email_code,omitempty"`
	NewEmailCode string `json:"new_email_code,omitempty"`
	Password     string `json:"password,omitempty"`
	Confirmation string `json:"password_confirmation,omitempty"`
	EmailCode    string `json:"email_code,omitempty"`
	TOTPCode     string `json:"totp_code,omitempty"`
}

type ResetPasswordRequest struct {
	Email                string `json:"email"`
	EmailCode            string `json:"email_code"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
	TurnstileToken       string `json:"turnstile_token"`
}

type TOTPSetup struct {
	SetupID    string    `json:"setup_id"`
	Secret     string    `json:"secret"`
	OTPAuthURL string    `json:"otpauth_url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type TOTPConfirm struct {
	SetupID  string `json:"setup_id"`
	TOTPCode string `json:"totp_code"`
}

type FactorProof struct {
	Password  string `json:"password"`
	TOTPCode  string `json:"totp_code"`
	EmailCode string `json:"email_code,omitempty"`
}

type SecurityProof struct {
	EmailCode string `json:"email_code"`
	TOTPCode  string `json:"totp_code,omitempty"`
}

type EmailCodeResult struct {
	Sent              bool `json:"sent"`
	RetryAfterSeconds int  `json:"retry_after_seconds"`
}

func NormalizeUsername(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !usernamePattern.MatchString(value) || strings.Contains(value, "@") {
		return "", ErrInvalidUsername
	}
	return strings.ToLower(value), nil
}

func NormalizeEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || strings.Count(value, "@") != 1 {
		return "", ErrInvalidEmail
	}
	parts := strings.SplitN(value, "@", 2)
	if parts[0] == "" || len(parts[0]) > 64 || strings.ContainsAny(parts[0], " \t\r\n") {
		return "", ErrInvalidEmail
	}
	domain, err := idna.Lookup.ToASCII(strings.TrimSuffix(parts[1], "."))
	if err != nil || domain == "" || strings.Contains(domain, "..") {
		return "", ErrInvalidEmail
	}
	return strings.ToLower(parts[0] + "@" + domain), nil
}

func ValidatePassword(password, confirmation string) error {
	if len([]rune(password)) < 12 || len([]rune(password)) > 128 || password != confirmation {
		return ErrInvalidPassword
	}
	return nil
}

func HashPassword(password string, random io.Reader) (string, error) {
	if err := ValidatePassword(password, password); err != nil {
		return "", err
	}
	if random == nil {
		random = rand.Reader
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(random, salt); err != nil {
		return "", errors.New("password hash salt unavailable")
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(encoded, password string) bool {
	var memory, iterations, parallelism uint32
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil || memory < 8*1024 || memory > 1024*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 16 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, uint8(parallelism), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func GeneratePID(random io.Reader) (string, error) {
	if random == nil {
		random = rand.Reader
	}
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(15), nil)
	span := new(big.Int).Mul(limit, big.NewInt(9))
	n, err := rand.Int(random, span)
	if err != nil {
		return "", errors.New("PID random source unavailable")
	}
	n.Add(n, limit)
	return n.String(), nil
}

func GenerateMerchantKey(random io.Reader) (string, error) {
	if random == nil {
		random = rand.Reader
	}
	data := make([]byte, 32)
	if _, err := io.ReadFull(random, data); err != nil {
		return "", errors.New("merchant key random source unavailable")
	}
	return hex.EncodeToString(data), nil
}

func HashOpaque(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func ValidateEmailDomain(email string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	parts := strings.SplitN(strings.ToLower(email), "@", 2)
	if len(parts) != 2 {
		return false
	}
	for _, domain := range allowed {
		if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(domain), "@"), parts[1]) {
			return true
		}
	}
	return false
}

func ValidSixDigitCode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func newNumericCode(random io.Reader) (string, error) {
	if random == nil {
		random = rand.Reader
	}
	value, err := rand.Int(random, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func verifyTOTP(code, secret string, now time.Time) (int64, bool) {
	if !ValidSixDigitCode(code) || strings.TrimSpace(secret) == "" {
		return 0, false
	}
	for delta := int64(-1); delta <= 1; delta++ {
		step := now.Unix()/30 + delta
		at := time.Unix(step*30, 0)
		valid, err := totp.ValidateCustom(code, secret, at, totp.ValidateOpts{Period: 30, Skew: 0, Digits: 6})
		if err == nil && valid {
			return step, true
		}
	}
	return 0, false
}
