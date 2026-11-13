package payments

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/device"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_ProtocolCreateReusesOnlyIdenticalPendingOrders(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	id, key := db.SeedMerchant(t)
	plan := db.SeedPlan(t, "protocol-compat", 1000, 365, []string{"alipay", "wxpay"}, nil)
	deviceToken := db.PrepareBillingMerchant(t, fixture.App, id, key, plan)
	pid, private := protocolMerchantKey(t, db, id)

	for _, protocol := range []string{"v1", "v2"} {
		t.Run(protocol, func(t *testing.T) {
			send := func(t *testing.T, values url.Values) map[string]any {
				t.Helper()
				values = cloneValues(values)
				if protocol == "v2" {
					return postCompatV2(t, fixture, "/api/pay/create", pid, private, values)
				}
				values.Set("pid", pid)
				values.Set("sign", mustV1ContractSignature(values, key))
				return postCompatForm(t, fixture, "/mapi.php", values)
			}
			base := url.Values{
				"out_trade_no": {protocol + "|order"}, "name": {strings.Repeat("商", 60)}, "money": {"2.00"},
				"notify_url": {"https://merchant.example.test/notify"}, "return_url": {"https://merchant.example.test/return"},
				"param": {"x"}, "product_id": {"sku"}, "clientip": {"192.0.2.1"}, "type": {"alipay"}, "device": {"future-client"},
			}
			original := send(t, base)
			trade, _ := original["trade_no"].(string)
			payurl, _ := original["payurl"].(string)
			if trade == "" || !strings.HasSuffix(payurl, "/pay/"+trade) {
				t.Fatalf("create response missing cashier: %v", original)
			}
			var expiration time.Time
			if err := db.Pool.QueryRow(t.Context(), `SELECT expires_at FROM orders WHERE trade_no=$1`, trade).Scan(&expiration); err != nil {
				t.Fatal(err)
			}
			retry := cloneValues(base)
			retry.Set("clientip", "192.0.2.2")
			retry.Set("expire_seconds", "99999")
			retry.Del("type")
			var wg sync.WaitGroup
			if got := send(t, retry); got["trade_no"] != trade || got["payurl"] != payurl {
				t.Fatalf("retry did not reuse original order: %v", got)
			}
			service := fixture.App.Services.Payment
			concurrent := payment.CreateCommand{
				MerchantID: id, Protocol: protocol, Purpose: "external", OutTradeNo: protocol + "|concurrent",
				Name: "Concurrent order", OriginalCents: 300, NotifyURL: base.Get("notify_url"), Channel: "alipay",
				Actor: platform.Actor{Role: "merchant", MerchantID: id},
			}
			results := make(chan string, 8)
			for range cap(results) {
				wg.Go(func() {
					order, err := service.Create(t.Context(), concurrent)
					if err != nil {
						t.Errorf("concurrent create: %v", err)
					}
					results <- order.TradeNo
				})
			}
			wg.Wait()
			close(results)
			first := <-results
			for result := range results {
				if result == "" || result != first {
					t.Errorf("concurrent create returned %q, expected %q", result, first)
				}
			}
			var count, reservations int
			var finalExpiration time.Time
			err := db.Pool.QueryRow(t.Context(), `SELECT count(*),min(expires_at),
				(SELECT count(*) FROM amount_reservations ar JOIN orders o ON o.id=ar.order_id WHERE o.trade_no=$1)
				FROM orders WHERE merchant_id=$2 AND out_trade_no=$3`, trade, id, base.Get("out_trade_no")).
				Scan(&count, &finalExpiration, &reservations)
			if err != nil || count != 1 || reservations != 1 || !expiration.Equal(finalExpiration) {
				t.Fatalf("duplicate changed order or reservation: count=%d reservations=%d expires=%v err=%v", count, reservations, finalExpiration, err)
			}
			for field, value := range map[string]string{
				"money": "2.0", "name": base.Get("name") + "品", "notify_url": "https://merchant.example.test/notify2",
				"return_url": "https://merchant.example.test/return2", "param": "y", "product_id": "other", "type": "wxpay",
			} {
				mismatch := cloneValues(base)
				mismatch.Set(field, value)
				if got := send(t, mismatch); got["error_code"] != "DUPLICATE_ORDER" {
					t.Errorf("mismatched %s: %v", field, got)
				}
			}
			unsupported := cloneValues(base)
			unsupported.Set("type", "qqpay")
			if got := send(t, unsupported); got["error_code"] != "VALIDATION_ERROR" {
				t.Errorf("retired channel: %v", got)
			}
			unlocked := cloneValues(base)
			unlocked.Set("out_trade_no", protocol+"|unlocked")
			unlocked.Del("type")
			if got := send(t, unlocked); got["trade_no"] == nil {
				t.Fatalf("create unlocked order: %v", got)
			}
			unlocked.Set("type", "alipay")
			if got := send(t, unlocked); got["error_code"] != "DUPLICATE_ORDER" {
				t.Errorf("retry with newly selected type: %v", got)
			}
			query := func() map[string]any {
				params := url.Values{"out_trade_no": {base.Get("out_trade_no")}}
				if protocol == "v2" {
					return postCompatV2(t, fixture, "/api/pay/query", pid, private, params)
				}
				params.Set("act", "order")
				return getCompatV1(t, fixture, pid, key, params)
			}
			if got := query(); got["payurl"] != payurl {
				t.Errorf("pending query payurl: %v", got)
			}
			if _, err := db.Pool.Exec(t.Context(), `UPDATE orders SET expires_at=$1 WHERE trade_no=$2`, db.Clock.Now(), trade); err != nil {
				t.Fatal(err)
			}
			if got := send(t, base); got["error_code"] != "DUPLICATE_ORDER" {
				t.Errorf("expired retry: %v", got)
			}
			if got := query(); got["payurl"] != nil {
				t.Errorf("expired query exposed payurl: %v", got)
			}
		})
	}
	rejected := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", device.ReceiptRequest{
		EventID: "cbf69244-e81e-43f7-a31b-0c9d4ce86785", Channel: "qqpay", AmountCents: 200, OccurredAt: db.Clock.Now(),
	}, "device:"+deviceToken)
	if rejected.Code != http.StatusBadRequest || !strings.Contains(rejected.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("legacy QQ receipt response=%d %s", rejected.Code, rejected.Body.String())
	}
}

