package payment

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"vanillapay/internal/auth"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/mailtmpl"
	"vanillapay/internal/platform"
)

type CallbackQueue interface {
	EnqueueCallbackTx(context.Context, pgx.Tx, int64) error
}

type Service struct {
	pool          *pgxpool.Pool
	repository    Repository
	key           [32]byte
	publicBaseURL string
	clock         platform.Clock
	random        io.Reader
	eligibility   SQLEligibility
	fulfillment   Fulfillment
	callbacks     CallbackQueue
	httpClient    platform.SafeHTTPClient
	turnstile     auth.TurnstileVerifier
}

func NewService(pool *pgxpool.Pool, key [32]byte, publicBaseURL string, clock platform.Clock, random io.Reader, fulfillment Fulfillment, callbacks CallbackQueue) *Service {
	if clock == nil {
		clock = platform.SystemClock{}
	}
	if random == nil {
		random = rand.Reader
	}
	return &Service{pool: pool, repository: NewRepository(pool), key: key, publicBaseURL: strings.TrimRight(publicBaseURL, "/"), clock: clock, random: random, eligibility: SQLEligibility{Now: clock.Now}, fulfillment: fulfillment, callbacks: callbacks}
}

func (s *Service) SetFulfillment(value Fulfillment)            { s.fulfillment = value }
func (s *Service) SetHTTPClient(value platform.SafeHTTPClient) { s.httpClient = value }
func (s *Service) SetTurnstile(value auth.TurnstileVerifier)   { s.turnstile = value }

func (s *Service) Create(ctx context.Context, in CreateCommand) (OrderDTO, error) {
	if in.Actor.Role != "merchant" || in.Actor.MerchantID <= 0 || in.Actor.MerchantID != in.MerchantID {
		return OrderDTO{}, apiErr("UNAUTHENTICATED", 401)
	}
	if in.Purpose != "external" || (in.Protocol != "v1" && in.Protocol != "v2") {
		return OrderDTO{}, apiErr("VALIDATION_ERROR", 400)
	}
	if err := validateExternalCommand(in); err != nil {
		return OrderDTO{}, err
	}
	digest := createRequestDigest(in)
	var result OrderDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{in.MerchantID}, false); err != nil {
			return err
		}
		sourceIP := protocolSourceIP(ctx, in.SourceIP)
		if err := s.eligibility.IPAllowed(ctx, tx, in.MerchantID, sourceIP); err != nil {
			return err
		}
		var banned bool
		if err := tx.QueryRow(ctx, `SELECT banned_at IS NOT NULL FROM merchants WHERE id=$1`, in.MerchantID).Scan(&banned); err != nil {
			return err
		}
		if banned {
			return apiErr("ACCOUNT_BANNED", 403)
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("order:%d:%s", in.MerchantID, in.OutTradeNo)); err != nil {
			return err
		}
		existing, found, err := s.reusableOrder(ctx, tx, in, digest)
		if err != nil {
			return err
		}
		risk, err := loadCreateRisk(ctx, tx)
		if err != nil {
			return err
		}
		if err := enforceCreateRisk(ctx, tx, in, risk); err != nil {
			return err
		}
		if found {
			result = existing
			return nil
		}
		snapshot, err := s.eligibility.CheckCreateTx(ctx, tx, in.MerchantID, in.OriginalCents)
		if err != nil {
			return err
		}
		channels := intersectChannels(snapshot.Channels, in.Channels, snapshot.QRAssets)
		if len(channels) == 0 {
			return apiErr("QR_UNAVAILABLE", 422)
		}
		seconds := snapshot.DefaultTTLSeconds
		if snapshot.MerchantTTL != nil {
			seconds = *snapshot.MerchantTTL
		}
		if in.ExpireSeconds != nil {
			seconds = *in.ExpireSeconds
		}
		if seconds < 1 || seconds > snapshot.MaxTTLSeconds {
			return apiErr("TTL_OUT_OF_RANGE", 400)
		}
		tradeNo, err := randomTradeNo(s.random)
		if err != nil {
			return err
		}
		createdAt, err := checkCreateLimits(ctx, tx, in, snapshot.PlanLimits, risk.IPDailyOrderLimit, s.clock.Now)
		if err != nil {
			return err
		}
		expires := createdAt.Add(time.Duration(seconds) * time.Second)
		channel := ""
		actual := (*int64)(nil)
		var asset *string
		var offset *int
		if in.Channel != "" {
			channel = in.Channel
			if !containsChannel(channels, channel) {
				return apiErr("CHANNEL_NOT_ALLOWED", 403)
			}
			value, off, err := s.reserveAmountTx(ctx, tx, in.MerchantID, channel, in.OriginalCents, snapshot)
			if err != nil {
				return err
			}
			actual = &value
			offset = &off
			id := snapshot.QRAssets[channel]
			asset = &id
		}
		snapshot.Channels = channels
		orderID, created, err := s.orderCreateTx(ctx, tx, in.MerchantID, tradeNo, in, digest, snapshot, createdAt, expires, channel, actual, offset, asset)
		if err != nil {
			return err
		}
		if actual != nil {
			if _, err := tx.Exec(ctx, `INSERT INTO amount_reservations(merchant_id,channel,actual_cents,order_id) VALUES($1,$2,$3,$4)`, in.MerchantID, channel, *actual, orderID); err != nil {
				return apiErr("AMOUNT_CAPACITY_EXHAUSTED", 409)
			}
		}
		result = OrderDTO{TradeNo: tradeNo, OutTradeNo: in.OutTradeNo, MerchantID: strconv.FormatInt(in.MerchantID, 10), Purpose: in.Purpose, Protocol: in.Protocol, Name: truncateUTF8(in.Name, 127), ProductID: in.ProductID, Param: in.Param, OriginalCents: in.OriginalCents, Status: "pending", CreatedAt: created, ExpiresAt: expires, CallbackStatus: "none"}
		if channel != "" {
			v := channel
			result.Channel = &v
		}
		if actual != nil {
			v := *actual
			result.ActualCents = &v
		}
		return auditOrderTx(ctx, tx, in.Actor, in.MerchantID, "order.create", tradeNo, map[string]any{"protocol": in.Protocol, "channel": channel})
	})
	return result, err
}

