package payment

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestUnit_V2RSAReplySignatureAndTimestampWindow(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	params := url.Values{"pid": {"1234567890123456"}, "money": {"12.34"}, "timestamp": {strconv.FormatInt(now.Unix(), 10)}, "sign_type": {"RSA"}}
	signature, err := SignV2(params, key)
	if err != nil {
		t.Fatal(err)
	}
	params.Set("sign", signature)
	if err := VerifyV2(params, &key.PublicKey, now); err != nil {
		t.Fatalf("valid V2 signature rejected: %v", err)
	}
	if err := VerifyV2(params, &key.PublicKey, now.Add(301*time.Second)); err == nil || err.Error() != "TIMESTAMP_EXPIRED" {
		t.Fatalf("expired V2 signature error = %v", err)
	}
}

func TestUnit_PlatformPublicKeyFingerprint(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	id, encoded, err := platformPublicKeyInfo(&key.PublicKey)
	if err != nil || len(id) != 16 || !strings.Contains(encoded, "BEGIN PUBLIC KEY") {
		t.Fatalf("platform public key info = %q, %q, %v", id, encoded, err)
	}
	block, _ := pem.Decode([]byte(encoded))
	if block == nil {
		t.Fatal("public key PEM could not be decoded")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil || parsed.(*rsa.PublicKey).N.Cmp(key.N) != 0 {
		t.Fatalf("platform public key did not round-trip: %v", err)
	}
}
