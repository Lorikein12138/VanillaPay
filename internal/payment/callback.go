package payment

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/mailtmpl"
	"vanillapay/internal/platform"
)

type callbackWork struct {
	eventID        int64
	orderID        int64
	publicEventID  string
	generation     int
	attempt        int
	protocol       string
	url            string
	keyCipher      []byte
	params         url.Values
	paramsTrade    string
	paramsOutTrade string
	paramsPID      string
	paramsChannel  string
	paramsName     string
	paramsCents    int64
	paramsExtra    string
	paramsBuyer    string
	paramsIP       string
	paramsCreated  time.Time
	paramsPaid     pgtype.Timestamptz
	paramsProvider string
	hideName       bool
	startedWall    time.Time
}

// ProcessCallback claims and executes the next due callback generation for an order.
func (s *Service) ProcessCallback(ctx context.Context, orderID int64) error {
	if orderID <= 0 {
		return errors.New("invalid callback order id")
	}
	if s.httpClient == nil {
		return platform.ErrOutboundHTTPUnavailable
	}
	var work callbackWork
	alreadyRunning := false
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var merchantID int64
		if err := tx.QueryRow(ctx, `SELECT merchant_id FROM orders WHERE id=$1`, orderID).Scan(&merchantID); err != nil {
			return err
		}
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		var state string
		var payload []byte
		var tries int
		err := tx.QueryRow(ctx, `SELECT c.id,c.public_id,c.generation,c.state,c.attempt_count,c.payload_snapshot,o.protocol,o.notify_url,o.signing_key_cipher,o.trade_no,o.out_trade_no,o.merchant_pid_snapshot,o.channel,o.name,o.original_cents,o.param,o.buyer_hint,COALESCE(host(o.clientip),''),o.created_at,o.paid_at,COALESCE((SELECT r.provider_trade_no FROM receipts r WHERE r.order_id=o.id LIMIT 1),''),(SELECT m.callback_hide_name FROM merchants m WHERE m.id=o.merchant_id) FROM callback_events c JOIN orders o ON o.id=c.order_id WHERE c.order_id=$1 AND (c.state='running' OR (c.state IN ('pending','failed') AND c.next_attempt_at<=$2)) ORDER BY c.generation DESC LIMIT 1 FOR UPDATE OF c,o`, orderID, s.clock.Now()).Scan(&work.eventID, &work.publicEventID, &work.generation, &state, &tries, &payload, &work.protocol, &work.url, &work.keyCipher, &work.paramsTrade, &work.paramsOutTrade, &work.paramsPID, &work.paramsChannel, &work.paramsName, &work.paramsCents, &work.paramsExtra, &work.paramsBuyer, &work.paramsIP, &work.paramsCreated, &work.paramsPaid, &work.paramsProvider, &work.hideName)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		var status string
		var refunded pgtype.Timestamptz
		if err := tx.QueryRow(ctx, `SELECT status,refunded_at FROM orders WHERE id=$1`, orderID).Scan(&status, &refunded); err != nil {
			return err
		}
		if status != "paid" || refunded.Valid {
			_, err := tx.Exec(ctx, `UPDATE callback_events SET state='cancelled',next_attempt_at=NULL WHERE id=$1 AND state IN ('pending','failed','running')`, work.eventID)
			return err
		}
		if state == "running" {
			var started pgtype.Timestamptz
			_ = tx.QueryRow(ctx, `SELECT started_at FROM callback_attempts WHERE event_id=$1 ORDER BY attempt_no DESC LIMIT 1`, work.eventID).Scan(&started)
			if started.Valid && started.Time.After(s.clock.Now().Add(-2*time.Minute)) {
				alreadyRunning = true
				return nil
			}
		}
		work.attempt = tries + 1
		if work.attempt > 10 {
			_, err := tx.Exec(ctx, `UPDATE callback_events SET state='failed',next_attempt_at=NULL WHERE id=$1`, work.eventID)
			return err
		}
		public, err := platform.RandomToken(s.random, 18)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE callback_events SET state='running',attempt_count=$1,next_attempt_at=NULL WHERE id=$2`, work.attempt, work.eventID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO callback_attempts(public_id,event_id,attempt_no,started_at,url_redacted) VALUES($1,$2,$3,$4,$5)`, public, work.eventID, work.attempt, s.clock.Now(), redactCallbackURL(work.url))
		if err != nil {
			return err
		}
		work.orderID = orderID
		work.params = url.Values{}
		work.params.Set("pid", work.paramsPID)
		work.params.Set("trade_no", work.paramsTrade)
		work.params.Set("out_trade_no", work.paramsOutTrade)
		work.params.Set("type", work.paramsChannel)
		work.params.Set("name", callbackName(work.paramsName, work.hideName))
		work.params.Set("money", formatMoney(work.paramsCents))
		work.params.Set("trade_status", "TRADE_SUCCESS")
		if work.paramsExtra != "" {
			work.params.Set("param", work.paramsExtra)
		}
		work.params.Set("sign_type", "MD5")
		// V1 notifications carry exactly the classic epay field set: strict
		// merchants (sub2api) reject any other key. V2 adds its extended fields.
		if work.protocol == "v2" {
			if work.paramsPaid.Valid {
				work.params.Set("endtime", work.paramsPaid.Time.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05"))
			}
			if !work.paramsCreated.IsZero() {
				work.params.Set("addtime", work.paramsCreated.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05"))
			}
			if work.paramsBuyer != "" {
				work.params.Set("buyer", work.paramsBuyer)
			}
			if work.paramsIP != "" {
				work.params.Set("clientip", work.paramsIP)
			}
			if work.paramsProvider != "" {
				work.params.Set("api_trade_no", work.paramsProvider)
			}
		}
		_ = payload
		return nil
	})
	if err != nil {
		return err
	}
	if work.eventID == 0 || alreadyRunning {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, work.url, nil)
	if err != nil {
		return s.finishCallbackAttempt(ctx, work, 0, "", "INVALID_URL", err)
	}
	// Older orders may have a query that cannot be represented in Canonical.
	// In that case deliver the signed protocol fields alone.
	if query, queryErr := mergeMerchantQuery(request.URL.RawQuery, work.params); queryErr == nil {
		work.params = query
	}
	key, err := platform.Decrypt(s.key[:], work.keyCipher)
	if err != nil {
		return s.finishCallbackAttempt(ctx, work, 0, "", "SIGNING_KEY_UNAVAILABLE", err)
	}
	switch work.protocol {
	case "v1":
		signature, signErr := SignV1(work.params, string(key))
		if signErr != nil {
			return s.finishCallbackAttempt(ctx, work, 0, "", "SIGNATURE_FAILED", signErr)
		}
		work.params.Set("sign", signature)
	case "v2":
		private, keyID, keyErr := s.platformSigningKey(ctx)
		if keyErr != nil {
			return s.finishCallbackAttempt(ctx, work, 0, "", "PLATFORM_KEY_UNAVAILABLE", keyErr)
		}
		work.params.Set("timestamp", strconv.FormatInt(s.clock.Now().Unix(), 10))
		work.params.Set("sign_type", "RSA")
		work.params.Set("key_id", keyID)
		signature, signErr := SignV2(work.params, private)
		if signErr != nil {
			return s.finishCallbackAttempt(ctx, work, 0, "", "SIGNATURE_FAILED", signErr)
		}
		work.params.Set("sign", signature)
	default:
		return s.finishCallbackAttempt(ctx, work, 0, "", "UNSUPPORTED_PROTOCOL", errors.New("unsupported callback protocol"))
	}
	request.URL.RawQuery = work.params.Encode()
	work.startedWall = time.Now()
	response, err := s.httpClient.DoPublic(ctx, request)
	if err != nil {
		// http.Client errors may contain the complete signed request URL.
		return s.finishCallbackAttempt(ctx, work, 0, "", "NETWORK_ERROR", errors.New("callback delivery failed"))
	}
	defer func() { _ = response.Body.Close() }() // Cleanup must not replace the operation result.
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 4097))
	if readErr != nil && len(body) <= 4096 {
		return s.finishCallbackAttempt(ctx, work, response.StatusCode, string(body), "RESPONSE_READ_ERROR", readErr)
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 && len(body) <= 4096 && strings.EqualFold(strings.TrimSpace(string(body)), "success") {
		return s.finishCallbackAttempt(ctx, work, response.StatusCode, string(body), "", nil)
	}
	return s.finishCallbackAttempt(ctx, work, response.StatusCode, string(body), "CALLBACK_NOT_CONFIRMED", fmt.Errorf("callback response was not strict success"))
}