// CreateSubscriptionOrderTx is the only internal purchase creation port. The
// caller supplies a validated purchase row and the shared transaction.
func (s *Service) CreateSubscriptionOrderTx(ctx context.Context, tx pgx.Tx, in BillingOrderCommand) (BillingOrderResult, error) {
	if in.PurchaseID <= 0 || in.BuyerID <= 0 || in.BillingMerchantID <= 0 || in.AmountCents <= 0 || len(in.Channels) == 0 {
		return BillingOrderResult{}, apiErr("BILLING_NOT_READY", 422)
	}
	snapshot, err := s.eligibility.CheckCreateTx(ctx, tx, in.BillingMerchantID, in.AmountCents)
	if err != nil {
		return BillingOrderResult{}, apiErr("BILLING_NOT_READY", 422)
	}
	channels := intersectChannels(snapshot.Channels, in.Channels, snapshot.QRAssets)
	if len(channels) == 0 {
		return BillingOrderResult{}, apiErr("BILLING_NOT_READY", 422)
	}
	tradeNo, err := randomTradeNo(s.random)
	if err != nil {
		return BillingOrderResult{}, err
	}
	expires := in.ExpiresAt
	if !expires.After(s.clock.Now()) {
		return BillingOrderResult{}, apiErr("BILLING_NOT_READY", 422)
	}
	var orderID int64
	var created time.Time
	var keyCipher []byte
	var keyVersion int32
	if err := tx.QueryRow(ctx, `SELECT key_cipher,key_version FROM merchants WHERE id=$1 AND banned_at IS NULL`, in.BillingMerchantID).Scan(&keyCipher, &keyVersion); err != nil {
		return BillingOrderResult{}, err
	}
	if err := tx.QueryRow(ctx, `INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,product_id,param,original_cents,channel,allowed_channels_snapshot,notify_url,return_url,signing_key_version,signing_key_cipher,merchant_username_snapshot,merchant_pid_snapshot,buyer_id,purchase_id,expires_at,status,created_at) SELECT $1,m.id,$2,'subscription','internal',$3,'','',$4,NULL,$5,NULL,NULL,$6,$7,m.username,m.pid,$8,$9,$10,'pending',$12 FROM merchants m WHERE m.id=$11 RETURNING id,created_at`, tradeNo, "sub_"+in.PublicPurchaseID, in.Name, in.AmountCents, channels, keyVersion, keyCipher, in.BuyerID, in.PurchaseID, expires, in.BillingMerchantID, s.clock.Now()).Scan(&orderID, &created); err != nil {
		return BillingOrderResult{}, err
	}
	_ = created
	return BillingOrderResult{TradeNo: tradeNo, CashierURL: "/pay/" + tradeNo, OrderID: orderID}, nil
}

func (s *Service) CancelSubscriptionOrderTx(ctx context.Context, tx pgx.Tx, orderID int64) error {
	tag, err := tx.Exec(ctx, `UPDATE orders SET status='expired',expire_reason='cancelled',updated_at=$1 WHERE id=$2 AND purpose='subscription' AND status='pending'`, s.clock.Now(), orderID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		err = scheduleReservationRelease(ctx, tx, orderID)
	}
	return err
}

func (s *Service) SelectChannel(ctx context.Context, tradeNo, channel string) (CheckoutDTO, error) {
	return s.selectChannel(ctx, tradeNo, channel, "", "")
}

func (s *Service) selectChannel(ctx context.Context, tradeNo, channel, ip, token string) (CheckoutDTO, error) {
	if !platform.ValidPaymentChannel(channel) {
		return CheckoutDTO{}, apiErr("VALIDATION_ERROR", 400)
	}
	var merchantID, orderID int64
	if err := s.pool.QueryRow(ctx, `SELECT id,merchant_id FROM orders WHERE trade_no=$1`, tradeNo).Scan(&orderID, &merchantID); err != nil {
		return CheckoutDTO{}, apiErr("NOT_FOUND", 404)
	}
	var dto CheckoutDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		risk, err := readCheckoutRisk(ctx, tx, ip)
		if err != nil {
			return err
		}
		var status, purpose, protocol, name, product, param string
		var original int64
		var allowed []string
		var expires, created time.Time
		var currentChannel, currentAsset pgtype.Text
		var actual pgtype.Int8
		var existingOffset pgtype.Int4
		if err := tx.QueryRow(ctx, `SELECT status,purpose,protocol,name,product_id,param,original_cents,allowed_channels_snapshot,expires_at,created_at,channel,qr_asset_id,actual_cents,amount_offset_cents FROM orders WHERE id=$1 AND merchant_id=$2 FOR UPDATE`, orderID, merchantID).Scan(&status, &purpose, &protocol, &name, &product, &param, &original, &allowed, &expires, &created, &currentChannel, &currentAsset, &actual, &existingOffset); err != nil {
			return err
		}
		if status != "pending" {
			return apiErr("ORDER_NOT_PAYABLE", 409)
		}
		if !expires.After(s.clock.Now()) {
			if _, err := tx.Exec(ctx, `UPDATE orders SET status='expired',expire_reason='timeout',updated_at=$1 WHERE id=$2`, s.clock.Now(), orderID); err != nil {
				return err
			}
			return apiErr("ORDER_NOT_PAYABLE", 409)
		}
		if risk.SiteKey != "" && !s.checkoutVerified(tradeNo, token, expires) {
			return apiErr("CHECKOUT_VERIFICATION_REQUIRED", 403)
		}
		if currentChannel.Valid {
			if currentChannel.String != channel {
				return apiErr("CHANNEL_LOCKED", 409)
			}
			v := actual.Int64
			asset := currentAsset.String
			return s.checkoutDTO(ctx, tx, tradeNo, orderID, merchantID, name, product, original, &v, &currentChannel.String, &asset, allowed, expires, &dto)
		}
		snapshot, err := s.eligibility.CheckChannelTx(ctx, tx, merchantID, channel, original)
		if err != nil {
			return err
		}
		if !containsChannel(allowed, channel) {
			return apiErr("CHANNEL_NOT_ALLOWED", 403)
		}
		offsetLimit := snapshot.FloatDefaultCents
		if snapshot.MerchantFloatLimit != nil {
			offsetLimit = *snapshot.MerchantFloatLimit
		}
		direction := snapshot.FloatDirection
		if snapshot.MerchantFloatDirection != nil {
			direction = *snapshot.MerchantFloatDirection
		}
		occupied := map[int64]struct{}{}
		if _, err := tx.Exec(ctx, `DELETE FROM amount_reservations WHERE merchant_id=$1 AND release_after IS NOT NULL AND release_after<=$2`, merchantID, s.clock.Now()); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT actual_cents FROM amount_reservations WHERE merchant_id=$1 AND channel=$2 FOR UPDATE`, merchantID, channel)
		if err != nil {
			return err
		}
		for rows.Next() {
			var n int64
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				return err
			}
			occupied[n] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		value, err := AllocateAmount(original, direction, offsetLimit, int(snapshot.MinAmountCents), int(snapshot.MaxAmountCents), occupied)
		if err != nil {
			return apiErr("AMOUNT_CAPACITY_EXHAUSTED", 409)
		}
		delta := int(value - original)
		asset := snapshot.QRAssets[channel]
		tag, err := tx.Exec(ctx, `UPDATE orders SET channel=$1,actual_cents=$2,amount_offset_cents=$3,qr_asset_id=$4,channel_locked_at=$5,settings_version=(SELECT version FROM site_settings WHERE singleton=true),collection_version=(SELECT version FROM merchant_collection_settings WHERE merchant_id=$7),updated_at=$5 WHERE id=$6 AND status='pending' AND channel IS NULL`, channel, value, delta, asset, s.clock.Now(), orderID, merchantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return apiErr("CHANNEL_LOCKED", 409)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO amount_reservations(merchant_id,channel,actual_cents,order_id) VALUES($1,$2,$3,$4)`, merchantID, channel, value, orderID); err != nil {
			return apiErr("AMOUNT_CAPACITY_EXHAUSTED", 409)
		}
		actualValue := value
		return s.checkoutDTO(ctx, tx, tradeNo, orderID, merchantID, name, product, original, &actualValue, &channel, &asset, allowed, expires, &dto)
	})
	return dto, err
}

