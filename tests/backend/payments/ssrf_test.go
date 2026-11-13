package payments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

type callbackRetryAttemptSnapshot struct {
	AttemptNo  int
	ID         int64
	StartedAt  time.Time
	FinishedAt time.Time
	Status     int
	Excerpt    string
	ErrorCode  string
	Redacted   string
	DurationMS int64
	Next       *time.Time
}

type callbackRetrySnapshot struct {
	Attempts     []callbackRetryAttemptSnapshot
	AttemptCount int
	State        string
	EventNext    *time.Time
	FirstID      int64
}

func TestHTTP_CallbackNetworkSafety(t *testing.T) {
	t.Run("V36.B.r01/localhost", func(t *testing.T) { rejectDNSAddress(t, "localhost", "127.0.0.1") })
	t.Run("V36.B.r01/v4_loopback", func(t *testing.T) { rejectLiteralTarget(t, "http://127.0.0.1/notify") })
	t.Run("V36.B.r01/private_10", func(t *testing.T) { rejectLiteralTarget(t, "http://10.0.0.1/notify") })
	t.Run("V36.B.r01/private_172", func(t *testing.T) { rejectLiteralTarget(t, "http://172.16.0.1/notify") })
	t.Run("V36.B.r01/private_192", func(t *testing.T) { rejectLiteralTarget(t, "http://192.168.0.1/notify") })
	t.Run("V36.B.r01/v4_link", func(t *testing.T) { rejectLiteralTarget(t, "http://169.254.1.1/notify") })
	t.Run("V36.B.r01/v6_loopback", func(t *testing.T) { rejectLiteralTarget(t, "http://[::1]/notify") })
	t.Run("V36.B.r01/v6_ula", func(t *testing.T) { rejectLiteralTarget(t, "http://[fc00::1]/notify") })
	t.Run("V36.B.r01/v6_link", func(t *testing.T) { rejectLiteralTarget(t, "http://[fe80::1]/notify") })
	t.Run("V36.B.r01/mapped_private", func(t *testing.T) { rejectLiteralTarget(t, "http://[::ffff:127.0.0.1]/notify") })
	t.Run("V36.B.r01/special_purpose_v4", func(t *testing.T) {
		for _, address := range []string{"0.0.0.1", "100.64.0.1", "192.0.0.1", "192.0.2.1", "192.88.99.1", "198.18.0.1", "198.51.100.1", "203.0.113.1", "240.0.0.1"} {
			rejectLiteralTarget(t, "http://"+address+"/notify")
		}
	})
	t.Run("V36.B.r01/special_purpose_v6", func(t *testing.T) {
		for _, address := range []string{"64:ff9b::a00:1", "100::1", "2001::1", "2001:db8::1", "2002:a00:1::1", "3fff::1", "5f00::1", "::ffff:100.64.0.1"} {
			rejectLiteralTarget(t, "http://["+address+"]/notify")
		}
	})
	t.Run("V36.B.r01/special_purpose_dns", func(t *testing.T) {
		for _, address := range []string{"100.64.0.1", "198.18.0.1"} {
			rejectDNSAnswers(t, []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr(address)})
		}
	})
	t.Run("V36.B.r01/mixed_public_private_first", func(t *testing.T) {
		rejectDNSAnswers(t, []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.1")})
	})
	t.Run("V36.B.r01/mixed_public_private_last", func(t *testing.T) {
		rejectDNSAnswers(t, []netip.Addr{netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("93.184.216.34")})
	})
	t.Run("V36.B.r01/mixed_A_AAAA", func(t *testing.T) {
		rejectDNSAnswers(t, []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("fd00::1")})
	})
	t.Run("V36.B.r01/dns_rebind", testDNSRebind)
	t.Run("V36.B.r01/redirect_private", func(t *testing.T) { testRedirect(t, "http://127.0.0.1/private") })
	t.Run("V36.B.r01/redirect_public", func(t *testing.T) { testRedirect(t, "https://other.vp.test/target") })

	t.Run("V36.B.r02/trusted_dns", func(t *testing.T) { testTrustedEndpoint(t, "https://callback.vp.test/notify") })
	t.Run("V36.B.r02/canonical_host", func(t *testing.T) { testTrustedEndpoint(t, "https://Callback.VP.Test./notify") })
	t.Run("V36.B.r02/public_ip_san", testPublicIPSAN)
	t.Run("V36.B.r02/wrong_san", func(t *testing.T) {
		rejectTLSIdentity(t, "A", []string{"other.vp.test"}, time.Now().Add(-time.Minute), time.Now().Add(time.Hour), true)
	})
	t.Run("V36.B.r02/unknown_ca", func(t *testing.T) {
		rejectTLSIdentity(t, "B", []string{"callback.vp.test"}, time.Now().Add(-time.Minute), time.Now().Add(time.Hour), true)
	})
	t.Run("V36.B.r02/expired_cert", func(t *testing.T) {
		rejectTLSIdentity(t, "A", []string{"callback.vp.test"}, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour), true)
	})
	t.Run("V36.B.r02/system_roots_only", func(t *testing.T) {
		rejectTLSIdentity(t, "A", []string{"callback.vp.test"}, time.Now().Add(-time.Minute), time.Now().Add(time.Hour), false)
	})

	t.Run("V36.B.r03/body_4096", func(t *testing.T) { testBodyLimit(t, 4096, false) })
	t.Run("V36.B.r03/body_4097", func(t *testing.T) { testBodyLimit(t, 4097, true) })
	t.Run("V36.B.r03/default_deadline", func(t *testing.T) { testMaximumDeadline(t, 0) })
	t.Run("V36.B.r03/oversized_timeout", func(t *testing.T) { testMaximumDeadline(t, time.Hour) })
	t.Run("V36.B.r03/shorter_context", testShorterContext)

	t.Run("V36.B.r04/scheme_file", func(t *testing.T) { rejectUnsafeURL(t, "file:///etc/passwd") })
	t.Run("V36.B.r04/scheme_javascript", func(t *testing.T) { rejectUnsafeURL(t, "javascript:alert(1)") })
	t.Run("V36.B.r04/userinfo", func(t *testing.T) { rejectUnsafeURL(t, "https://user@callback.vp.test/notify") })
	t.Run("V36.B.r04/port_22", func(t *testing.T) { rejectUnsafeURL(t, "https://callback.vp.test:22/notify") })
	t.Run("V36.B.r04/port_8443", func(t *testing.T) { rejectUnsafeURL(t, "https://callback.vp.test:8443/notify") })
	t.Run("V34.B.r01/real_callback_modes", testRealCallbackAttempts)
	var retrySnapshot callbackRetrySnapshot
	t.Run("V34.B.r02/v01", func(t *testing.T) {
		retrySnapshot = testCallbackRetryScheduleAndEvidence(t)
	})
	t.Run("V34.B.r03/v01", func(t *testing.T) {
		assertCallbackRetryEvidence(t, retrySnapshot)
	})
}

