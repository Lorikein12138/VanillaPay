package identity

import (
	"context"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"vanillapay/internal/merchant"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_WhitelistEnforcement(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "protocol whitelist plan", 1000, 3650, []string{"alipay"}, nil)
	fixture := testsupport.NewHTTPFixtureWithTrustedProxyCIDRs(t, db, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	deviceToken := db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	_ = deviceToken
	var pid string
	if err := db.Pool.QueryRow(context.Background(), `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&pid); err != nil {
		t.Fatal("read fixture PID")
	}

	peer4 := "192.0.2.10:43001"
	peerIP4 := "192.0.2.10"
	trustedProxy := "10.0.0.2:43000"
	const forgedClientIP = "203.0.113.77"
	if err := setProtocolWhitelist(t, db, merchantID, true, "198.51.100.0/24"); err != nil {
		t.Fatal(err)
	}
	emptyRuleOutTrade := "whitelist-v1-empty-" + strconv.FormatInt(db.Clock.Now().UnixNano(), 10)
	emptyRuleValues := url.Values{
		"pid": {pid}, "notify_url": {"https://merchant.example.test/notify"}, "return_url": {"https://merchant.example.test/return"},
		"out_trade_no": {emptyRuleOutTrade}, "name": {"nonmatching whitelist"}, "money": {"1.00"}, "clientip": {forgedClientIP}, "type": {"alipay"},
	}
	if result := postV1Protocol(t, fixture, "/mapi.php", peer4, emptyRuleValues, merchantKey); result.ErrorCode != "IP_NOT_ALLOWED" {
		t.Fatalf("enabled nonmatching whitelist accepted V1 create: %#v", result)
	}
	if err := setProtocolWhitelist(t, db, merchantID, true, peerIP4); err != nil {
		t.Fatal(err)
	}

	valuesV1 := url.Values{
		"pid": {pid}, "notify_url": {"https://merchant.example.test/notify"}, "return_url": {"https://merchant.example.test/return"},
		"out_trade_no": {"whitelist-v1-" + strconv.FormatInt(db.Clock.Now().UnixNano(), 10)},
		"name":         {"whitelist contract"}, "money": {"1.00"}, "clientip": {forgedClientIP}, "type": {"alipay"}, "param": {"source-ip"},
	}
	v1Create := postV1Protocol(t, fixture, "/mapi.php", peer4, valuesV1, merchantKey)
	if v1Create.Code != 1 || v1Create.TradeNo == "" {
		t.Fatalf("V1 create with an exact matching source IP = %#v", v1Create)
	}
	trustedHeaders := make(http.Header)
	trustedHeaders.Set("X-Forwarded-For", peerIP4)
	v1TrustedValues := cloneProtocolValues(valuesV1)
	v1TrustedValues.Set("out_trade_no", "whitelist-v1-trusted-"+strconv.FormatInt(db.Clock.Now().UnixNano(), 10))
	v1TrustedCreate := postV1ProtocolWithHeaders(t, fixture, "/mapi.php", trustedProxy, v1TrustedValues, merchantKey, trustedHeaders)
	if v1TrustedCreate.Code != 1 || v1TrustedCreate.TradeNo == "" {
		t.Fatalf("V1 create through a trusted proxy with the forwarded source IP = %#v", v1TrustedCreate)
	}
	markProtocolOrderPaid(t, fixture, v1TrustedCreate.TradeNo)
	if err := setProtocolWhitelist(t, db, merchantID, true, forgedClientIP+"/32"); err != nil {
		t.Fatal(err)
	}
	deniedV1CreateValues := cloneProtocolValues(valuesV1)
	deniedV1OutTrade := "whitelist-v1-denied-create-" + strconv.FormatInt(db.Clock.Now().UnixNano(), 10)
	deniedV1CreateValues.Set("out_trade_no", deniedV1OutTrade)
	spoofedHeaders := make(http.Header)
	spoofedHeaders.Set("X-Forwarded-For", forgedClientIP)
	deniedV1Create := postV1ProtocolWithHeaders(t, fixture, "/mapi.php", peer4, deniedV1CreateValues, merchantKey, spoofedHeaders)
	if deniedV1Create.ErrorCode != "IP_NOT_ALLOWED" || deniedV1Create.TradeNo != "" {
		t.Fatalf("V1 create accepted forwarded/clientip spoof instead of the direct peer: %#v", deniedV1Create)
	}
	checkout := fixture.RequestWithRemoteAddr(t, http.MethodGet, "/api/v1/checkout/"+v1Create.TradeNo, nil, "guest", http.Header{}, peer4)
	if checkout.Code != http.StatusOK {
		t.Fatalf("checkout was incorrectly subjected to the protocol whitelist: status=%d body=%s", checkout.Code, checkout.Body.String())
	}
	deviceHeartbeat := fixture.RequestWithRemoteAddr(t, http.MethodPost, "/api/v1/device/heartbeat", strings.NewReader(`{"app_version":"1.0"}`), "device:"+deviceToken, http.Header{}, peer4)
	if deviceHeartbeat.Code != http.StatusOK {
		t.Fatalf("device heartbeat was incorrectly subjected to the protocol whitelist: status=%d body=%s", deviceHeartbeat.Code, deviceHeartbeat.Body.String())
	}
	markProtocolOrderPaid(t, fixture, v1Create.TradeNo)
	if err := setProtocolWhitelist(t, db, merchantID, true, peerIP4); err != nil {
		t.Fatal(err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("generate independent V2 merchant key")
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal("marshal independent V2 merchant key")
	}
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
	if _, err := db.Pool.Exec(context.Background(), `UPDATE merchants SET rsa_public_key_pem=$1 WHERE id=$2`, publicPEM, merchantID); err != nil {
		t.Fatal("install independent V2 public key")
	}
	valuesV2 := url.Values{
		"pid": {pid}, "out_trade_no": {"whitelist-v2-" + strconv.FormatInt(db.Clock.Now().UnixNano(), 10)},
		"notify_url": {"https://merchant.example.test/v2-notify"}, "return_url": {"https://merchant.example.test/v2-return"},
		"name": {"V2 whitelist contract"}, "money": {"1.00"}, "clientip": {forgedClientIP}, "type": {"alipay"},
		"timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}, "method": {"web"},
	}
	v2CreateValues := cloneProtocolValues(valuesV2)
	v2CreateValues.Set("sign", signWhitelistV2(t, v2CreateValues, privateKey))
	v2Create := postV2Protocol(t, fixture, "/api/pay/create", peer4, v2CreateValues)
	if v2Create.Code != 0 || v2Create.TradeNo == "" {
		t.Fatalf("V2 create with an exact matching source IP = %#v", v2Create)
	}
	markProtocolOrderPaid(t, fixture, v2Create.TradeNo)

	v1Query := url.Values{"act": {"order"}, "pid": {pid}, "key": {merchantKey}, "trade_no": {v1Create.TradeNo}}
	if result := postV1Protocol(t, fixture, "/api.php", peer4, v1Query, merchantKey); result.Code != 1 {
		t.Fatalf("V1 query with exact IPv4 whitelist = %#v", result)
	} else if result.ClientIP != forgedClientIP {
		t.Fatalf("V1 submit did not preserve payer-supplied clientip for display: got %q want %q", result.ClientIP, forgedClientIP)
	}
	v2Query := url.Values{"pid": {pid}, "trade_no": {v2Create.TradeNo}, "timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}}
	v2Query.Set("sign", signWhitelistV2(t, v2Query, privateKey))
	if result := postV2Protocol(t, fixture, "/api/pay/query", peer4, v2Query); result.Code != 0 {
		t.Fatalf("V2 query with exact IPv4 whitelist = %#v", result)
	} else if result.ClientIP != forgedClientIP {
		t.Fatalf("V2 create did not preserve payer-supplied clientip for display: got %q want %q", result.ClientIP, forgedClientIP)
	}
	v2TrustedHeaders := make(http.Header)
	v2TrustedHeaders.Set("X-Forwarded-For", peerIP4)
	v2TrustedQuery := cloneProtocolValues(v2Query)
	v2TrustedQuery.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	v2TrustedQuery.Set("sign", signWhitelistV2(t, v2TrustedQuery, privateKey))
	if result := postV2ProtocolWithHeaders(t, fixture, "/api/pay/query", trustedProxy, v2TrustedQuery, v2TrustedHeaders); result.Code != 0 {
		t.Fatalf("V2 query through trusted proxy rejected the forwarded source address: %#v", result)
	}
	untrustedHeaders := make(http.Header)
	untrustedHeaders.Set("X-Forwarded-For", forgedClientIP)
	if result := postV1ProtocolWithHeaders(t, fixture, "/api.php", peer4, v1Query, merchantKey, untrustedHeaders); result.Code != 1 {
		t.Fatalf("untrusted proxy forwarded spoof changed the V1 source address: %#v", result)
	}
	v2Query.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	v2Query.Set("sign", signWhitelistV2(t, v2Query, privateKey))
	if result := postV2ProtocolWithHeaders(t, fixture, "/api/pay/query", peer4, v2Query, untrustedHeaders); result.Code != 0 {
		t.Fatalf("untrusted proxy forwarded spoof changed the V2 source address: %#v", result)
	}

	if err := setProtocolWhitelist(t, db, merchantID, true, forgedClientIP+"/32"); err != nil {
		t.Fatal(err)
	}
	deniedV2Values := cloneProtocolValues(valuesV2)
	deniedV2OutTrade := "whitelist-v2-denied-create-" + strconv.FormatInt(db.Clock.Now().UnixNano(), 10)
	deniedV2Values.Set("out_trade_no", deniedV2OutTrade)
	deniedV2Values.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	deniedV2Values.Set("sign", signWhitelistV2(t, deniedV2Values, privateKey))
	deniedV2Create := postV2ProtocolWithHeaders(t, fixture, "/api/pay/create", peer4, deniedV2Values, spoofedHeaders)
	if deniedV2Create.ErrorCode != "IP_NOT_ALLOWED" || deniedV2Create.TradeNo != "" {
		t.Fatalf("V2 create accepted forwarded/clientip spoof instead of the direct peer: %#v", deniedV2Create)
	}
	v1Query.Set("act", "order")
	v1DeniedQuery := postV1Protocol(t, fixture, "/api.php", peer4, v1Query, merchantKey)
	if v1DeniedQuery.ErrorCode != "IP_NOT_ALLOWED" {
		t.Fatalf("V1 query accepted forged clientip instead of actual peer: %#v", v1DeniedQuery)
	}
	v2Query.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	v2Query.Set("sign", signWhitelistV2(t, v2Query, privateKey))
	v2DeniedQuery := postV2Protocol(t, fixture, "/api/pay/query", peer4, v2Query)
	if v2DeniedQuery.ErrorCode != "IP_NOT_ALLOWED" {
		t.Fatalf("V2 query accepted forged clientip instead of actual peer: %#v", v2DeniedQuery)
	}

	v1Refund := url.Values{"act": {"refund"}, "pid": {pid}, "key": {merchantKey}, "trade_no": {v1Create.TradeNo}, "money": {"1.00"}, "out_refund_no": {"whitelist-v1-refund"}}
	v1DeniedRefund := postV1Protocol(t, fixture, "/api.php", peer4, v1Refund, merchantKey)
	if v1DeniedRefund.ErrorCode != "IP_NOT_ALLOWED" {
		t.Fatalf("V1 refund accepted forged clientip instead of actual peer: %#v", v1DeniedRefund)
	}
	v2Refund := url.Values{"pid": {pid}, "trade_no": {v2Create.TradeNo}, "money": {"1.00"}, "out_refund_no": {"whitelist-v2-refund"}, "reason": {"protocol test"}, "timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}}
	v2Refund.Set("sign", signWhitelistV2(t, v2Refund, privateKey))
	v2DeniedRefund := postV2Protocol(t, fixture, "/api/pay/refund", peer4, v2Refund)
	if v2DeniedRefund.ErrorCode != "IP_NOT_ALLOWED" {
		t.Fatalf("V2 refund accepted forged clientip instead of actual peer: %#v", v2DeniedRefund)
	}
	var refundCount int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM refunds WHERE order_id IN (SELECT id FROM orders WHERE trade_no=ANY($1::text[]))`, []string{v1Create.TradeNo, v1TrustedCreate.TradeNo, v2Create.TradeNo}).Scan(&refundCount); err != nil || refundCount != 0 {
		t.Fatalf("rejected query/refund changed refund state: count=%d err=%v", refundCount, err)
	}
	var paidCount int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM orders WHERE trade_no=ANY($1::text[]) AND status='paid'`, []string{v1Create.TradeNo, v1TrustedCreate.TradeNo, v2Create.TradeNo}).Scan(&paidCount); err != nil || paidCount != 3 {
		t.Fatalf("rejected create/query/refund changed existing paid orders: paid=%d err=%v", paidCount, err)
	}
	var deniedOrderCount int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM orders WHERE out_trade_no=ANY($1::text[])`, []string{emptyRuleOutTrade, deniedV1OutTrade, deniedV2OutTrade}).Scan(&deniedOrderCount); err != nil || deniedOrderCount != 0 {
		t.Fatalf("rejected V1/V2 creates left orders behind: count=%d err=%v", deniedOrderCount, err)
	}

	if err := setProtocolWhitelist(t, db, merchantID, false); err != nil {
		t.Fatal(err)
	}
	if result := postV1Protocol(t, fixture, "/api.php", "198.51.100.40:43002", v1Query, merchantKey); result.Code != 1 {
		t.Fatalf("disabled whitelist blocked V1 query: %#v", result)
	}
	v2Query.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	v2Query.Set("sign", signWhitelistV2(t, v2Query, privateKey))
	if result := postV2Protocol(t, fixture, "/api/pay/query", "198.51.100.40:43002", v2Query); result.Code != 0 {
		t.Fatalf("disabled whitelist blocked V2 query: %#v", result)
	}
	disabledV1Values := cloneProtocolValues(valuesV1)
	disabledV1Values.Set("out_trade_no", "whitelist-v1-disabled-create-"+strconv.FormatInt(db.Clock.Now().UnixNano(), 10))
	if result := postV1Protocol(t, fixture, "/mapi.php", "198.51.100.40:43002", disabledV1Values, merchantKey); result.Code != 1 {
		t.Fatalf("disabled whitelist blocked V1 create: %#v", result)
	}
	disabledV2Values := cloneProtocolValues(valuesV2)
	disabledV2Values.Set("out_trade_no", "whitelist-v2-disabled-create-"+strconv.FormatInt(db.Clock.Now().UnixNano(), 10))
	disabledV2Values.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	disabledV2Values.Set("sign", signWhitelistV2(t, disabledV2Values, privateKey))
	if result := postV2Protocol(t, fixture, "/api/pay/create", "198.51.100.40:43002", disabledV2Values); result.Code != 0 {
		t.Fatalf("disabled whitelist blocked V2 create: %#v", result)
	}
	if result := postV1Protocol(t, fixture, "/api.php", "198.51.100.40:43002", v1Refund, merchantKey); result.Code != 1 {
		t.Fatalf("disabled whitelist blocked V1 refund: %#v", result)
	}
	v2Refund.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	v2Refund.Set("sign", signWhitelistV2(t, v2Refund, privateKey))
	if result := postV2Protocol(t, fixture, "/api/pay/refund", "198.51.100.40:43002", v2Refund); result.Code != 0 {
		t.Fatalf("disabled whitelist blocked V2 refund: %#v", result)
	}

	if err := setProtocolWhitelist(t, db, merchantID, true, "192.0.2.0/24"); err != nil {
		t.Fatal(err)
	}
	if result := postV1Protocol(t, fixture, "/api.php", peer4, v1Query, merchantKey); result.Code != 1 {
		t.Fatalf("IPv4 CIDR whitelist rejected an in-range peer: %#v", result)
	}
	v2Query.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	v2Query.Set("sign", signWhitelistV2(t, v2Query, privateKey))
	if result := postV2Protocol(t, fixture, "/api/pay/query", peer4, v2Query); result.Code != 0 {
		t.Fatalf("IPv4 CIDR whitelist rejected an in-range V2 peer: %#v", result)
	}
	peer6 := "[2001:db8::10]:43003"
	if err := setProtocolWhitelist(t, db, merchantID, true, "2001:db8::10"); err != nil {
		t.Fatal(err)
	}
	if result := postV1Protocol(t, fixture, "/api.php", peer6, v1Query, merchantKey); result.Code != 1 {
		t.Fatalf("exact IPv6 whitelist rejected an in-range V1 peer: %#v", result)
	}
	v2Query.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	v2Query.Set("sign", signWhitelistV2(t, v2Query, privateKey))
	if result := postV2Protocol(t, fixture, "/api/pay/query", peer6, v2Query); result.Code != 0 {
		t.Fatalf("exact IPv6 whitelist rejected an in-range V2 peer: %#v", result)
	}
	if err := setProtocolWhitelist(t, db, merchantID, true, "2001:db8::/64"); err != nil {
		t.Fatal(err)
	}
	if result := postV1Protocol(t, fixture, "/api.php", peer6, v1Query, merchantKey); result.Code != 1 {
		t.Fatalf("IPv6 CIDR whitelist rejected an in-range peer: %#v", result)
	}
	v2Query.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
	v2Query.Set("sign", signWhitelistV2(t, v2Query, privateKey))
	if result := postV2Protocol(t, fixture, "/api/pay/query", peer6, v2Query); result.Code != 0 {
		t.Fatalf("IPv6 CIDR whitelist rejected an in-range V2 peer: %#v", result)
	}
	if result := postV1Protocol(t, fixture, "/api.php", peer4, v1Query, merchantKey); result.ErrorCode != "IP_NOT_ALLOWED" {
		t.Fatalf("IPv6-only whitelist accepted an IPv4 peer: %#v", result)
	}

	t.Run("V15.B.r01/v01", func(t *testing.T) {
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		if err := fixture.App.Services.Merchant.SetIPWhitelist(t.Context(), actor, merchantID, merchant.IPWhitelist{Enabled: true}); identityProfileErrorCode(err) != "VALIDATION_ERROR" {
			t.Fatalf("enabled empty whitelist configuration error=%v, want VALIDATION_ERROR", err)
		}
		var enabled bool
		var entries []string
		if err := db.Pool.QueryRow(t.Context(), `SELECT enabled,entries::text[] FROM merchant_ip_rules WHERE merchant_id=$1`, merchantID).Scan(&enabled, &entries); err != nil || !enabled || len(entries) != 1 || entries[0] != "2001:db8::/64" {
			t.Fatalf("rejected empty whitelist changed the persisted rule: enabled=%t entries=%v err=%v", enabled, entries, err)
		}
	})
	t.Run("V15.B.r01/v02", func(t *testing.T) {
		if err := setProtocolWhitelist(t, db, merchantID, true, peerIP4); err != nil {
			t.Fatal(err)
		}
		v1 := cloneProtocolValues(valuesV1)
		v1.Set("out_trade_no", "whitelist-ipv4-vector-"+strconv.FormatInt(db.Clock.Now().UnixNano(), 10))
		if result := postV1Protocol(t, fixture, "/mapi.php", peer4, v1, merchantKey); result.Code != 1 || result.TradeNo == "" {
			t.Fatalf("exact IPv4 whitelist response = %#v", result)
		}
		if err := setProtocolWhitelist(t, db, merchantID, true, "2001:db8::10"); err != nil {
			t.Fatal(err)
		}
		v6 := cloneProtocolValues(valuesV1)
		v6.Set("out_trade_no", "whitelist-ipv6-vector-"+strconv.FormatInt(db.Clock.Now().UnixNano(), 10))
		if result := postV1Protocol(t, fixture, "/mapi.php", peer6, v6, merchantKey); result.Code != 1 || result.TradeNo == "" {
			t.Fatalf("exact IPv6 whitelist response = %#v", result)
		}
		if err := setProtocolWhitelist(t, db, merchantID, true, "192.0.2.0/24"); err != nil {
			t.Fatal(err)
		}
		if result := postV2Protocol(t, fixture, "/api/pay/query", peer4, v2Query); result.Code != 0 {
			t.Fatalf("IPv4 CIDR whitelist rejected a V2 peer: %#v", result)
		}
		if err := setProtocolWhitelist(t, db, merchantID, true, "2001:db8::/64"); err != nil {
			t.Fatal(err)
		}
		if result := postV2Protocol(t, fixture, "/api/pay/query", peer6, v2Query); result.Code != 0 {
			t.Fatalf("IPv6 CIDR whitelist rejected a V2 peer: %#v", result)
		}
	})
	t.Run("V15.B.r02/v01", func(t *testing.T) {
		if err := setProtocolWhitelist(t, db, merchantID, true, forgedClientIP+"/32"); err != nil {
			t.Fatal(err)
		}
		v1Create := cloneProtocolValues(valuesV1)
		v1Create.Set("out_trade_no", "whitelist-v1-vector-denied-"+strconv.FormatInt(db.Clock.Now().UnixNano(), 10))
		v2Create := cloneProtocolValues(valuesV2)
		v2Create.Set("out_trade_no", "whitelist-v2-vector-denied-"+strconv.FormatInt(db.Clock.Now().UnixNano(), 10))
		v2Create.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
		v2Create.Set("sign", signWhitelistV2(t, v2Create, privateKey))
		for _, result := range []protocolResult{
			postV1Protocol(t, fixture, "/mapi.php", peer4, v1Create, merchantKey),
			postV2Protocol(t, fixture, "/api/pay/create", peer4, v2Create),
			postV1Protocol(t, fixture, "/api.php", peer4, v1Query, merchantKey),
			postV2Protocol(t, fixture, "/api/pay/query", peer4, v2Query),
			postV1Protocol(t, fixture, "/api.php", peer4, v1Refund, merchantKey),
			postV2Protocol(t, fixture, "/api/pay/refund", peer4, v2Refund),
		} {
			if result.ErrorCode != "IP_NOT_ALLOWED" {
				t.Fatalf("protocol operation escaped the whitelist: %#v", result)
			}
		}
	})
	t.Run("V15.B.r02/v02", func(t *testing.T) {
		spoofed := make(http.Header)
		spoofed.Set("X-Forwarded-For", forgedClientIP)
		v1 := cloneProtocolValues(valuesV1)
		v1.Set("out_trade_no", "whitelist-v1-spoof-vector-"+strconv.FormatInt(db.Clock.Now().UnixNano(), 10))
		if result := postV1ProtocolWithHeaders(t, fixture, "/mapi.php", peer4, v1, merchantKey, spoofed); result.ErrorCode != "IP_NOT_ALLOWED" || result.TradeNo != "" {
			t.Fatalf("forged V1 clientip/forwarding headers accepted: %#v", result)
		}
		v2 := cloneProtocolValues(valuesV2)
		v2.Set("out_trade_no", "whitelist-v2-spoof-vector-"+strconv.FormatInt(db.Clock.Now().UnixNano(), 10))
		v2.Set("timestamp", strconv.FormatInt(db.Clock.Now().Unix(), 10))
		v2.Set("sign", signWhitelistV2(t, v2, privateKey))
		if result := postV2ProtocolWithHeaders(t, fixture, "/api/pay/create", peer4, v2, spoofed); result.ErrorCode != "IP_NOT_ALLOWED" || result.TradeNo != "" {
			t.Fatalf("forged V2 clientip/forwarding headers accepted: %#v", result)
		}
	})
	t.Run("V15.B.r03/v01", func(t *testing.T) {
		if err := setProtocolWhitelist(t, db, merchantID, true, "198.51.100.99"); err != nil {
			t.Fatal(err)
		}
		checkout := fixture.RequestWithRemoteAddr(t, http.MethodGet, "/api/v1/checkout/"+v1Create.TradeNo, nil, "guest", http.Header{}, peer4)
		if checkout.Code != http.StatusOK {
			t.Fatalf("protocol whitelist incorrectly blocked public checkout: status=%d body=%s", checkout.Code, checkout.Body.String())
		}
		device := fixture.RequestWithRemoteAddr(t, http.MethodPost, "/api/v1/device/heartbeat", strings.NewReader(`{"app_version":"1.0"}`), "device:"+deviceToken, http.Header{}, peer4)
		if device.Code != http.StatusOK {
			t.Fatalf("protocol whitelist incorrectly blocked device API: status=%d body=%s", device.Code, device.Body.String())
		}
	})
	t.Run("V15.B.r04/v01", func(t *testing.T) {
		if err := setProtocolWhitelist(t, db, merchantID, false); err != nil {
			t.Fatal(err)
		}
		query := cloneProtocolValues(v1Query)
		result := postV1Protocol(t, fixture, "/api.php", peer4, query, merchantKey)
		if result.Code != 1 || result.ClientIP != forgedClientIP || result.ClientIP == peerIP4 {
			t.Fatalf("payer clientip is not preserved separately from actual submit source: %#v", result)
		}
	})
}