func (s *Service) checkoutDTO(ctx context.Context, tx pgx.Tx, trade string, orderID, merchantID int64, name, product string, original int64, actual *int64, channel, asset *string, allowed []string, expires time.Time, dto *CheckoutDTO) error {
	var username string
	var banned bool
	if err := tx.QueryRow(ctx, `SELECT username,banned_at IS NOT NULL FROM merchants WHERE id=$1`, merchantID).Scan(&username, &banned); err != nil {
		return err
	}
	var online bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL AND last_seen_at>$2::timestamptz-interval '90 seconds')`, merchantID, s.clock.Now()).Scan(&online); err != nil {
		return err
	}
	var status, reason string
	if err := tx.QueryRow(ctx, `SELECT status,COALESCE(expire_reason,'') FROM orders WHERE id=$1`, orderID).Scan(&status, &reason); err != nil {
		return err
	}
	dto.TradeNo = trade
	dto.Name = name
	dto.ProductID = product
	dto.Merchant = map[string]string{"id": strconv.FormatInt(merchantID, 10), "username": username}
	dto.Status = status
	dto.OriginalCents = original
	dto.ActualCents = actual
	dto.Channel = channel
	dto.AvailableChannels = append([]string{}, allowed...)
	dto.ExpiresAt = expires
	dto.ServerTime = s.clock.Now()
	dto.DeviceOnline = online
	dto.RequiresExactAmount = true
	dto.CanPay = status == "pending" && expires.After(s.clock.Now()) && !banned && online
	if banned {
		reason = "merchant_banned"
	} else if !online {
		reason = "device_offline"
	} else if status == "expired" {
		reason = "expired"
	}
	if reason != "" {
		dto.Reason = &reason
	}
	if asset != nil && *asset != "" && channel != nil && dto.CanPay {
		sigExpiry := expires
		expiresText := strconv.FormatInt(sigExpiry.Unix(), 10)
		signature := signCheckoutMedia(s.key[:], trade, *asset, sigExpiry)
		url := fmt.Sprintf("/media/%s?trade_no=%s&expires=%s&signature=%s", url.PathEscape(*asset), url.QueryEscape(trade), expiresText, signature)
		dto.QRImageURL = &url
	}
	return nil
}

func (s *Service) Query(ctx context.Context, actor platform.Actor, selector OrderSelector) (OrderDTO, error) {
	if actor.Role != "merchant" && actor.Role != "admin" {
		return OrderDTO{}, apiErr("UNAUTHENTICATED", 401)
	}
	if actor.Role == "merchant" {
		selector.MerchantID = actor.MerchantID
	}
	if selector.TradeNo == "" && selector.OutTradeNo == "" || selector.MerchantID <= 0 {
		return OrderDTO{}, apiErr("VALIDATION_ERROR", 400)
	}
	var d OrderDTO
	var ch, reason pgtype.Text
	var actualCents pgtype.Int8
	var created, expires time.Time
	var paid, refunded pgtype.Timestamptz
	err := s.pool.QueryRow(ctx, `SELECT o.trade_no,o.out_trade_no,o.merchant_id,o.purpose,o.protocol,o.channel,o.name,o.product_id,o.param,o.original_cents,o.actual_cents,o.status,o.expire_reason,o.created_at,o.paid_at,o.expires_at,o.refunded_at,COALESCE((SELECT c.state FROM callback_events c WHERE c.order_id=o.id ORDER BY c.generation DESC LIMIT 1),'none') FROM orders o WHERE o.merchant_id=$1 AND ($2='' OR o.trade_no=$2) AND ($3='' OR o.out_trade_no=$3)`, selector.MerchantID, selector.TradeNo, selector.OutTradeNo).Scan(&d.TradeNo, &d.OutTradeNo, &d.MerchantID, &d.Purpose, &d.Protocol, &ch, &d.Name, &d.ProductID, &d.Param, &d.OriginalCents, &actualCents, &d.Status, &reason, &created, &paid, &expires, &refunded, &d.CallbackStatus)
	if err != nil {
		return OrderDTO{}, apiErr("NOT_FOUND", 404)
	}
	d.CreatedAt = created
	d.ExpiresAt = expires
	if ch.Valid {
		v := ch.String
		d.Channel = &v
	}
	if actualCents.Valid {
		v := actualCents.Int64
		d.ActualCents = &v
	}
	if reason.Valid {
		v := reason.String
		d.ExpireReason = &v
	}
	if paid.Valid {
		v := paid.Time
		d.PaidAt = &v
	}
	if refunded.Valid {
		v := refunded.Time
		d.RefundedAt = &v
	}
	if d.Status == "pending" && !expires.After(s.clock.Now()) {
		_ = s.expireOne(ctx, d.TradeNo)
		d.Status = "expired"
		v := "timeout"
		d.ExpireReason = &v
	}
	return d, nil
}

func (s *Service) MarkPaidTx(ctx context.Context, tx pgx.Tx, orderID int64, in PaidCommand) error {
	var merchantID, affectedMerchantID int64
	var status, purpose, channel, expireReason string
	var actual pgtype.Int8
	var expires time.Time
	var notify pgtype.Text
	var trade string
	if err := tx.QueryRow(ctx, `SELECT merchant_id,COALESCE(buyer_id,merchant_id),status,purpose,COALESCE(channel,''),actual_cents,expires_at,notify_url,trade_no,COALESCE(expire_reason,'') FROM orders WHERE id=$1 FOR UPDATE`, orderID).Scan(&merchantID, &affectedMerchantID, &status, &purpose, &channel, &actual, &expires, &notify, &trade, &expireReason); err != nil {
		return err
	}
	if status == "paid" {
		return nil
	}
	if in.Manual {
		if status != "pending" && (status != "expired" || expireReason == "cancelled" && purpose == "external") {
			return apiErr("ORDER_NOT_PAYABLE", 409)
		}
	} else if status != "pending" || !expires.After(s.clock.Now()) {
		return apiErr("ORDER_NOT_PAYABLE", 409)
	}
	if !platform.ValidPaymentChannel(in.Channel) {
		return apiErr("VALIDATION_ERROR", 400)
	}
	if channel != "" && channel != in.Channel {
		return apiErr("CHANNEL_LOCKED", 409)
	}
	if !actual.Valid {
		var original int64
		if err := tx.QueryRow(ctx, `SELECT original_cents FROM orders WHERE id=$1`, orderID).Scan(&original); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET channel=$1,actual_cents=$2,amount_offset_cents=0 WHERE id=$3`, in.Channel, original, orderID); err != nil {
			return err
		}
		channel = in.Channel
	}
	at := s.clock.Now()
	tag, err := tx.Exec(ctx, `UPDATE orders SET status='paid',expire_reason=NULL,paid_at=$1,updated_at=$1,is_manual=$2,manual_reason=NULLIF($3,''),manual_actor_id=$4,buyer_hint=COALESCE(NULLIF($5,''),buyer_hint) WHERE id=$6 AND status IN ('pending','expired') AND ($7 OR expires_at>$1)`, at, in.Manual, in.ManualReason, in.ActorID, in.BuyerHint, orderID, in.Manual)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apiErr("ORDER_NOT_PAYABLE", 409)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM amount_reservations WHERE order_id=$1`, orderID); err != nil {
		return err
	}
	if purpose == "subscription" {
		if s.fulfillment == nil {
			return errors.New("subscription fulfillment port is not connected")
		}
		if err := s.fulfillment.PaidTx(ctx, tx, orderID, at); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO callback_events(public_id,order_id,generation,state,payload_snapshot) VALUES($1,$2,1,'internal_fulfillment','{}') ON CONFLICT(order_id,generation) DO NOTHING`, randomID(s.random), orderID)
		if err != nil {
			return err
		}
		actor := paidActor(ctx)
		return auditOrderTx(ctx, tx, actor, affectedMerchantID, "order.paid", trade, map[string]any{"manual": in.Manual, "channel": channel, "reason": in.ManualReason})
	}
	if notify.Valid && notify.String != "" {
		if err := s.scheduleCallbackTx(ctx, tx, orderID, trade, at); err != nil {
			return err
		}
		if s.callbacks != nil {
			if err := s.callbacks.EnqueueCallbackTx(ctx, tx, orderID); err != nil {
				return err
			}
		}
	}
	return auditOrderTx(ctx, tx, paidActor(ctx), merchantID, "order.paid", trade, map[string]any{"manual": in.Manual, "channel": channel, "reason": in.ManualReason})
}