// ReplayCallback creates a new persistent callback generation for a paid
// merchant order. The event and queue insert share one transaction.
func (s *Service) ReplayCallback(ctx context.Context, actor platform.Actor, trade, idempotencyKey string) (map[string]string, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return nil, apiErr("UNAUTHENTICATED", 401)
	}
	var result map[string]string
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		digest := idempotencyDigest(trade)
		oldResult, replayed, err := beginIdempotencyTx(ctx, tx, actor, "merchant.orders.callback-replay", idempotencyKey, digest, s.clock.Now())
		if err != nil {
			return err
		}
		if replayed {
			if err := json.Unmarshal(oldResult, &result); err != nil {
				return errors.New("stored callback replay result is invalid")
			}
			return nil
		}
		var orderID int64
		var status, purpose string
		var notify pgtype.Text
		if err := tx.QueryRow(ctx, `SELECT id,status,purpose,notify_url FROM orders WHERE trade_no=$1 AND merchant_id=$2 FOR UPDATE`, trade, actor.MerchantID).Scan(&orderID, &status, &purpose, &notify); err != nil {
			return apiErr("NOT_FOUND", 404)
		}
		if status == "refunded" {
			return apiErr("ALREADY_REFUNDED", 409)
		}
		if status != "paid" {
			return apiErr("ORDER_NOT_PAID", 409)
		}
		if purpose == "subscription" || !notify.Valid || notify.String == "" {
			return apiErr("CALLBACK_UNAVAILABLE", 409)
		}
		var generation int
		var state string
		err = tx.QueryRow(ctx, `SELECT generation,state FROM callback_events WHERE order_id=$1 ORDER BY generation DESC LIMIT 1 FOR UPDATE`, orderID).Scan(&generation, &state)
		if errors.Is(err, pgx.ErrNoRows) {
			return apiErr("CALLBACK_UNAVAILABLE", 409)
		}
		if err != nil {
			return err
		}
		if state == "pending" || state == "running" {
			return apiErr("CALLBACK_PENDING", 409)
		}
		if generation >= math.MaxInt32 {
			return apiErr("VERSION_CONFLICT", 409)
		}
		publicID, err := randomOpaque(s.random, 18)
		if err != nil {
			return err
		}
		var payload []byte
		if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('trade_no',trade_no,'created_at',created_at,'paid_at',paid_at) FROM orders WHERE id=$1`, orderID).Scan(&payload); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO callback_events(public_id,order_id,generation,state,next_attempt_at,payload_snapshot) VALUES($1,$2,$3,'pending',$4,$5)`, publicID, orderID, generation+1, s.clock.Now(), payload); err != nil {
			return err
		}
		if s.callbacks == nil {
			return errors.New("callback queue is unavailable")
		}
		if err := s.callbacks.EnqueueCallbackTx(ctx, tx, orderID); err != nil {
			return err
		}
		if err := auditOrderTx(ctx, tx, actor, actor.MerchantID, "callback.replay", trade, map[string]any{"generation": generation + 1}); err != nil {
			return err
		}
		result = map[string]string{"event_id": publicID, "callback_status": "pending"}
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return recordIdempotencyTx(ctx, tx, actor, "merchant.orders.callback-replay", idempotencyKey, digest, encoded, s.clock.Now())
	})
	return result, err
}

