package payments

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"vanillapay/internal/payment"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_V1SubmitCreatesOneSignedOrderAndRejectsDuplicateTradeNumber(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "api merchant", 1000, 3650, []string{"alipay"}, nil)
	application := db.NewApplication(t)
	db.PrepareBillingMerchant(t, application, merchantID, merchantKey, planID)
	var pid string
	if err := db.Pool.QueryRow(context.Background(), `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	router := paymentProtocolRouter(t, application.Services.Payment)
	params := url.Values{"pid": {pid}, "out_trade_no": {"v1-order-1"}, "notify_url": {"https://merchant.example/notify?out_trade_no=old"}, "return_url": {"https://merchant.example/return?out_trade_no=old"}, "name": {"测试商品"}, "money": {"1.00"}, "clientip": {"198.51.100.8"}, "type": {"alipay"}}
	params.Set("sign", mustV1ContractSignature(params, merchantKey))
	request := httptest.NewRequest(http.MethodGet, "/submit.php?"+params.Encode(), nil)
	request.RemoteAddr = "192.0.2.8:32000"
	first := httptest.NewRecorder()
	router.ServeHTTP(first, request)
	if first.Code != http.StatusFound || !strings.HasPrefix(first.Header().Get("Location"), "/pay/") {
		t.Fatalf("V1 redirect status=%d location=%q body=%s", first.Code, first.Header().Get("Location"), first.Body.String())
	}
	secondRequest := httptest.NewRequest(http.MethodGet, "/submit.php?"+params.Encode(), nil)
	secondRequest.RemoteAddr = "192.0.2.8:32000"
	second := httptest.NewRecorder()
	router.ServeHTTP(second, secondRequest)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "DUPLICATE_ORDER") {
		t.Fatalf("duplicate V1 response status=%d body=%s", second.Code, second.Body.String())
	}
	var count int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no='v1-order-1'`, merchantID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("V1 duplicate created %d orders", count)
	}
	t.Run("V28.B.r03/v02", func(t *testing.T) {
		var channel string
		var actual int64
		if err := db.Pool.QueryRow(t.Context(), `SELECT channel,actual_cents FROM orders WHERE merchant_id=$1 AND out_trade_no='v1-order-1'`, merchantID).Scan(&channel, &actual); err != nil || channel != "alipay" || actual < 100 {
			t.Fatalf("V1 selected type snapshot channel=%q actual=%d err=%v", channel, actual, err)
		}
	})
	t.Run("V28.B.r04/v01", func(t *testing.T) {
		if count != 1 {
			t.Fatalf("same-merchant out_trade_no has %d rows after duplicate rejection", count)
		}
	})
	secondID, secondKey := db.SeedMerchant(t)
	secondPlan := db.SeedPlan(t, "api merchant second", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, application, secondID, secondKey, secondPlan)
	var secondPID string
	if err := db.Pool.QueryRow(t.Context(), `SELECT pid FROM merchants WHERE id=$1`, secondID).Scan(&secondPID); err != nil {
		t.Fatal("read second merchant PID")
	}
	secondParams := url.Values{"pid": {secondPID}, "out_trade_no": {"v1-order-1"}, "notify_url": {"https://merchant.example/notify"}, "return_url": {"https://merchant.example/return"}, "name": {"second merchant order"}, "money": {"1.00"}, "type": {"alipay"}}
	secondParams.Set("sign", mustV1ContractSignature(secondParams, secondKey))
	secondMerchantRequest := httptest.NewRequest(http.MethodGet, "/submit.php?"+secondParams.Encode(), nil)
	secondMerchantRequest.RemoteAddr = "192.0.2.10:32000"
	secondResponse := httptest.NewRecorder()
	paymentProtocolRouter(t, application.Services.Payment).ServeHTTP(secondResponse, secondMerchantRequest)
	if secondResponse.Code != http.StatusFound {
		t.Fatalf("another merchant reusing out_trade_no status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	t.Run("V28.B.r04/v02", func(t *testing.T) {
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no='v1-order-1'`, secondID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("second merchant same out_trade_no rows=%d err=%v, want one", count, err)
		}
	})
}

func paymentProtocolRouter(t *testing.T, service *payment.Service) http.Handler {
	router := chi.NewRouter()
	(&payment.Handler{Service: service, PublicBaseURL: "https://pay.example.test"}).RegisterProtocolRoutes(router)
	return testsupport.ContractHandler(t, router)
}

func mustV1ContractSignature(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for name, entries := range values {
		if name != "sign" && name != "sign_type" && len(entries) > 0 && entries[0] != "" {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	canonical := ""
	for _, name := range keys {
		if canonical != "" {
			canonical += "&"
		}
		canonical += name + "=" + values.Get(name)
	}
	sum := md5.Sum([]byte(canonical + key))
	return hex.EncodeToString(sum[:])
}