func (s *Service) Supplement(ctx context.Context, actor platform.Actor, trade string, in SupplementRequest) error {
	return s.SupplementIdempotent(ctx, actor, trade, in, "")
}

func (s *Service) SupplementIdempotent(ctx context.Context, actor platform.Actor, trade string, in SupplementRequest, idempotencyKey string) error {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return apiErr("UNAUTHENTICATED", 401)
	}
	if len([]rune(strings.TrimSpace(in.Reason))) < 1 || len([]rune(in.Reason)) > 500 {
		return apiErr("VALIDATION_ERROR", 400)
	}
	ctx = platform.WithActor(ctx, actor)
	var orderID int64
	var merchantID int64
	if err := s.pool.QueryRow(ctx, `SELECT id,merchant_id FROM orders WHERE trade_no=$1`, trade).Scan(&orderID, &merchantID); err != nil || merchantID != actor.MerchantID {
		return apiErr("NOT_FOUND", 404)
	}
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		var digest []byte
		if idempotencyKey != "" {
			digest = idempotencyDigest(trade, strings.TrimSpace(in.Reason), in.Channel)
			if _, replayed, err := beginIdempotencyTx(ctx, tx, actor, "merchant.orders.supplement", idempotencyKey, digest, s.clock.Now()); err != nil || replayed {
				return err
			}
		}
		var banned bool
		if err := tx.QueryRow(ctx, `SELECT banned_at IS NOT NULL FROM merchants WHERE id=$1 FOR UPDATE`, merchantID).Scan(&banned); err != nil {
			return err
		}
		if banned {
			return apiErr("ACCOUNT_BANNED", 403)
		}
		var status, purpose, expireReason string
		var channel pgtype.Text
		var original int64
		var allowed []string
		if err := tx.QueryRow(ctx, `SELECT status,purpose,channel,original_cents,allowed_channels_snapshot,COALESCE(expire_reason,'') FROM orders WHERE id=$1 FOR UPDATE`, orderID).Scan(&status, &purpose, &channel, &original, &allowed, &expireReason); err != nil {
			return err
		}
		if status != "pending" && (status != "expired" || expireReason == "cancelled" && purpose == "external") {
			return apiErr("ORDER_NOT_PAYABLE", 409)
		}
		selected := in.Channel
		if selected == "" && channel.Valid {
			selected = channel.String
		}
		if !platform.ValidPaymentChannel(selected) || !containsChannel(allowed, selected) {
			return apiErr("CHANNEL_NOT_ALLOWED", 403)
		}
		if !channel.Valid {
			if _, err := tx.Exec(ctx, `UPDATE orders SET channel=$1,actual_cents=original_cents,amount_offset_cents=0 WHERE id=$2`, selected, orderID); err != nil {
				return err
			}
		} else if channel.String != selected {
			return apiErr("CHANNEL_LOCKED", 409)
		}
		if err := s.MarkPaidTx(ctx, tx, orderID, PaidCommand{Channel: selected, Manual: true, ManualReason: strings.TrimSpace(in.Reason), ActorID: &actor.SubjectID}); err != nil {
			return err
		}
		if idempotencyKey != "" {
			return recordIdempotencyTx(ctx, tx, actor, "merchant.orders.supplement", idempotencyKey, digest, []byte(`{"completed":true}`), s.clock.Now())
		}
		return nil
	})
}