// The callback snapshot is held in private typed fields to avoid serializing sensitive data.
func (s *Service) finishCallbackAttempt(ctx context.Context, work callbackWork, status int, body, code string, cause error) error {
	if len(body) > 4096 {
		body = body[:4096]
	}
	if len(body) > 0 {
		body = strings.ToValidUTF8(body, "")
	}
	if len(code) > 80 {
		code = code[:80]
	}
	if err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var merchantID int64
		if err := tx.QueryRow(ctx, `SELECT merchant_id FROM orders WHERE id=$1`, work.orderID).Scan(&merchantID); err != nil {
			return err
		}
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		finished := s.clock.Now()
		next := any(nil)
		state := "failed"
		if cause == nil {
			state = "success"
		} else if delay, retry := callbackRetryDelay(work.attempt); retry {
			value := finished.Add(delay)
			next = value
		}
		duration := int64(0)
		if !work.startedWall.IsZero() {
			duration = time.Since(work.startedWall).Milliseconds()
		}
		if _, err := tx.Exec(ctx, `UPDATE callback_attempts SET finished_at=$1,http_status=NULLIF($2,0),duration_ms=$3,response_excerpt=$4,error_code=NULLIF($5,''),next_attempt_at=$6 WHERE event_id=$7 AND attempt_no=$8`, finished, status, duration, body, code, next, work.eventID, work.attempt); err != nil {
			return err
		}
		finalState := state
		if cause != nil && work.attempt >= 10 {
			finalState = "failed"
		}
		if _, err := tx.Exec(ctx, `UPDATE callback_events SET state=$1,next_attempt_at=$2 WHERE id=$3`, finalState, next, work.eventID); err != nil {
			return err
		}
		return s.updateCallbackAlertTx(ctx, tx, merchantID, work, cause == nil)
	}); err != nil {
		return err
	}
	return cause
}

