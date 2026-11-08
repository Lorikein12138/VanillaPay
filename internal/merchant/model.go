package merchant

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"vanillapay/internal/auth"
)

type CreateMerchant struct {
	Username             string `json:"username"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
}

type MerchantDTO = auth.UserDTO

type BanResult struct {
	BannedAt      string `json:"banned_at"`
	Reason        string `json:"reason"`
	ExpiredOrders int64  `json:"expired_orders"`
}

type RSAKeyRequest struct {
	PublicKeyPEM string             `json:"public_key_pem"`
	Proof        auth.SecurityProof `json:"security_proof"`
}

type KeyDTO struct {
	PID        string `json:"pid"`
	Key        string `json:"key"`
	KeyVersion int32  `json:"key_version"`
}

type IPWhitelist struct {
	Enabled bool     `json:"enabled"`
	Entries []string `json:"entries"`
}

func ParseRSAPublicKey(value string) (*rsa.PublicKey, string, error) {
	block, rest := pem.Decode([]byte(value))
	if block == nil || len(strings.TrimSpace(string(rest))) > 0 || strings.Contains(block.Type, "PRIVATE KEY") {
		return nil, "", errors.New("INVALID_PUBLIC_KEY")
	}
	var key *rsa.PublicKey
	switch block.Type {
	case "PUBLIC KEY":
		if parsed, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
			key, _ = parsed.(*rsa.PublicKey)
		}
	case "RSA PUBLIC KEY":
		key, _ = x509.ParsePKCS1PublicKey(block.Bytes)
	}
	if key == nil || (key.N.BitLen() != 2048 && key.N.BitLen() != 3072 && key.N.BitLen() != 4096) || key.E < 3 || key.E%2 == 0 {
		return nil, "", errors.New("INVALID_PUBLIC_KEY")
	}
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, "", errors.New("INVALID_PUBLIC_KEY")
	}
	digest := sha256.Sum256(der)
	return key, hex.EncodeToString(digest[:]), nil
}

func NormalizeWhitelist(values []string, enabled bool) ([]string, error) {
	if len(values) > 100 || (enabled && len(values) == 0) {
		return nil, errors.New("VALIDATION_ERROR")
	}
	seen := make(map[netip.Prefix]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("VALIDATION_ERROR")
		}
		var prefix netip.Prefix
		if parsed, err := netip.ParsePrefix(value); err == nil {
			prefix = parsed.Masked()
		} else if addr, addrErr := netip.ParseAddr(value); addrErr == nil {
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		} else {
			return nil, errors.New("VALIDATION_ERROR")
		}
		if _, exists := seen[prefix]; exists {
			continue
		}
		seen[prefix] = struct{}{}
		out = append(out, prefix.String())
	}
	if enabled && len(out) == 0 {
		return nil, errors.New("VALIDATION_ERROR")
	}
	return out, nil
}

func validateBanReason(reason string) error {
	if n := len([]rune(strings.TrimSpace(reason))); n < 1 || n > 500 {
		return fmt.Errorf("VALIDATION_ERROR")
	}
	return nil
}

type BanRequest struct {
	Reason string `json:"reason"`
}

type ResetPasswordRequest struct {
	Password     string `json:"password"`
	Confirmation string `json:"password_confirmation"`
}

type ResetKeyRequest struct {
	Proof auth.SecurityProof `json:"security_proof"`
}

type IPWhitelistRequest struct {
	Enabled bool      `json:"enabled"`
	Entries *[]string `json:"entries"`
}