func (s *Service) ExpirePendingForBannedTx(ctx context.Context, tx pgx.Tx, merchantID int64) (int64, error) {
	tag, err := tx.Exec(ctx, `UPDATE orders SET status='expired',expire_reason='banned',updated_at=$1 WHERE merchant_id=$2 AND status='pending'`, s.clock.Now(), merchantID)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE amount_reservations r SET release_after=o.expires_at+interval '24 hours' FROM orders o WHERE r.order_id=o.id AND o.merchant_id=$1 AND o.status='expired' AND o.expire_reason='banned'`, merchantID); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Service) ExpireBatch(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT id,merchant_id FROM orders WHERE status='pending' AND expires_at<=$1 ORDER BY expires_at,id LIMIT $2`, s.clock.Now(), limit)
	if err != nil {
		return 0, err
	}
	type item struct{ id, merchant int64 }
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.merchant); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	count := 0
	for _, it := range items {
		changed := false
		err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			if err := sqlc.LockBusinessRows(ctx, tx, []int64{it.merchant}, false); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `UPDATE orders SET status='expired',expire_reason='timeout',updated_at=$1 WHERE id=$2 AND status='pending' AND expires_at<=$1`, s.clock.Now(), it.id)
			if err != nil {
				return err
			}
			changed = tag.RowsAffected() == 1
			if changed {
				err = scheduleReservationRelease(ctx, tx, it.id)
			}
			return err
		})
		if err != nil {
			return count, err
		}
		if changed {
			count++
		}
	}
	return count, nil
}