type protocolResult struct {
	Code      int    `json:"code"`
	TradeNo   string `json:"trade_no"`
	ClientIP  string `json:"clientip"`
	ErrorCode string `json:"error_code"`
}

func postV1Protocol(t *testing.T, fixture *testsupport.HTTPFixture, path, remote string, values url.Values, key string) protocolResult {
	return postV1ProtocolWithHeaders(t, fixture, path, remote, values, key, make(http.Header))
}

func postV1ProtocolWithHeaders(t *testing.T, fixture *testsupport.HTTPFixture, path, remote string, values url.Values, key string, header http.Header) protocolResult {
	t.Helper()
	values = cloneProtocolValues(values)
	values.Set("sign", signWhitelistV1(values, key))
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := fixture.RequestWithRemoteAddr(t, http.MethodPost, path, strings.NewReader(values.Encode()), "guest", header, remote)
	var result protocolResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode V1 protocol response (HTTP %d): %v: %s", response.Code, err, response.Body.String())
	}
	return result
}

func postV2Protocol(t *testing.T, fixture *testsupport.HTTPFixture, path, remote string, values url.Values) protocolResult {
	return postV2ProtocolWithHeaders(t, fixture, path, remote, values, make(http.Header))
}

func postV2ProtocolWithHeaders(t *testing.T, fixture *testsupport.HTTPFixture, path, remote string, values url.Values, header http.Header) protocolResult {
	t.Helper()
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := fixture.RequestWithRemoteAddr(t, http.MethodPost, path, strings.NewReader(values.Encode()), "guest", header, remote)
	var result protocolResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode V2 protocol response (HTTP %d): %v: %s", response.Code, err, response.Body.String())
	}
	return result
}

