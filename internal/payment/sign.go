package payment

import (
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidSignature = errors.New("INVALID_SIGNATURE")
var errTimestampExpired = errors.New("TIMESTAMP_EXPIRED")

func Canonical(params url.Values) (string, error) {
	keys := make([]string, 0, len(params))
	for key, values := range params {
		if !utf8.ValidString(key) || len(values) > 1 {
			return "", ErrInvalidSignature
		}
		if key == "sign" || key == "sign_type" {
			continue
		}
		if len(values) == 0 || values[0] == "" {
			continue
		}
		if !utf8.ValidString(values[0]) {
			return "", ErrInvalidSignature
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params.Get(key))
	}
	return strings.Join(parts, "&"), nil
}

func VerifyV1(params url.Values, key string) error {
	canonical, err := Canonical(params)
	if err != nil {
		return err
	}
	provided := strings.ToLower(params.Get("sign"))
	if len(provided) != 32 {
		return ErrInvalidSignature
	}
	sum := md5.Sum([]byte(canonical + key))
	want := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(provided), []byte(want)) != 1 {
		return ErrInvalidSignature
	}
	return nil
}
func SignV1(params url.Values, key string) (string, error) {
	canonical, err := Canonical(params)
	if err != nil {
		return "", err
	}
	sum := md5.Sum([]byte(canonical + key))
	return hex.EncodeToString(sum[:]), nil
}
func VerifyV2(params url.Values, publicKey *rsa.PublicKey, now time.Time) error {
	if publicKey == nil {
		return ErrInvalidSignature
	}
	timestamp := params.Get("timestamp")
	if len(timestamp) != 10 {
		return ErrInvalidSignature
	}
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrInvalidSignature
	}
	delta := now.Unix() - unix
	if delta < -300 || delta > 300 {
		return errTimestampExpired
	}
	sig, err := base64.StdEncoding.DecodeString(params.Get("sign"))
	if err != nil {
		return ErrInvalidSignature
	}
	canonical, err := Canonical(params)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(canonical))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], sig); err != nil {
		return ErrInvalidSignature
	}
	typ := params.Get("sign_type")
	if typ != "" && typ != "RSA" {
		return ErrInvalidSignature
	}
	return nil
}
func SignV2(params url.Values, privateKey *rsa.PrivateKey) (string, error) {
	if privateKey == nil {
		return "", ErrInvalidSignature
	}
	canonical, err := Canonical(params)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}