func (s *Service) expireOne(ctx context.Context, trade string) error {
	var merchantID int64
	if err := s.pool.QueryRow(ctx, `SELECT merchant_id FROM orders WHERE trade_no=$1`, trade).Scan(&merchantID); err != nil {
		return err
	}
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE orders SET status='expired',expire_reason='timeout',updated_at=$1 WHERE trade_no=$2 AND status='pending' AND expires_at<=$1`, s.clock.Now(), trade)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			_, err = tx.Exec(ctx, `UPDATE amount_reservations SET release_after=(SELECT expires_at+interval '24 hours' FROM orders WHERE trade_no=$1) WHERE order_id=(SELECT id FROM orders WHERE trade_no=$1)`, trade)
		}
		return err
	})
}

func (s *Service) RequestRefund(ctx context.Context, in RefundCommand) (RefundDTO, error) {
	if err := validateRefundRequest(in); err != nil {
		return RefundDTO{}, err
	}
	var orderID int64
	var d RefundDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{in.MerchantID}, false); err != nil {
			return err
		}
		var amount int64
		var state string
		var trade string
		if err := tx.QueryRow(ctx, `SELECT id,original_cents,status,trade_no FROM orders WHERE merchant_id=$1 AND trade_no=$2 FOR UPDATE`, in.MerchantID, in.TradeNo).Scan(&orderID, &amount, &state, &trade); err != nil {
			return apiErr("NOT_FOUND", 404)
		}
		if state == "refunded" {
			return apiErr("ALREADY_REFUNDED", 409)
		}
		if state != "paid" {
			return apiErr("ORDER_NOT_PAID", 409)
		}
		if in.AmountCents != amount {
			return apiErr("PARTIAL_REFUND_UNSUPPORTED", 400)
		}
		var old []byte
		var oldID string
		var oldStatus string
		err := tx.QueryRow(ctx, `SELECT public_id,request_digest,status FROM refunds WHERE order_id=$1 AND status='pending' FOR UPDATE`, orderID).Scan(&oldID, &old, &oldStatus)
		if err == nil {
			if len(in.RequestDigest) > 0 && subtle.ConstantTimeCompare(old, in.RequestDigest) != 1 {
				return apiErr("REFUND_PENDING", 409)
			}
			d = RefundDTO{ID: oldID, TradeNo: trade, AmountCents: amount, Status: oldStatus}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		id, err := randomOpaque(s.random, 20)
		if err != nil {
			return err
		}
		if in.OutRefundNo != "" {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM refunds WHERE merchant_id=$1 AND out_refund_no=$2)`, in.MerchantID, in.OutRefundNo).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return apiErr("IDEMPOTENCY_CONFLICT", 409)
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO refunds(public_id,order_id,out_refund_no,requested_cents,status,reason,request_digest,merchant_id) VALUES($1,$2,NULLIF($3,''),$4,'pending',NULLIF($5,''),$6,$7)`, id, orderID, in.OutRefundNo, amount, in.Reason, in.RequestDigest, in.MerchantID)
		if err != nil {
			return err
		}
		if err := s.createRefundNotificationTx(ctx, tx, in.MerchantID, id, trade); err != nil {
			return err
		}
		if err := s.createRefundMailTx(ctx, tx, in.MerchantID, id, trade); err != nil {
			return err
		}
		d = RefundDTO{ID: id, TradeNo: trade, AmountCents: amount, Status: "pending", RequestedAt: s.clock.Now()}
		if in.OutRefundNo != "" {
			v := in.OutRefundNo
			d.OutRefundNo = &v
		}
		if in.Reason != "" {
			v := in.Reason
			d.Reason = &v
		}
		return auditOrderTx(ctx, tx, in.Actor, in.MerchantID, "refund.request", trade, map[string]any{"refund_id": id})
	})
	return d, err
}

func validateRefundRequest(in RefundCommand) error {
	if in.MerchantID <= 0 || in.Actor.Role != "merchant" || in.Actor.MerchantID != in.MerchantID {
		return apiErr("UNAUTHENTICATED", 401)
	}
	if strings.TrimSpace(in.TradeNo) == "" || in.AmountCents <= 0 || len([]rune(in.Reason)) > 500 || len(in.OutRefundNo) > 64 {
		return apiErr("VALIDATION_ERROR", 400)
	}
	return nil
}

func (s *Service) ResolveRefund(ctx context.Context, actor platform.Actor, refundID string, in RefundResolution) error {
	_, err := s.ResolveRefundIdempotent(ctx, actor, refundID, in, "")
	return err
}

func (s *Service) ResolveRefundIdempotent(ctx context.Context, actor platform.Actor, refundID string, in RefundResolution, key string) (RefundDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return RefundDTO{}, apiErr("UNAUTHENTICATED", 401)
	}
	if in.Action != "reject" && in.Action != "mark_refunded" || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 || in.Action == "mark_refunded" && !in.Confirm {
		return RefundDTO{}, apiErr("VALIDATION_ERROR", 400)
	}
	const scope = "merchant.refunds.resolve"
	target := "rejected"
	if in.Action == "mark_refunded" {
		target = "completed"
	}
	ctx = platform.WithActor(ctx, actor)
	var dto RefundDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		var digest []byte
		if key != "" {
			digest = idempotencyDigest(refundID, in.Action, in.Reason, strconv.FormatBool(in.Confirm))
			result, replayed, err := beginIdempotencyTx(ctx, tx, actor, scope, key, digest, s.clock.Now())
			if err != nil {
				return err
			}
			if replayed {
				return json.Unmarshal(result, &dto)
			}
		}
		var orderID int64
		var orderStatus string
		err := tx.QueryRow(ctx, `SELECT r.order_id,o.status,r.public_id,o.trade_no,r.out_refund_no,r.requested_cents,r.status,r.reason,r.requested_at,r.handled_at,r.handled_by::text
			FROM refunds r JOIN orders o ON o.id=r.order_id WHERE r.public_id=$1 AND o.merchant_id=$2 FOR UPDATE OF r,o`, refundID, actor.MerchantID).
			Scan(&orderID, &orderStatus, &dto.ID, &dto.TradeNo, &dto.OutRefundNo, &dto.AmountCents, &dto.Status, &dto.Reason, &dto.RequestedAt, &dto.HandledAt, &dto.HandledBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return apiErr("NOT_FOUND", 404)
		}
		if err != nil {
			return err
		}
		switch dto.Status {
		case target:
			// A retry with a different key still observes the already-applied result.
		case "pending":
			if orderStatus != "paid" {
				return apiErr("ORDER_NOT_PAID", 409)
			}
			if err := s.applyRefundResolutionTx(ctx, tx, actor, orderID, target, strings.TrimSpace(in.Reason), &dto); err != nil {
				return err
			}
		default:
			return apiErr("REFUND_ALREADY_HANDLED", 409)
		}
		if key == "" {
			return nil
		}
		result, err := json.Marshal(dto)
		if err != nil {
			return err
		}
		return recordIdempotencyTx(ctx, tx, actor, scope, key, digest, result, s.clock.Now())
	})
	return dto, err
}

func (s *Service) applyRefundResolutionTx(ctx context.Context, tx pgx.Tx, actor platform.Actor, orderID int64, target, reason string, dto *RefundDTO) error {
	// public_id is unique. RETURNING requires exactly one pending row to be updated.
	err := tx.QueryRow(ctx, `UPDATE refunds SET status=$1,reason=$2,handled_at=$3,handled_by=$4 WHERE public_id=$5 AND status='pending'
		RETURNING status,reason,handled_at,handled_by::text`, target, reason, s.clock.Now(), actor.SubjectID, dto.ID).
		Scan(&dto.Status, &dto.Reason, &dto.HandledAt, &dto.HandledBy)
	if err != nil {
		return fmt.Errorf("update pending refund: %w", err)
	}
	if target == "completed" {
		return s.markRefundedTx(ctx, tx, orderID, actor, reason)
	}
	return auditOrderTx(ctx, tx, actor, actor.MerchantID, "refund.reject", dto.ID, map[string]any{})
}

func (s *Service) MarkRefunded(ctx context.Context, actor platform.Actor, trade, reason string) error {
	return s.MarkRefundedIdempotent(ctx, actor, trade, reason, "")
}

func (s *Service) MarkRefundedIdempotent(ctx context.Context, actor platform.Actor, trade, reason, idempotencyKey string) error {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return apiErr("UNAUTHENTICATED", 401)
	}
	if len(strings.TrimSpace(reason)) < 1 || len(reason) > 500 {
		return apiErr("VALIDATION_ERROR", 400)
	}
	ctx = platform.WithActor(ctx, actor)
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		var digest []byte
		if idempotencyKey != "" {
			digest = idempotencyDigest(trade, reason)
			if _, replayed, err := beginIdempotencyTx(ctx, tx, actor, "merchant.orders.refund-mark", idempotencyKey, digest, s.clock.Now()); err != nil || replayed {
				return err
			}
		}
		var orderID int64
		if err := tx.QueryRow(ctx, `SELECT id FROM orders WHERE merchant_id=$1 AND trade_no=$2 FOR UPDATE`, actor.MerchantID, trade).Scan(&orderID); err != nil {
			return apiErr("NOT_FOUND", 404)
		}
		if err := s.markRefundedTx(ctx, tx, orderID, actor, reason); err != nil {
			return err
		}
		if idempotencyKey != "" {
			return recordIdempotencyTx(ctx, tx, actor, "merchant.orders.refund-mark", idempotencyKey, digest, []byte(`{"completed":true}`), s.clock.Now())
		}
		return nil
	})
}

func (s *Service) markRefundedTx(ctx context.Context, tx pgx.Tx, orderID int64, actor platform.Actor, reason string) error {
	var status, purpose, trade string
	var mid int64
	if err := tx.QueryRow(ctx, `SELECT status,purpose,trade_no,merchant_id FROM orders WHERE id=$1 FOR UPDATE`, orderID).Scan(&status, &purpose, &trade, &mid); err != nil {
		return err
	}
	if status == "refunded" {
		return apiErr("ALREADY_REFUNDED", 409)
	}
	if status != "paid" {
		return apiErr("ORDER_NOT_PAID", 409)
	}
	at := s.clock.Now()
	if _, err := tx.Exec(ctx, `UPDATE orders SET status='refunded',refunded_at=$1,updated_at=$1 WHERE id=$2`, at, orderID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE callback_events SET state='cancelled',next_attempt_at=NULL WHERE order_id=$1 AND state IN ('pending','failed')`, orderID); err != nil {
		return err
	}
	if purpose == "subscription" {
		if s.fulfillment == nil {
			return errors.New("subscription refund port is not connected")
		}
		if err := s.fulfillment.RefundedTx(ctx, tx, orderID, at); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE refunds SET status='completed',reason=COALESCE(NULLIF($1,''),reason),handled_at=$2,handled_by=$3 WHERE order_id=$4 AND status='pending'`, reason, at, actor.SubjectID, orderID); err != nil {
		return err
	}
	return auditOrderTx(ctx, tx, actor, mid, "order.refunded", trade, map[string]any{"reason": reason})
}

func (s *Service) createRefundNotificationTx(ctx context.Context, tx pgx.Tx, merchantID int64, refundID, trade string) error {
	public, err := randomOpaque(s.random, 18)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"title": map[string]string{"zh_CN": "退款申请", "en": "Refund request"}, "summary": map[string]string{"zh_CN": "订单 " + trade + " 有新的退款申请", "en": "A refund was requested for order " + trade}, "action": map[string]string{"page": "/dashboard/orders", "entity_id": trade}})
	dedupe := "refund:" + refundID
	_, err = tx.Exec(ctx, `INSERT INTO notifications(public_id,merchant_id,kind,source_id,dedupe_key,safe_payload) VALUES($1,$2,'refund_request',$3,$4,$5) ON CONFLICT(dedupe_key) DO NOTHING`, public, merchantID, refundID, dedupe, payload)
	return err
}

func (s *Service) createRefundMailTx(ctx context.Context, tx pgx.Tx, merchantID int64, refundID, trade string) error {
	var recipient, locale string
	if err := tx.QueryRow(ctx, `SELECT email_normalized,COALESCE(preferences->>'locale','zh-CN') FROM merchants WHERE id=$1`, merchantID).Scan(&recipient, &locale); err != nil {
		return err
	}
	payload, err := json.Marshal(mailtmpl.Payload{Locale: locale, TradeNo: trade})
	if err != nil {
		return err
	}
	recipientCipher, err := platform.Encrypt(s.key[:], []byte(recipient), s.random)
	if err != nil {
		return err
	}
	payloadCipher, err := platform.Encrypt(s.key[:], payload, s.random)
	if err != nil {
		return err
	}
	public, err := randomOpaque(s.random, 18)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO mail_deliveries(public_id,dedupe_key,template,recipient_cipher,payload_cipher,state) VALUES($1,$2,$3,$4,$5,'queued') ON CONFLICT(dedupe_key) DO NOTHING`, public, "refund:"+refundID, mailtmpl.RefundRequest, recipientCipher, payloadCipher)
	return err
}