func setProtocolWhitelist(t *testing.T, db *testsupport.Database, merchantID int64, enabled bool, entries ...string) error {
	t.Helper()
	if entries == nil {
		entries = []string{}
	}
	_, err := db.Pool.Exec(context.Background(), `UPDATE merchant_ip_rules SET enabled=$1,entries=$2::cidr[],version=version+1,updated_at=$3 WHERE merchant_id=$4`, enabled, entries, db.Clock.Now(), merchantID)
	return err
}

func markProtocolOrderPaid(t *testing.T, fixture *testsupport.HTTPFixture, tradeNo string) {
	t.Helper()
	var orderID int64
	if err := fixture.DB.Pool.QueryRow(context.Background(), `SELECT id FROM orders WHERE trade_no=$1`, tradeNo).Scan(&orderID); err != nil {
		t.Fatalf("read protocol order before paid transition: %v", err)
	}
	tx, err := fixture.DB.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal("begin protocol paid transition")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := fixture.App.Services.Payment.MarkPaidTx(context.Background(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
		t.Fatalf("prepare paid protocol order: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal("commit paid protocol setup")
	}
}

func cloneProtocolValues(values url.Values) url.Values {
	clone := make(url.Values, len(values))
	for name, entries := range values {
		clone[name] = append([]string(nil), entries...)
	}
	return clone
}

func signWhitelistV1(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for name, entries := range values {
		if name == "sign" || name == "sign_type" || len(entries) != 1 || entries[0] == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+values.Get(name))
	}
	digest := md5.Sum([]byte(strings.Join(parts, "&") + key))
	return fmt.Sprintf("%x", digest)
}

func signWhitelistV2(t *testing.T, values url.Values, key *rsa.PrivateKey) string {
	t.Helper()
	keys := make([]string, 0, len(values))
	for name, entries := range values {
		if name == "sign" || name == "sign_type" || len(entries) != 1 || entries[0] == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+values.Get(name))
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "&")))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal("sign independent V2 whitelist request")
	}
	return base64.StdEncoding.EncodeToString(signature)
}