func testCallbackRetryScheduleAndEvidence(t *testing.T) callbackRetrySnapshot {
	database := testsupport.NewDatabase(t)
	merchantID, merchantKey := database.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, database)
	planID := database.SeedPlan(t, "callback retry schedule", 1000, 3650, []string{"alipay"}, nil)
	database.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	fixture.Network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	const secret = "callback-retry-secret"
	fixture.Network.ServeTLS(t, "callback.vp.test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "BODY_MARKER_"+strings.Repeat("x", 5000))
	}))
	start := database.Clock.Now().Truncate(time.Microsecond)
	snapshot := callbackRetrySnapshot{Attempts: make([]callbackRetryAttemptSnapshot, 0, 10)}
	order, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
		MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "retry-schedule-order", Name: "retry schedule", OriginalCents: 100,
		NotifyURL: "https://callback.vp.test/notify?token=" + secret, Channel: "alipay", SourceIP: "192.0.2.161", Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID},
	})
	if err != nil {
		t.Fatalf("create callback schedule order: %v", err)
	}
	orderID, err := fixture.App.Services.Payment.OrderID(t.Context(), order.TradeNo)
	if err != nil {
		t.Fatalf("read callback order ID: %v", err)
	}
	tx, err := database.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.App.Services.Payment.MarkPaidTx(t.Context(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("pay callback schedule order: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit callback schedule order: %v", err)
	}
	var initial time.Time
	if err := database.Pool.QueryRow(t.Context(), `SELECT next_attempt_at FROM callback_events WHERE order_id=$1 AND generation=1`, orderID).Scan(&initial); err != nil || !initial.Equal(start) {
		t.Fatalf("initial callback due time=%s err=%v, want %s", initial, err, start)
	}
	if len(fixture.Network.Evidence().LogicalDials) != 0 {
		t.Fatal("callback network fixture was used before the first scheduled attempt")
	}
	var firstAttemptID int64
	for attempt := 1; attempt <= 10; attempt++ {
		if attempt > 1 {
			var due time.Time
			if err := database.Pool.QueryRow(t.Context(), `SELECT next_attempt_at FROM callback_events WHERE order_id=$1 AND generation=1`, orderID).Scan(&due); err != nil || !due.After(database.Clock.Now()) {
				t.Fatalf("callback retry %d due time=%s current=%s err=%v", attempt, due, database.Clock.Now(), err)
			}
			database.Clock.Set(due)
		}
		if err := fixture.App.Services.Payment.ProcessCallback(t.Context(), orderID); err == nil {
			t.Fatalf("controlled HTTP 500 attempt %d unexpectedly succeeded", attempt)
		}
		var attemptID int64
		var started, finished time.Time
		var status int
		var excerpt, code, redacted string
		var duration int64
		var next *time.Time
		if err := database.Pool.QueryRow(t.Context(), `SELECT id,started_at,finished_at,http_status,response_excerpt,error_code,url_redacted,duration_ms,next_attempt_at FROM callback_attempts WHERE event_id=(SELECT id FROM callback_events WHERE order_id=$1 AND generation=1) AND attempt_no=$2`, orderID, attempt).Scan(&attemptID, &started, &finished, &status, &excerpt, &code, &redacted, &duration, &next); err != nil {
			t.Fatalf("read retry attempt %d: %v", attempt, err)
		}
		if attempt == 1 {
			firstAttemptID = attemptID
			snapshot.FirstID = attemptID
		}
		snapshot.Attempts = append(snapshot.Attempts, callbackRetryAttemptSnapshot{
			AttemptNo: attempt, ID: attemptID, StartedAt: started, FinishedAt: finished, Status: status,
			Excerpt: excerpt, ErrorCode: code, Redacted: redacted, DurationMS: duration, Next: next,
		})
		if attemptID == 0 || started.IsZero() || finished.Before(started) || status != http.StatusInternalServerError || code != "CALLBACK_NOT_CONFIRMED" || duration < 0 || strings.Contains(redacted, secret) || !strings.Contains(redacted, "<redacted>") {
			t.Fatalf("retry attempt %d evidence id=%d start=%s finish=%s status=%d code=%q duration=%d url=%q", attempt, attemptID, started, finished, status, code, duration, redacted)
		}
		if attempt < 10 {
			wantDelay := []time.Duration{0, time.Minute, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 6 * time.Hour, 14 * time.Hour}[attempt]
			if next == nil {
				t.Fatalf("retry attempt %d omitted next attempt time, want delay %s", attempt, wantDelay)
			}
			if gotDelay := next.Sub(finished); gotDelay != wantDelay {
				t.Fatalf("retry attempt %d next=%s finish=%s delay=%s, want %s", attempt, next, finished, gotDelay, wantDelay)
			}
		} else if next != nil {
			t.Fatalf("tenth retry scheduled an eleventh attempt at %s", next)
		}
		if attempt == 1 && (len(excerpt) != 4096 || !strings.HasPrefix(excerpt, "BODY_MARKER_")) {
			t.Fatalf("first callback response excerpt length=%d prefix=%q, want truncated 4096-byte body", len(excerpt), excerpt[:min(len(excerpt), 32)])
		}
	}
	if err := database.Pool.QueryRow(t.Context(), `SELECT attempt_count,state,next_attempt_at FROM callback_events WHERE order_id=$1 AND generation=1`, orderID).Scan(&snapshot.AttemptCount, &snapshot.State, &snapshot.EventNext); err != nil {
		t.Fatalf("read callback terminal state: %v", err)
	}
	var attempts int
	if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM callback_attempts WHERE event_id=(SELECT id FROM callback_events WHERE order_id=$1 AND generation=1)`, orderID).Scan(&attempts); err != nil || attempts != 10 || firstAttemptID == 0 {
		t.Fatalf("callback evidence rows=%d first_attempt=%d err=%v", attempts, firstAttemptID, err)
	}
	if len(snapshot.Attempts) != attempts {
		t.Fatalf("captured callback evidence rows=%d, database rows=%d", len(snapshot.Attempts), attempts)
	}
	return snapshot
}

func assertCallbackRetryEvidence(t *testing.T, snapshot callbackRetrySnapshot) {
	t.Helper()
	if len(snapshot.Attempts) != 10 || snapshot.AttemptCount != 10 || snapshot.State != "failed" || snapshot.EventNext != nil || snapshot.FirstID == 0 {
		t.Fatalf("callback terminal evidence attempts=%d rows=%d state=%q next=%v first_id=%d", snapshot.AttemptCount, len(snapshot.Attempts), snapshot.State, snapshot.EventNext, snapshot.FirstID)
	}
	for index, attempt := range snapshot.Attempts {
		if attempt.AttemptNo != index+1 || attempt.ID == 0 || attempt.StartedAt.IsZero() || attempt.FinishedAt.Before(attempt.StartedAt) || attempt.Status != http.StatusInternalServerError || attempt.ErrorCode != "CALLBACK_NOT_CONFIRMED" || attempt.DurationMS < 0 || strings.Contains(attempt.Redacted, "callback-retry-secret") || !strings.Contains(attempt.Redacted, "<redacted>") {
			t.Fatalf("callback retry evidence #%d = %#v", index+1, attempt)
		}
		if len(attempt.Excerpt) != 4096 || !strings.HasPrefix(attempt.Excerpt, "BODY_MARKER_") {
			t.Fatalf("callback retry excerpt #%d length=%d prefix=%q", index+1, len(attempt.Excerpt), attempt.Excerpt[:min(len(attempt.Excerpt), 32)])
		}
		if index < len(snapshot.Attempts)-1 && attempt.Next == nil {
			t.Fatalf("callback retry evidence #%d omitted next attempt time", index+1)
		}
		if index == len(snapshot.Attempts)-1 && attempt.Next != nil {
			t.Fatalf("final callback retry evidence scheduled another attempt at %s", attempt.Next)
		}
	}
}

func testRealCallbackAttempts(t *testing.T) {
	database := testsupport.NewDatabase(t)
	merchantID, merchantKey := database.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, database)
	planID := database.SeedPlan(t, "network callback plan", 1000, 3650, []string{"alipay"}, nil)
	deviceToken := database.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	network := fixture.Network
	network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	mode := "timeout"
	timeoutFinished := make(chan struct{}, 1)
	const callbackQuerySecret = "callback-query-secret-must-not-be-stored"
	network.ServeTLS(t, "callback.vp.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/notify" {
			t.Errorf("callback request route = %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse real signed callback form: %v", err)
		}
		signed := r.URL.Query()
		signed.Del("token")
		if err := payment.VerifyV1(signed, merchantKey); err != nil {
			t.Errorf("independently configured V1 callback signature was rejected: %v", err)
		}
		if signed.Get("money") != "1.00" || signed.Get("param") != "callback=round4" {
			t.Errorf("callback did not use original money/param snapshot: %#v", signed)
		}
		switch mode {
		case "timeout":
			<-r.Context().Done()
			timeoutFinished <- struct{}{}
		case "status500":
			http.Error(w, "controlled callback failure", http.StatusInternalServerError)
		case "redirect":
			http.Redirect(w, r, "https://other.vp.test/notify", http.StatusFound)
		case "wrong_body":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "unsuccessful")
		case "success":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "success")
		default:
			t.Errorf("unknown callback response mode %q", mode)
		}
	}))

	var pid string
	if err := database.Pool.QueryRow(t.Context(), `SELECT pid FROM merchants WHERE id=$1`, merchantID).Scan(&pid); err != nil {
		t.Fatal("read callback fixture PID")
	}
	params := url.Values{
		"pid": {pid}, "out_trade_no": {"callback-r04"}, "notify_url": {"https://callback.vp.test/notify?token=" + callbackQuerySecret},
		"return_url": {"https://merchant.example.test/return"}, "name": {"callback fixture"}, "money": {"1.00"},
		"clientip": {"198.51.100.50"}, "type": {"alipay"}, "param": {"callback=round4"},
	}
	signature, err := payment.SignV1(params, merchantKey)
	if err != nil {
		t.Fatal("sign real callback-order input")
	}
	params.Set("sign", signature)
	createHeaders := http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}}
	createResponse := fixture.RequestWithHeaders(t, http.MethodPost, "/mapi.php", strings.NewReader(params.Encode()), "guest", createHeaders)
	var created struct {
		Code    int    `json:"code"`
		TradeNo string `json:"trade_no"`
	}
	if createResponse.Code != http.StatusOK || json.Unmarshal(createResponse.Body.Bytes(), &created) != nil || created.Code != 1 || created.TradeNo == "" {
		t.Fatalf("real router callback order create = %d %s", createResponse.Code, createResponse.Body.String())
	}
	orderID, err := fixture.App.Services.Payment.OrderID(t.Context(), created.TradeNo)
	if err != nil {
		t.Fatal("read real callback order ID")
	}
	var actual int64
	if err := database.Pool.QueryRow(t.Context(), `SELECT actual_cents FROM orders WHERE trade_no=$1`, created.TradeNo).Scan(&actual); err != nil || actual <= 0 {
		t.Fatalf("read real callback order locked amount: amount=%d err=%v", actual, err)
	}
	deviceResponse := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", map[string]any{
		"event_id": "e0000000-0000-4000-8000-000000000003", "channel": "alipay", "amount_cents": actual,
		"occurred_at": database.Clock.Now(), "provider_trade_no": "callback-provider-r04",
	}, "device:"+deviceToken)
	if deviceResponse.Code != http.StatusCreated || !strings.Contains(deviceResponse.Body.String(), `"match_state":"matched"`) {
		t.Fatalf("real router device receipt did not pay callback order: %d %s", deviceResponse.Code, deviceResponse.Body.String())
	}

	for index, responseMode := range []string{"timeout", "status500", "redirect", "wrong_body", "success"} {
		mode = responseMode
		if index > 0 {
			if _, err := database.Pool.Exec(t.Context(), `UPDATE callback_events SET state='failed',next_attempt_at=$1 WHERE order_id=$2 AND generation=1`, database.Clock.Now(), orderID); err != nil {
				t.Fatalf("make failed callback generation due for retry %d: %v", index+1, err)
			}
		}
		err := fixture.App.Services.Payment.ProcessCallback(context.Background(), orderID)
		if responseMode == "success" && err != nil {
			t.Fatalf("process successful real callback mode: %v", err)
		}
		if responseMode != "success" && err == nil {
			t.Fatalf("failed real callback mode %s did not return its delivery error", responseMode)
		}
		if responseMode == "timeout" {
			select {
			case <-timeoutFinished:
			case <-time.After(2 * time.Second):
				t.Fatal("real callback endpoint did not observe the client deadline")
			}
		}
	}

	rows, err := database.Pool.Query(t.Context(), `SELECT attempt_no,COALESCE(http_status,0),response_excerpt,COALESCE(error_code,''),url_redacted FROM callback_attempts WHERE event_id=(SELECT id FROM callback_events WHERE order_id=$1 AND generation=1) ORDER BY attempt_no`, orderID)
	if err != nil {
		t.Fatal("read persisted callback attempts")
	}
	defer rows.Close()
	wantStatus := []int{0, http.StatusInternalServerError, http.StatusFound, http.StatusOK, http.StatusOK}
	wantCode := []string{"NETWORK_ERROR", "CALLBACK_NOT_CONFIRMED", "CALLBACK_NOT_CONFIRMED", "CALLBACK_NOT_CONFIRMED", ""}
	for index := range wantStatus {
		if !rows.Next() {
			t.Fatalf("callback attempt %d is missing", index+1)
		}
		var attempt, status int
		var excerpt, code, redactedURL string
		if err := rows.Scan(&attempt, &status, &excerpt, &code, &redactedURL); err != nil {
			t.Fatal("decode persisted callback attempt")
		}
		if attempt != index+1 || status != wantStatus[index] || code != wantCode[index] {
			t.Fatalf("callback attempt %d = attempt=%d status=%d code=%q, want status=%d code=%q", index+1, attempt, status, code, wantStatus[index], wantCode[index])
		}
		if strings.Contains(redactedURL, callbackQuerySecret) || !strings.Contains(redactedURL, "<redacted>") || strings.Contains(excerpt, callbackQuerySecret) {
			t.Fatalf("callback evidence exposed secret query material: URL=%q excerpt=%q", redactedURL, excerpt)
		}
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("callback attempt result set contains excess/error rows: %v", rows.Err())
	}
	var finalState string
	if err := database.Pool.QueryRow(t.Context(), `SELECT state FROM callback_events WHERE order_id=$1 AND generation=1`, orderID).Scan(&finalState); err != nil || finalState != "success" {
		t.Fatalf("strict final callback state = %q err=%v", finalState, err)
	}
	evidence := network.Evidence()
	if len(evidence.HTTPRequests) != 5 || len(evidence.LogicalDials) != 5 || len(evidence.ServerNames) != 5 {
		t.Fatalf("real callback TLS/HTTP evidence = %+v", evidence)
	}
	for index, serverName := range evidence.ServerNames {
		if serverName != "callback.vp.test" || evidence.LogicalDials[index] != "tcp|93.184.216.34:443" {
			t.Fatalf("callback dial %d lost validated IP/SNI: %+v", index, evidence)
		}
	}
}

func rejectLiteralTarget(t *testing.T, target string) {
	t.Helper()
	network := testsupport.NewPublicNetwork(t)
	request, err := http.NewRequest(http.MethodPost, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := network.Client(2*time.Second).DoPublic(context.Background(), request); err == nil {
		t.Fatalf("unsafe callback target was accepted: %s", target)
	}
	assertNoNetworkRequest(t, network.Evidence())
}

func rejectDNSAddress(t *testing.T, host, address string) {
	t.Helper()
	network := testsupport.NewPublicNetwork(t)
	network.SetDNS(host, []netip.Addr{netip.MustParseAddr(address)})
	_, _, err := doPublic(t, network.Client(2*time.Second), "http://"+host+"/notify")
	if err == nil {
		t.Fatalf("private DNS answer %s was accepted", address)
	}
	assertNoNetworkRequest(t, network.Evidence())
}

func rejectDNSAnswers(t *testing.T, answers []netip.Addr) {
	t.Helper()
	network := testsupport.NewPublicNetwork(t)
	network.SetDNS("callback.vp.test", answers)
	_, _, err := doPublic(t, network.Client(2*time.Second), "https://callback.vp.test/notify")
	if err == nil {
		t.Fatalf("unsafe DNS answer set was accepted: %v", answers)
	}
	assertNoNetworkRequest(t, network.Evidence())
}

func testDNSRebind(t *testing.T) {
	network := testsupport.NewPublicNetwork(t)
	network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	network.ServeTLS(t, "callback.vp.test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "controlled callback failure", http.StatusInternalServerError)
	}))
	client := network.Client(2 * time.Second)
	network.BeforeDial(func() { network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("10.0.0.1")}) })
	status, _, err := doPublic(t, client, "https://callback.vp.test/notify")
	if err != nil || status != http.StatusInternalServerError {
		t.Fatalf("first pinned request = status %d err %v, want controlled HTTP 500", status, err)
	}
	if _, _, err := doPublic(t, client, "https://callback.vp.test/notify"); err == nil {
		t.Fatal("second request accepted rebound private DNS")
	}
	evidence := network.Evidence()
	if len(evidence.LogicalDials) != 1 || len(evidence.PhysicalDials) != 1 || len(evidence.HTTPRequests) != 1 || evidence.LogicalDials[0] != "tcp|93.184.216.34:443" {
		t.Fatalf("rebind evidence = %+v, want exactly one pinned connection/request", evidence)
	}
}

func testRedirect(t *testing.T, location string) {
	t.Helper()
	network := testsupport.NewPublicNetwork(t)
	network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	network.SetDNS("other.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.36")})
	network.ServeTLS(t, "callback.vp.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, location, http.StatusFound)
	}))
	network.ServeTLS(t, "other.vp.test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "should not be followed") }))
	status, _, err := doPublic(t, network.Client(2*time.Second), "https://callback.vp.test/redirect")
	if err != nil || status != http.StatusFound {
		t.Fatalf("redirect response = status %d err %v", status, err)
	}
	evidence := network.Evidence()
	if len(evidence.HTTPRequests) != 1 || len(evidence.LogicalDials) != 1 || evidence.HTTPRequests[0].Host != "callback.vp.test" {
		t.Fatalf("redirect target was followed: %+v", evidence)
	}
}

func testTrustedEndpoint(t *testing.T, target string) {
	t.Helper()
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	network := testsupport.NewPublicNetwork(t)
	network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	network.ServeTLS(t, "callback.vp.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "success:"+r.Host)
	}))
	status, body, err := doPublic(t, network.Client(2*time.Second), target)
	if err != nil || status != http.StatusOK || !strings.HasPrefix(body, "success:") {
		t.Fatalf("trusted request = status %d body %q err %v", status, body, err)
	}
	evidence := network.Evidence()
	if len(evidence.ServerNames) != 1 || evidence.ServerNames[0] != "callback.vp.test" || len(evidence.HTTPRequests) != 1 || evidence.HTTPRequests[0].Host != parsed.Host || len(evidence.PhysicalDials) != 1 {
		t.Fatalf("trusted DNS/SNI/HTTP evidence = %+v", evidence)
	}
}

func testPublicIPSAN(t *testing.T) {
	network := testsupport.NewPublicNetwork(t)
	address := netip.MustParseAddr("93.184.216.34")
	network.SetDNS(address.String(), []netip.Addr{address})
	certificate := network.IssueCertificate(t, []string{address.String()}, "A", time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	network.ServeTLSWithCertificate(t, address.String(), certificate, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "success") }))
	status, body, err := doPublic(t, network.Client(2*time.Second), "https://"+address.String()+"/notify")
	if err != nil || status != http.StatusOK || body != "success" {
		t.Fatalf("public IP SAN request = status %d body %q err %v", status, body, err)
	}
	if evidence := network.Evidence(); len(evidence.ServerNames) != 1 || evidence.ServerNames[0] != "" || len(evidence.HTTPRequests) != 1 {
		t.Fatalf("IP literal did not use IP SAN identity: %+v", evidence)
	}
}

func rejectTLSIdentity(t *testing.T, issuer string, names []string, notBefore, notAfter time.Time, trustedRoots bool) {
	t.Helper()
	network := testsupport.NewPublicNetwork(t)
	network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	certificate := network.IssueCertificate(t, names, issuer, notBefore, notAfter)
	network.ServeTLSWithCertificate(t, "callback.vp.test", certificate, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "must not reach handler") }))
	client := network.Client(2 * time.Second)
	if !trustedRoots {
		client.RootCAs = nil
	}
	if _, _, err := doPublic(t, client, "https://callback.vp.test/notify"); err == nil {
		t.Fatal("invalid TLS identity was accepted")
	}
	evidence := network.Evidence()
	if len(evidence.ServerNames) != 1 || evidence.ServerNames[0] != "callback.vp.test" || len(evidence.HTTPRequests) != 0 || len(evidence.LogicalDials) != 1 {
		t.Fatalf("TLS rejection evidence = %+v", evidence)
	}
}

func testBodyLimit(t *testing.T, size int, wantError bool) {
	t.Helper()
	network := testsupport.NewPublicNetwork(t)
	network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	network.ServeTLS(t, "callback.vp.test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, strings.Repeat("x", size)) }))
	_, body, err := doPublic(t, network.Client(2*time.Second), "https://callback.vp.test/notify")
	if wantError && (err == nil || len(body) != 4097) {
		t.Fatalf("%d-byte response body = %d bytes, err=%v; want bounded error", size, len(body), err)
	}
	if !wantError && (err != nil || len(body) != 4096) {
		t.Fatalf("%d-byte response body = %d bytes, err=%v", size, len(body), err)
	}
}

func testMaximumDeadline(t *testing.T, timeout time.Duration) {
	t.Helper()
	network := testsupport.NewPublicNetwork(t)
	network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	startedAtEndpoint := make(chan struct{}, 1)
	endpointClosed := make(chan struct{}, 1)
	network.ServeTLS(t, "callback.vp.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		startedAtEndpoint <- struct{}{}
		<-r.Context().Done()
		endpointClosed <- struct{}{}
	}))
	started := time.Now()
	_, _, err := doPublic(t, network.Client(timeout), "https://callback.vp.test/slow-body")
	elapsed := time.Since(started)
	select {
	case <-startedAtEndpoint:
	default:
		t.Fatal("slow endpoint did not flush response headers before the deadline")
	}
	if err == nil || elapsed < 9*time.Second || elapsed > 12*time.Second {
		t.Fatalf("timeout=%s returned err=%v after %s, want the capped 10s deadline", timeout, err, elapsed)
	}
	select {
	case <-endpointClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed-out HTTP connection remained open at the server")
	}
}

func testShorterContext(t *testing.T) {
	network := testsupport.NewPublicNetwork(t)
	network.SetDNS("callback.vp.test", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	endpointStarted := make(chan struct{}, 1)
	endpointClosed := make(chan struct{}, 1)
	network.ServeTLS(t, "callback.vp.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		endpointStarted <- struct{}{}
		<-r.Context().Done()
		endpointClosed <- struct{}{}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://callback.vp.test/slow-body", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := network.Client(time.Hour).DoPublic(ctx, request)
	if err == nil && response != nil {
		_, err = io.ReadAll(response.Body)
		_ = response.Body.Close()
	}
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("earlier context deadline returned err=%v after %s", err, time.Since(started))
	}
	select {
	case <-endpointStarted:
	default:
		t.Fatal("short-context test did not reach the post-handshake response body")
	}
	select {
	case <-endpointClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled HTTP connection remained open at the server")
	}
}

func rejectUnsafeURL(t *testing.T, target string) {
	t.Helper()
	network := testsupport.NewPublicNetwork(t)
	request, err := http.NewRequest(http.MethodPost, target, nil)
	if err != nil {
		t.Fatalf("construct unsafe target request %q: %v", target, err)
	}
	if _, err := network.Client(2*time.Second).DoPublic(context.Background(), request); err == nil {
		t.Fatalf("unsafe URL was accepted: %s", target)
	}
	if evidence := network.Evidence(); len(evidence.DNSQueries) != 0 || len(evidence.LogicalDials) != 0 || len(evidence.HTTPRequests) != 0 {
		t.Fatalf("unsafe URL reached DNS/dial/HTTP: %+v", evidence)
	}
}

func assertNoNetworkRequest(t *testing.T, evidence testsupport.PublicNetworkEvidence) {
	t.Helper()
	if len(evidence.LogicalDials) != 0 || len(evidence.PhysicalDials) != 0 || len(evidence.HTTPRequests) != 0 {
		t.Fatalf("rejected target reached a dialer or HTTP endpoint: %+v", evidence)
	}
}

func doPublic(t *testing.T, client interface {
	DoPublic(context.Context, *http.Request) (*http.Response, error)
}, target string) (int, string, error) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, target, strings.NewReader("signed-payload"))
	if err != nil {
		return 0, "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.DoPublic(context.Background(), request)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = response.Body.Close() }() // Cleanup must not replace the operation result.
	body, readErr := io.ReadAll(response.Body)
	return response.StatusCode, string(body), readErr
}