func TestHTTP_MerchantProtocolStatisticsAndPagination(t *testing.T) {
	db := testsupport.NewDatabase(t)
	db.Clock.Set(time.Date(2026, 11, 13, 16, 1, 0, 0, time.UTC))
	fixture := testsupport.NewHTTPFixture(t, db)
	id, key := db.SeedMerchant(t)
	otherID, _ := db.SeedMerchant(t)
	pid, private := protocolMerchantKey(t, db, id)
	midnight := db.Clock.Now().Truncate(time.Hour)
	for i, row := range []struct {
		merchant       int64
		purpose, state string
		at             time.Time
	}{
		{id, "external", "paid", midnight.Add(-24 * time.Hour)},
		{id, "external", "paid", midnight},
		{id, "external", "refunded", midnight},
		{id, "external", "pending", midnight},
		{id, "external", "expired", midnight},
		{id, "subscription", "paid", midnight},
		{otherID, "external", "paid", midnight},
	} {
		trade := fmt.Sprintf("merchant-%d", i)
		orderID := seedPendingExternalOrder(t, db, row.merchant, trade, trade+"|out", "https://merchant.example.test/notify")
		_, err := db.Pool.Exec(t.Context(), `UPDATE orders SET purpose=$2,status=$3,created_at=$4,clientip='192.0.2.3',
			channel='alipay',actual_cents=100,
			paid_at=CASE WHEN $3 IN ('paid','refunded') THEN $4::timestamptz ELSE NULL END,
			refunded_at=CASE WHEN $3='refunded' THEN $4::timestamptz ELSE NULL END WHERE id=$1`, orderID, row.purpose, row.state, row.at)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, v2 := range []bool{false, true} {
		info := func(t *testing.T) map[string]any {
			t.Helper()
			if v2 {
				return postCompatV2(t, fixture, "/api/merchant/info", pid, private, nil)
			}
			return getCompatV1(t, fixture, pid, key, url.Values{"act": {"query"}})
		}
		active, total, today, yesterday, code := "active", "orders", "orders_today", "orders_lastday", float64(1)
		if v2 {
			active, total, today, yesterday, code = "status", "order_num", "order_num_today", "order_num_lastday", 0
		}
		got := info(t)
		if got["code"] != code || got["pid"] != pid || got[active] != float64(1) || got[total] != float64(5) ||
			got[today] != float64(1) || got[yesterday] != float64(1) || got["order_money_today"] != "1.00" || got["order_money_lastday"] != "1.00" ||
			got["key"] != nil || got["balance"] != nil {
			t.Fatalf("v2=%t merchant info: %v", v2, got)
		}
		list := func(t *testing.T, values url.Values) map[string]any {
			t.Helper()
			if v2 {
				return postCompatV2(t, fixture, "/api/merchant/orders", pid, private, values)
			}
			values.Set("act", "orders")
			return getCompatV1(t, fixture, pid, key, values)
		}
		page := list(t, url.Values{"limit": {"2"}, "offset": {"1"}})
		items, _ := page["data"].([]any)
		if page["count"] != float64(5) || len(items) != 2 || items[0].(map[string]any)["trade_no"] != "merchant-3" || items[1].(map[string]any)["trade_no"] != "merchant-2" {
			t.Fatalf("v2=%t page and tie break: %v", v2, page)
		}
		if v2 && (items[1].(map[string]any)["refundmoney"] != "1.00" || items[1].(map[string]any)["clientip"] != "192.0.2.3") {
			t.Fatalf("v2 extended fields: %v", items[1])
		}
		filtered := list(t, url.Values{"status": {"0"}, "limit": {"51"}})
		if filtered["count"] != float64(2) {
			t.Fatalf("v2=%t status filter: %v", v2, filtered)
		}
		if empty := list(t, url.Values{"offset": {"10000"}}); empty["count"] != float64(5) || len(empty["data"].([]any)) != 0 {
			t.Fatalf("v2=%t empty page: %v", v2, empty)
		}
		for field, value := range map[string]string{"offset": "-1", "limit": "1.5", "status": "3"} {
			if bad := list(t, url.Values{field: {value}}); bad["error_code"] != "VALIDATION_ERROR" {
				t.Fatalf("v2=%t bad pagination %s=%s: %v", v2, field, value, bad)
			}
		}
	}
	if _, err := db.Pool.Exec(t.Context(), `UPDATE merchants SET banned_at=$2,ban_reason='test' WHERE id=$1`, id, db.Clock.Now()); err != nil {
		t.Fatal(err)
	}
	if got := getCompatV1(t, fixture, pid, key, url.Values{"act": {"query"}}); got["active"] != float64(0) {
		t.Fatalf("banned merchant info: %v", got)
	}
	if got := getCompatV1(t, fixture, pid, "wrong", url.Values{"act": {"query"}}); got["error_code"] != "INVALID_SIGNATURE" {
		t.Fatalf("invalid merchant key: %v", got)
	}
}

func TestHTTP_ProtocolCloseAndRefundQueryAreMerchantScoped(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	merchant, key := db.SeedMerchant(t)
	other, otherKey := db.SeedMerchant(t)
	plan := db.SeedPlan(t, "protocol-close", 1000, 365, []string{"alipay"}, nil)
	token := db.PrepareBillingMerchant(t, fixture.App, merchant, key, plan)
	db.PrepareBillingMerchant(t, fixture.App, other, otherKey, plan)
	pid, private := protocolMerchantKey(t, db, merchant)
	otherPID, otherPrivate := protocolMerchantKey(t, db, other)
	service := fixture.App.Services.Payment
	actor := platform.Actor{Role: "merchant", MerchantID: merchant, SubjectID: merchant}
	makeOrder := func(id int64, number string) payment.OrderDTO {
		t.Helper()
		order, err := service.Create(t.Context(), payment.CreateCommand{
			MerchantID: id, Protocol: "v2", Purpose: "external", OutTradeNo: number,
			Name: "Test order", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", Channel: "alipay",
			Actor: platform.Actor{Role: "merchant", MerchantID: id, SubjectID: id},
		})
		if err != nil {
			t.Fatal(err)
		}
		return order
	}
	order := makeOrder(merchant, "close|one")
	foreign := postCompatV2(t, fixture, "/api/pay/close", otherPID, otherPrivate, url.Values{"trade_no": {order.TradeNo}})
	if foreign["msg"] != "当前订单不存在" {
		t.Fatalf("foreign close: %v", foreign)
	}
	closeOrder := func(order payment.OrderDTO) map[string]any {
		return postCompatV2(t, fixture, "/api/pay/close", pid, private, url.Values{"out_trade_no": {order.OutTradeNo}})
	}
	if got := closeOrder(order); got["msg"] != "订单关闭成功" {
		t.Fatalf("close pending order: %v", got)
	}
	if got := closeOrder(order); got["msg"] != "该订单状态不支持关闭" {
		t.Fatalf("close already closed order: %v", got)
	}
	var state, reason string
	var release time.Time
	var audits int
	err := db.Pool.QueryRow(t.Context(), `SELECT o.status,o.expire_reason,a.release_after,
		(SELECT count(*) FROM audit_logs WHERE action='order.close' AND target_id=o.trade_no)
		FROM orders o JOIN amount_reservations a ON a.order_id=o.id WHERE o.trade_no=$1`, order.TradeNo).Scan(&state, &reason, &release, &audits)
	if err != nil || state != "expired" || reason != "cancelled" || !release.Equal(order.ExpiresAt.Truncate(time.Microsecond).Add(24*time.Hour)) || audits != 1 {
		t.Fatalf("closed row: state=%s reason=%s release=%v audit=%d err=%v", state, reason, release, audits, err)
	}
	if err := service.SupplementIdempotent(t.Context(), actor, order.TradeNo, payment.SupplementRequest{Channel: "alipay", Reason: "late receipt"}, ""); errorCode(err) != "ORDER_NOT_PAYABLE" {
		t.Fatalf("closed order accepted supplement: %v", err)
	}
	receipt := func(order payment.OrderDTO, eventID string) *httptest.ResponseRecorder {
		return fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", device.ReceiptRequest{
			EventID: eventID, Channel: "alipay", AmountCents: *order.ActualCents, OccurredAt: db.Clock.Now(),
		}, "device:"+token)
	}
	late := receipt(order, "20b29584-f78c-4ace-a49f-453630fd78f8")
	if late.Code != http.StatusCreated || !strings.Contains(late.Body.String(), `"match_state":"unmatched"`) {
		t.Fatalf("late receipt: %d %s", late.Code, late.Body.String())
	}

	racing := makeOrder(merchant, "close|race")
	// Start close while the paid transition holds the order lock. It must
	// observe the committed paid state, even though it started while pending.
	tx, err := db.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	racingID, err := service.OrderID(t.Context(), racing.TradeNo)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.MarkPaidTx(t.Context(), tx, racingID, payment.PaidCommand{Channel: "alipay"}); err != nil {
		t.Fatal(err)
	}
	params := url.Values{"pid": {pid}, "out_trade_no": {racing.OutTradeNo}, "timestamp": {strconv.FormatInt(db.Clock.Now().Unix(), 10)}, "sign_type": {"RSA"}}
	params.Set("sign", signV2Independent(t, params, private))
	handler := &payment.Handler{Service: service}
	request := httptest.NewRequest(http.MethodPost, "/api/pay/close", strings.NewReader(params.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	racingClose := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ProtocolRoutes().ServeHTTP(racingClose, request)
	}()
	select {
	case <-done:
		t.Fatal("close returned before the paid transaction committed")
	case <-time.After(40 * time.Millisecond):
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-done
	if got := decodeCompatResponse(t, racingClose.Code, racingClose.Body.Bytes()); got["msg"] != "该订单状态不支持关闭" {
		t.Fatalf("payment won the order lock: %v", got)
	}
	if got := postCompatV2(t, fixture, "/api/pay/query", pid, private, url.Values{"trade_no": {racing.TradeNo}}); got["payurl"] != nil {
		t.Fatalf("paid query exposed a cashier URL: %v", got)
	}

	otherOrder := makeOrder(other, "refund|foreign")
	otherActor := platform.Actor{Role: "merchant", SubjectID: other, MerchantID: other}
	if err := service.SupplementIdempotent(t.Context(), otherActor, otherOrder.TradeNo, payment.SupplementRequest{Channel: "alipay", Reason: "refund fixture"}, ""); err != nil {
		t.Fatal(err)
	}
	shared := "same-refund-no"
	refund := func(pid string, private *rsa.PrivateKey, order payment.OrderDTO) string {
		t.Helper()
		got := postCompatV2(t, fixture, "/api/pay/refund", pid, private, url.Values{"trade_no": {order.TradeNo}, "out_refund_no": {shared}, "money": {"1.00"}})
		if id, ok := got["refund_no"].(string); ok && id != "" {
			return id
		}
		t.Fatalf("request shared refund number: %v", got)
		return ""
	}
	first, second := refund(pid, private, racing), refund(otherPID, otherPrivate, otherOrder)
	if first == second {
		t.Fatal("distinct merchants share a refund ID")
	}
	query := func(values url.Values) map[string]any {
		return postCompatV2(t, fixture, "/api/pay/refundquery", pid, private, values)
	}
	if got := query(url.Values{"refund_no": {second}}); got["error_code"] != "NOT_FOUND" {
		t.Fatalf("foreign refund query: %v", got)
	}
	if got := query(url.Values{"out_refund_no": {shared}}); got["refund_no"] != first || got["trade_no"] != racing.TradeNo ||
		got["status"] != float64(0) || got["refund_status"] != "pending" || got["endtime"] != "" || got["money"] != "1.00" {
		t.Fatalf("pending refund query: %v", got)
	}
	if err := service.ResolveRefund(t.Context(), actor, first, payment.RefundResolution{Action: "mark_refunded", Confirm: true, Reason: "settled"}); err != nil {
		t.Fatal(err)
	}
	if got := query(url.Values{"refund_no": {first}}); got["status"] != float64(1) || got["refund_status"] != "completed" || got["endtime"] == "" {
		t.Fatalf("completed refund query: %v", got)
	}
	if err := service.ResolveRefund(t.Context(), otherActor, second, payment.RefundResolution{Action: "reject", Reason: "denied"}); err != nil {
		t.Fatal(err)
	}
	got := postCompatV2(t, fixture, "/api/pay/refundquery", otherPID, otherPrivate, url.Values{"refund_no": {second}, "out_refund_no": {shared}})
	if got["status"] != float64(2) || got["refund_status"] != "rejected" || got["endtime"] == "" {
		t.Fatalf("rejected refund query: %v", got)
	}
}

func protocolMerchantKey(t *testing.T, db *testsupport.Database, id int64) (string, *rsa.PrivateKey) {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	public := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: mustMarshalPKIX(t, &private.PublicKey)})
	var pid string
	if err := db.Pool.QueryRow(t.Context(), `UPDATE merchants SET rsa_public_key_pem=$2 WHERE id=$1 RETURNING pid`, id, string(public)).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return pid, private
}