func (s *Service) scheduleCallbackTx(ctx context.Context, tx pgx.Tx, orderID int64, trade string, at time.Time) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM callback_events WHERE order_id=$1 AND generation=1)`, orderID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	public, err := randomOpaque(s.random, 18)
	if err != nil {
		return err
	}
	var payload []byte
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('trade_no',trade_no,'created_at',created_at,'paid_at',$2::timestamptz) FROM orders WHERE id=$1`, orderID, at).Scan(&payload); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO callback_events(public_id,order_id,generation,state,next_attempt_at,payload_snapshot) VALUES($1,$2,1,'pending',$3,$4)`, public, orderID, at, payload)
	return err
}

func (s *Service) ApplyPaidReceiptTx(ctx context.Context, tx pgx.Tx, orderID int64, receiptID int64, channel, providerTradeNo, buyerHint string) error {
	return s.MarkPaidTx(ctx, tx, orderID, PaidCommand{Channel: channel, ReceiptID: &receiptID, ProviderTradeNo: providerTradeNo, BuyerHint: buyerHint})
}

func (s *Service) KeyForOrder(ctx context.Context, trade string) ([]byte, error) {
	return s.repository.KeyCipherForOrder(ctx, trade)
}

func (s *Service) OrderID(ctx context.Context, trade string) (int64, error) {
	return s.repository.OrderID(ctx, trade)
}

func (s *Service) orderCreateTx(ctx context.Context, tx pgx.Tx, merchantID int64, trade string, in CreateCommand, digest []byte, snapshot EligibilitySnapshot, createdAt, expires time.Time, channel string, actual *int64, offset *int, asset *string) (int64, time.Time, error) {
	keyVersion := snapshot.KeyVersion
	id := int64(0)
	created := time.Time{}
	name := truncateUTF8(in.Name, 127)
	if name == "" {
		return 0, created, apiErr("VALIDATION_ERROR", 400)
	}
	var notify, returnURL, clientIP any
	if in.NotifyURL != "" {
		notify = in.NotifyURL
	}
	if in.ReturnURL != "" {
		returnURL = in.ReturnURL
	}
	if in.ClientIP != "" {
		clientIP = in.ClientIP
	}
	var channelValue, assetValue any
	if channel != "" {
		channelValue = channel
	}
	if asset != nil && *asset != "" {
		assetValue = *asset
	}
	var actualValue any
	if actual != nil {
		actualValue = *actual
	}
	var offsetValue any
	if offset != nil {
		offsetValue = *offset
	}
	var keyCipher []byte
	err := tx.QueryRow(ctx, `SELECT key_cipher FROM merchants WHERE id=$1`, merchantID).Scan(&keyCipher)
	if err != nil {
		return 0, created, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,product_id,param,original_cents,actual_cents,amount_offset_cents,channel,allowed_channels_snapshot,qr_asset_id,notify_url,return_url,signing_key_version,signing_key_cipher,merchant_username_snapshot,merchant_pid_snapshot,buyer_hint,clientip,channel_locked_at,expires_at,status,created_at,create_request_digest) SELECT $1,m.id,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,m.username,m.pid,$18,$19,CASE WHEN $11::text IS NULL THEN NULL ELSE $22::timestamptz END,$20,'pending',$22::timestamptz,$23 FROM merchants m WHERE m.id=$21 RETURNING id,created_at`, trade, in.OutTradeNo, in.Purpose, in.Protocol, name, in.ProductID, in.Param, in.OriginalCents, actualValue, offsetValue, channelValue, snapshot.Channels, assetValue, notify, returnURL, keyVersion, keyCipher, in.BuyerHint, clientIP, expires, merchantID, createdAt, digest).Scan(&id, &created)
	return id, created, err
}