func callbackRetryDelay(attempt int) (time.Duration, bool) {
	schedule := [...]time.Duration{0, time.Minute, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 6 * time.Hour, 14 * time.Hour}
	if attempt < 1 || attempt >= len(schedule) {
		return 0, false
	}
	return schedule[attempt], true
}

func redactCallbackURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	u.RawQuery = "<redacted>"
	u.ForceQuery = true
	return u.Scheme + "://" + u.Host + u.EscapedPath() + "?<redacted>"
}
func formatMoney(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

func (s *Service) platformSigningKey(ctx context.Context) (*rsa.PrivateKey, string, error) {
	var cipher []byte
	err := s.pool.QueryRow(ctx, `SELECT encrypted_value FROM secret_settings WHERE name='platform_rsa_private'`).Scan(&cipher)
	if err == nil {
		plain, e := platform.Decrypt(s.key[:], cipher)
		if e != nil {
			return nil, "", e
		}
		block, _ := pem.Decode(plain)
		if block == nil {
			return nil, "", errors.New("platform key PEM is invalid")
		}
		parsed, e := x509.ParsePKCS8PrivateKey(block.Bytes)
		if e != nil {
			return nil, "", e
		}
		private, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, "", errors.New("platform key is not RSA")
		}
		keyID, _, e := platformPublicKeyInfo(&private.PublicKey)
		if e != nil {
			return nil, "", e
		}
		return private, keyID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", err
	}
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", err
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return nil, "", err
	}
	cipher, err = platform.Encrypt(s.key[:], pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), s.random)
	if err != nil {
		return nil, "", err
	}
	_, _, err = platformPublicKeyInfo(&private.PublicKey)
	if err != nil {
		return nil, "", err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO secret_settings(name,encrypted_value) VALUES('platform_rsa_private',$1) ON CONFLICT(name) DO NOTHING`, cipher)
	if err != nil {
		return nil, "", err
	}
	return s.platformSigningKey(ctx)
}

func (s *Service) PlatformPublicKeys(ctx context.Context) ([]map[string]string, error) {
	private, _, err := s.platformSigningKey(ctx)
	if err != nil {
		return nil, err
	}
	keyID, publicPEM, err := platformPublicKeyInfo(&private.PublicKey)
	if err != nil {
		return nil, err
	}
	return []map[string]string{{"id": keyID, "pem": publicPEM}}, nil
}

func platformPublicKeyInfo(key *rsa.PublicKey) (string, string, error) {
	if key == nil {
		return "", "", errors.New("platform public key is unavailable")
	}
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:8]), string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

// callbackAlertThreshold is the attempt number whose failure raises the
// merchant alert. Each event can reach it once; the merchant flag keeps one
// alert per failure episode until any callback succeeds again.
const callbackAlertThreshold = 3

func (s *Service) updateCallbackAlertTx(ctx context.Context, tx pgx.Tx, merchantID int64, work callbackWork, succeeded bool) error {
	if succeeded {
		_, err := tx.Exec(ctx, `UPDATE merchants SET callback_alert_armed=true WHERE id=$1 AND NOT callback_alert_armed`, merchantID)
		return err
	}
	if work.attempt != callbackAlertThreshold {
		return nil
	}
	var episode int64
	var email, locale string
	err := tx.QueryRow(ctx, `UPDATE merchants SET callback_alert_armed=false,callback_alert_episode=callback_alert_episode+1 WHERE id=$1 AND callback_alert_armed RETURNING callback_alert_episode,email_normalized,COALESCE(preferences->>'locale','zh-CN')`, merchantID).Scan(&episode, &email, &locale)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	dedupe := fmt.Sprintf("callback_failed:%d:%d", merchantID, episode)
	public, err := randomOpaque(s.random, 18)
	if err != nil {
		return err
	}
	trade := work.paramsTrade
	payload, _ := json.Marshal(map[string]any{
		"title":   map[string]string{"zh_CN": "异步通知连续失败", "en": "Payment notifications failing"},
		"summary": map[string]string{"zh_CN": "订单 " + trade + " 的异步通知已连续 3 次失败，请检查回调地址", "en": "Notifications for order " + trade + " failed 3 times in a row. Please check your notify URL."},
		"action":  map[string]string{"page": "/dashboard/orders", "entity_id": trade},
	})
	if _, err := tx.Exec(ctx, `INSERT INTO notifications(public_id,merchant_id,kind,source_id,dedupe_key,safe_payload) VALUES($1,$2,'callback_failed',$3,$4,$5) ON CONFLICT(dedupe_key) DO NOTHING`, public, merchantID, work.publicEventID, dedupe, payload); err != nil {
		return err
	}
	return s.queueMerchantMailTx(ctx, tx, email, mailtmpl.CallbackFailed, dedupe, mailtmpl.Payload{Locale: locale, TradeNo: trade})
}

func (s *Service) queueMerchantMailTx(ctx context.Context, tx pgx.Tx, recipient, template, dedupe string, payload mailtmpl.Payload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	recipientCipher, err := platform.Encrypt(s.key[:], []byte(recipient), s.random)
	if err != nil {
		return err
	}
	payloadCipher, err := platform.Encrypt(s.key[:], data, s.random)
	if err != nil {
		return err
	}
	public, err := randomOpaque(s.random, 18)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO mail_deliveries(public_id,dedupe_key,template,recipient_cipher,payload_cipher,state) VALUES($1,$2,$3,$4,$5,'queued') ON CONFLICT(dedupe_key) DO NOTHING`, public, dedupe, template, recipientCipher, payloadCipher)
	return err
}

// callbackName is the name field merchants receive in notifications and
// signed returns. Merchants may hide the buyer-facing product name.
func callbackName(name string, hide bool) string {
	if hide {
		return "product"
	}
	return truncateUTF8(name, 127)
}