func postCompatV2(t *testing.T, fixture *testsupport.HTTPFixture, path, pid string, private *rsa.PrivateKey, values url.Values) map[string]any {
	t.Helper()
	values = cloneValues(values)
	values.Set("pid", pid)
	values.Set("timestamp", strconv.FormatInt(fixture.DB.Clock.Now().Unix(), 10))
	values.Set("sign_type", "RSA")
	values.Set("sign", signV2Independent(t, values, private))
	response := fixture.RequestWithHeaders(t, http.MethodPost, path, strings.NewReader(values.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	payload := decodeCompatResponse(t, response.Code, response.Body.Bytes())
	verifyPlatformV2Response(t, fixture, response.Body.Bytes())
	return payload
}

func getCompatV1(t *testing.T, fixture *testsupport.HTTPFixture, pid, key string, values url.Values) map[string]any {
	t.Helper()
	values.Set("pid", pid)
	values.Set("key", key)
	response := fixture.Request(t, http.MethodGet, "/api.php?"+values.Encode(), nil, "guest")
	return decodeCompatResponse(t, response.Code, response.Body.Bytes())
}

func postCompatForm(t *testing.T, fixture *testsupport.HTTPFixture, path string, values url.Values) map[string]any {
	t.Helper()
	response := fixture.RequestWithHeaders(t, http.MethodPost, path, strings.NewReader(values.Encode()), "guest", http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	return decodeCompatResponse(t, response.Code, response.Body.Bytes())
}

func decodeCompatResponse(t *testing.T, status int, body []byte) map[string]any {
	t.Helper()
	var data map[string]any
	if status != http.StatusOK || json.Unmarshal(body, &data) != nil {
		t.Fatalf("unexpected protocol response: HTTP %d", status)
	}
	return data
}