func (s *Service) reserveAmountTx(ctx context.Context, tx pgx.Tx, merchantID int64, channel string, original int64, snapshot EligibilitySnapshot) (int64, int, error) {
	limit := snapshot.FloatDefaultCents
	if snapshot.MerchantFloatLimit != nil {
		limit = *snapshot.MerchantFloatLimit
	}
	direction := snapshot.FloatDirection
	if snapshot.MerchantFloatDirection != nil {
		direction = *snapshot.MerchantFloatDirection
	}
	if _, err := tx.Exec(ctx, `DELETE FROM amount_reservations WHERE merchant_id=$1 AND release_after IS NOT NULL AND release_after<=$2`, merchantID, s.clock.Now()); err != nil {
		return 0, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT actual_cents FROM amount_reservations WHERE merchant_id=$1 AND channel=$2 FOR UPDATE`, merchantID, channel)
	if err != nil {
		return 0, 0, err
	}
	occupied := map[int64]struct{}{}
	for rows.Next() {
		var value int64
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return 0, 0, err
		}
		occupied[value] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, err
	}
	rows.Close()
	actual, err := AllocateAmount(original, direction, limit, int(snapshot.MinAmountCents), int(snapshot.MaxAmountCents), occupied)
	if err != nil {
		return 0, 0, apiErr("AMOUNT_CAPACITY_EXHAUSTED", 409)
	}
	return actual, int(actual - original), nil
}

func validateExternalCommand(in CreateCommand) error {
	if !validOutTradeNo(in.OutTradeNo) || len([]byte(strings.TrimSpace(in.Name))) == 0 || len([]byte(in.Name)) > 4096 || len([]byte(in.Param)) > 1024 || len([]byte(in.ProductID)) > 64 || in.OriginalCents < 1 || in.OriginalCents > MaxAmountCents {
		return apiErr("VALIDATION_ERROR", 400)
	}
	if in.NotifyURL == "" || !validCallbackURL(in.NotifyURL) {
		return apiErr("INVALID_URL", 400)
	}
	if in.ReturnURL != "" && !validReturnURL(in.ReturnURL) {
		return apiErr("INVALID_URL", 400)
	}
	if in.ClientIP != "" {
		if address, err := netip.ParseAddr(in.ClientIP); err != nil || address.Zone() != "" {
			return apiErr("VALIDATION_ERROR", 400)
		}
	}
	if in.Channel != "" && !platform.ValidPaymentChannel(in.Channel) {
		return apiErr("VALIDATION_ERROR", 400)
	}
	if len(in.Channels) > 0 {
		return apiErr("VALIDATION_ERROR", 400)
	}
	return nil
}
func validOutTradeNo(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && !strings.ContainsRune("_-.|", r) {
			return false
		}
	}
	return true
}
func validCallbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	port := u.Port()
	if port != "" && port != "80" && port != "443" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
		return false
	}
	return signableMerchantQuery(u.RawQuery)
}
func validReturnURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	return signableMerchantQuery(u.RawQuery)
}

func intersectChannels(subscription, requested []string, assets map[string]string) []string {
	out := []string{}
	for _, ch := range platform.PaymentChannels() {
		if containsChannel(subscription, ch) && assets[ch] != "" && (len(requested) == 0 || containsChannel(requested, ch)) {
			out = append(out, ch)
		}
	}
	return out
}
func randomTradeNo(reader io.Reader) (string, error) {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	if reader == nil {
		reader = rand.Reader
	}
	buf := make([]byte, 32)
	raw := make([]byte, 32)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return "", err
	}
	for i, b := range raw {
		buf[i] = chars[int(b)%len(chars)]
	}
	return string(buf), nil
}
func randomOpaque(reader io.Reader, n int) (string, error) { return platform.RandomToken(reader, n) }
func randomID(reader io.Reader) string {
	id, err := randomOpaque(reader, 18)
	if err != nil {
		return ""
	}
	return id
}
func truncateUTF8(value string, max int) string {
	if len(value) <= max {
		return value
	}
	for max > 0 && strings.ToValidUTF8(value[:max], "") != value[:max] {
		max--
	}
	return value[:max]
}
func signCheckoutMedia(key []byte, trade, asset string, expires time.Time) string {
	message := trade + "\n" + asset + "\n" + strconv.FormatInt(expires.Unix(), 10)
	mac := hmacSHA256(key, []byte(message))
	return hexLower(mac)
}
func hmacSHA256(key, value []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(value)
	return h.Sum(nil)
}
func hexLower(value []byte) string { return fmt.Sprintf("%x", value) }
func auditOrderTx(ctx context.Context, tx pgx.Tx, actor platform.Actor, merchantID int64, action, trade string, metadata map[string]any) error {
	raw, _ := json.Marshal(metadata)
	return platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: &merchantID, Action: action, TargetType: "order", TargetID: trade, Result: "success", SafeMetadata: raw})
}
func paidActor(ctx context.Context) platform.Actor {
	if actor, ok := platform.ActorFromContext(ctx); ok {
		return actor
	}
	return platform.Actor{Role: "system"}
}
func protocolSourceIP(ctx context.Context, sourceIP string) string {
	if metadata, ok := platform.RequestMetadataFromContext(ctx); ok {
		return metadata.IP
	}
	address, err := netip.ParseAddr(strings.TrimSpace(sourceIP))
	if err != nil || address.Zone() != "" {
		return ""
	}
	return address.Unmap().String()
}

func (s *Service) CheckProtocolIP(ctx context.Context, merchantID int64, sourceIP string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.eligibility.IPAllowed(ctx, tx, merchantID, protocolSourceIP(ctx, sourceIP)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func apiErr(code string, status int) error { return &platform.Error{Code: code, HTTPStatus: status} }
