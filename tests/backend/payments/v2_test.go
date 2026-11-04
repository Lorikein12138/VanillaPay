package payments

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_V2CreateUsesMerchantKeyAndReturnsPlatformSignedJump(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "api merchant", 1000, 3650, []string{"alipay"}, nil)
	application := db.NewApplication(t)
	db.PrepareBillingMerchant(t, application, merchantID, merchantKey, planID)
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	if _, err := db.Pool.Exec(context.Background(), `UPDATE merchants SET rsa_public_key_pem=$1 WHERE id=$2`, string(publicPEM), merchantID); err != nil {
		t.Fatal(err)
	}
	var pid string
	if err := db.Pool.QueryRow(context.Background(), `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	params := url.Values{"pid": {pid}, "out_trade_no": {"v2-order-1"}, "notify_url": {"https://merchant.example/notify"}, "name": {"商品"}, "money": {"2.00"}, "clientip": {"198.51.100.9"}, "timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}, "method": {"web"}}
	params.Set("sign", mustSignV2(t, params, private))
	request := httptest.NewRequest(http.MethodPost, "/api/pay/create", strings.NewReader(params.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.RemoteAddr = "192.0.2.9:32000"
	response := httptest.NewRecorder()
	paymentProtocolRouter(t, application.Services.Payment).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("V2 response status=%d body=%s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != float64(0) || payload["pay_type"] != "jump" || !strings.HasPrefix(payload["pay_info"].(string), "https://pay.example.test/pay/") {
		t.Fatalf("V2 create payload = %#v", payload)
	}
	responseValues := url.Values{}
	for key, value := range payload {
		switch v := value.(type) {
		case string:
			responseValues.Set(key, v)
		case float64:
			responseValues.Set(key, strconv.FormatInt(int64(v), 10))
		}
	}
	platformKeys, err := application.Services.Payment.PlatformPublicKeys(context.Background())
	if err != nil || len(platformKeys) != 1 {
		t.Fatalf("platform signing key list = %#v, %v", platformKeys, err)
	}
	block, _ := pem.Decode([]byte(platformKeys[0]["pem"]))
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := payment.VerifyV2(responseValues, parsed.(*rsa.PublicKey), db.Clock.Now()); err != nil {
		t.Fatalf("platform reply signature rejected: %v", err)
	}
}

func mustSignV2(t *testing.T, params url.Values, key *rsa.PrivateKey) string {
	t.Helper()
	signature, err := payment.SignV2(params, key)
	if err != nil {
		t.Fatal(err)
	}
	return signature
}
