package merchant

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestUnit_MerchantSuccessEnvelopeIncludesNullError(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		merchantWrite(w, r, map[string]any{"id": "merchant-1"}, nil, http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/test", nil))
	var body map[string]json.RawMessage
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || string(body["error"]) != "null" || len(body["data"]) == 0 || len(body["request_id"]) == 0 {
		t.Fatalf("merchant success envelope status=%d body=%s; data, error=null, and request_id are required", response.Code, response.Body.String())
	}
}

func TestUnit_IPWhitelistNormalization(t *testing.T) {
	got, err := NormalizeWhitelist([]string{"192.0.2.7", "192.0.2.0/24", "2001:db8::1", "2001:db8::1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "192.0.2.7/32" || got[1] != "192.0.2.0/24" || got[2] != "2001:db8::1/128" {
		t.Fatalf("normalized whitelist = %#v", got)
	}
}

func TestUnit_IPWhitelistRejectsUnsafeRules(t *testing.T) {
	for _, input := range []IPWhitelist{{Enabled: true}, {Entries: []string{"not-an-ip"}}, {Entries: []string{"192.0.2.1/99"}}} {
		if _, err := NormalizeWhitelist(input.Entries, input.Enabled); err == nil {
			t.Errorf("NormalizeWhitelist(%#v) unexpectedly succeeded", input)
		}
	}
	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = "192.0.2.1"
	}
	if _, err := NormalizeWhitelist(tooMany, false); err == nil {
		t.Fatal("more than 100 IP rules were accepted")
	}
}

func TestUnit_RSAKeyContract(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	parsed, fingerprint, err := ParseRSAPublicKey(encoded)
	if err != nil || parsed.N.Cmp(key.N) != 0 || len(fingerprint) != 64 {
		t.Fatalf("valid public key parse failed: fingerprint=%q err=%v", fingerprint, err)
	}
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if _, _, err := ParseRSAPublicKey(privatePEM); err == nil {
		t.Fatal("private key PEM was accepted as a public key")
	}
	if _, _, err := ParseRSAPublicKey(strings.ReplaceAll(encoded, "PUBLIC KEY", "RSA PUBLIC KEY")); err == nil {
		t.Fatal("mismatched PEM type was accepted")
	}
}
