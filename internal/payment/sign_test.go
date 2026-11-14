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

func TestUnit_InlineQRCanonicalEncoding(t *testing.T) {
	params := url.Values{
		"name":       {"Tea & Coffee = 1+1"},
		"inline_qr":  {"1"},
		"return_url": {""},
		"sign":       {"excluded"},
		"sign_type":  {"RSA"},
	}
	got, err := inlineQRCanonical(params)
	want := "\xffvanillapay:inline-qr:v1\ninline_qr=1&name=Tea+%26+Coffee+%3D+1%2B1"
	if err != nil || got != want {
		t.Fatalf("inline QR canonical encoding = %q, %v; want %q", got, err, want)
	}
}

func TestUnit_PaymentURLsWithEmptyBaseQuery(t *testing.T) {
	handler := Handler{PublicBaseURL: "https://pay.example.test?"}
	if got := handler.cashierURL("test"); got != "https://pay.example.test/pay/test" {
		t.Errorf("cashier URL = %q", got)
	}
	media := &url.URL{Path: "/media/qr.png", RawQuery: "inline=1"}
	if got := handler.absolutePaymentURL(media); got != "https://pay.example.test/media/qr.png?inline=1" {
		t.Errorf("media URL = %q", got)
	}
}
