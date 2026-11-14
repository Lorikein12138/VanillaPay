package subscription

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
)

type BillingOrderPort interface {
	CreateSubscriptionOrderTx(context.Context, pgx.Tx, payment.BillingOrderCommand) (payment.BillingOrderResult, error)
	CancelSubscriptionOrderTx(context.Context, pgx.Tx, int64) error
}

type Service struct {
	pool       *pgxpool.Pool
	repository Repository
	key        [32]byte
	clock      platform.Clock
	random     io.Reader
	billing    BillingOrderPort
}

func NewService(pool *pgxpool.Pool, key [32]byte, clock platform.Clock, random io.Reader, billing BillingOrderPort) *Service {
	if clock == nil {
		clock = platform.SystemClock{}
	}
	if random == nil {
		random = rand.Reader
	}
	return &Service{pool: pool, repository: NewRepository(pool), key: key, clock: clock, random: random, billing: billing}
}

func (s *Service) SavePlan(ctx context.Context, actor platform.Actor, in PlanCommand) (PlanDTO, error) {
	if actor.Role != "admin" {
		return PlanDTO{}, subError("FORBIDDEN", 403)
	}
	if in.PurchaseLimit != nil && (*in.PurchaseLimit < 1 || *in.PurchaseLimit > math.MaxInt32) {
		return PlanDTO{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400, Fields: map[string]string{"purchase_limit": "invalid"}}
	}
	if err := validatePlan(in); err != nil {
		return PlanDTO{}, subError("VALIDATION_ERROR", 400)
	}
	for field, amount := range map[string]*int64{"min_order_cents": in.MinOrderCents, "max_order_cents": in.MaxOrderCents, "daily_limit_cents": in.DailyLimitCents} {
		if amount != nil && (*amount < 1 || *amount > platform.HardAmountLimit) {
			return PlanDTO{}, platform.InvalidJSONField(field)
		}
	}
	if in.MinOrderCents != nil && in.MaxOrderCents != nil && *in.MinOrderCents > *in.MaxOrderCents {
		return PlanDTO{}, platform.InvalidJSONField("max_order_cents")
	}
	channels := canonicalChannels(in.Channels)
	var id int64
	var revision int64
	var deletedAt *time.Time
	var created PlanDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if in.ID == "" {
			if err := tx.QueryRow(ctx, `INSERT INTO plans(name_zh,name_en,price_cents,duration_days,channels,purchase_limit,min_order_cents,max_order_cents,daily_limit_cents) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,revision,deleted_at`, strings.TrimSpace(in.Name.ZhCN), strings.TrimSpace(in.Name.En), in.PriceCents, in.DurationDays, channels, in.PurchaseLimit, in.MinOrderCents, in.MaxOrderCents, in.DailyLimitCents).Scan(&id, &revision, &deletedAt); err != nil {
				return err
			}
		} else {
			parsed, err := strconv.ParseInt(in.ID, 10, 64)
			if err != nil || parsed <= 0 {
				return subError("VALIDATION_ERROR", 400)
			}
			id = parsed
			if in.Revision <= 0 {
				return subError("VALIDATION_ERROR", 400)
			}
			err = tx.QueryRow(ctx, `UPDATE plans SET revision=revision+1,name_zh=$1,name_en=$2,price_cents=$3,duration_days=$4,channels=$5,purchase_limit=$6,min_order_cents=$7,max_order_cents=$8,daily_limit_cents=$9,updated_at=$10 WHERE id=$11 AND revision=$12 AND deleted_at IS NULL RETURNING revision,deleted_at`, strings.TrimSpace(in.Name.ZhCN), strings.TrimSpace(in.Name.En), in.PriceCents, in.DurationDays, channels, in.PurchaseLimit, in.MinOrderCents, in.MaxOrderCents, in.DailyLimitCents, s.clock.Now(), id, in.Revision).Scan(&revision, &deletedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return subError("VERSION_CONFLICT", 409)
			}
			if err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `SELECT name_zh,name_en,price_cents,duration_days,channels,purchase_limit FROM plans WHERE id=$1`, id).Scan(&created.Name.ZhCN, &created.Name.En, &created.PriceCents, &created.DurationDays, &created.Channels, &created.PurchaseLimit); err != nil {
			return err
		}
		created.ID = strconv.FormatInt(id, 10)
		created.PlanLimits = in.PlanLimits
		created.Revision = revision
		created.Deleted = deletedAt != nil
		return auditTx(ctx, tx, actor, "plan.save", created.ID, map[string]any{"revision": revision})
	})
	return created, err
}

func (s *Service) DeletePlan(ctx context.Context, actor platform.Actor, id string, revision int64) (DeletedPlanResult, error) {
	if actor.Role != "admin" {
		return DeletedPlanResult{}, subError("FORBIDDEN", 403)
	}
	planID, err := strconv.ParseInt(id, 10, 64)
	if err != nil || planID <= 0 || revision <= 0 {
		return DeletedPlanResult{}, subError("VALIDATION_ERROR", 400)
	}
	var result DeletedPlanResult
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var current int64
		if err := tx.QueryRow(ctx, `SELECT revision FROM plans WHERE id=$1 FOR UPDATE`, planID).Scan(&current); err != nil {
			return subError("NOT_FOUND", 404)
		}
		if current != revision {
			return subError("VERSION_CONFLICT", 409)
		}
		tag, err := tx.Exec(ctx, `UPDATE plans SET deleted_at=$1,revision=revision+1,updated_at=$1 WHERE id=$2 AND deleted_at IS NULL`, s.clock.Now(), planID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return subError("VERSION_CONFLICT", 409)
		}
		rows, err := tx.Query(ctx, `SELECT id,buyer_id,payment_trade_no FROM purchases WHERE plan_id=$1 AND state='pending' ORDER BY buyer_id,id FOR UPDATE`, planID)
		if err != nil {
			return err
		}
		type pending struct {
			id, buyer int64
			trade     *string
		}
		var list []pending
		for rows.Next() {
			var item pending
			if err := rows.Scan(&item.id, &item.buyer, &item.trade); err != nil {
				rows.Close()
				return err
			}
			list = append(list, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, item := range list {
			if item.trade != nil && s.billing != nil {
				var orderID int64
				if err := tx.QueryRow(ctx, `SELECT id FROM orders WHERE trade_no=$1 FOR UPDATE`, *item.trade).Scan(&orderID); err == nil {
					if err := s.billing.CancelSubscriptionOrderTx(ctx, tx, orderID); err != nil {
						return err
					}
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE purchases SET state='cancelled',updated_at=$1 WHERE id=$2 AND state='pending'`, s.clock.Now(), item.id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE plan_purchase_counters SET reserved_count=GREATEST(0,reserved_count-1) WHERE merchant_id=$1 AND plan_id=$2`, item.buyer, planID); err != nil {
				return err
			}
			result.CancelledPendingPurchases++
		}
		result.Deleted = true
		return auditTx(ctx, tx, actor, "plan.delete", id, map[string]any{"cancelled_pending_purchases": result.CancelledPendingPurchases})
	})
	return result, err
}

func (s *Service) ListPlans(ctx context.Context, includeDeleted bool, actor platform.Actor) ([]PlanDTO, error) {
	if actor.Role != "admin" && actor.Role != "merchant" && actor.Role != "public" {
		return nil, subError("UNAUTHENTICATED", 401)
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id,p.revision,p.name_zh,p.name_en,p.price_cents,p.duration_days,p.channels,p.purchase_limit,p.deleted_at,p.min_order_cents,p.max_order_cents,p.daily_limit_cents FROM plans p WHERE ($1::boolean OR p.deleted_at IS NULL) ORDER BY p.id`, includeDeleted && actor.Role == "admin")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlanDTO{}
	for rows.Next() {
		var id int64
		var p PlanDTO
		var deleted *time.Time
		if err := rows.Scan(&id, &p.Revision, &p.Name.ZhCN, &p.Name.En, &p.PriceCents, &p.DurationDays, &p.Channels, &p.PurchaseLimit, &deleted, &p.MinOrderCents, &p.MaxOrderCents, &p.DailyLimitCents); err != nil {
			return nil, err
		}
		p.ID = strconv.FormatInt(id, 10)
		p.Deleted = deleted != nil
		if actor.Role == "merchant" {
			var paid, reserved int
			if err := s.pool.QueryRow(ctx, `SELECT COALESCE(paid_count,0),COALESCE(reserved_count,0) FROM plan_purchase_counters WHERE merchant_id=$1 AND plan_id=$2`, actor.MerchantID, id).Scan(&paid, &reserved); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if p.PurchaseLimit != nil {
				p.PurchasedCount = &paid
				remaining := *p.PurchaseLimit - paid - reserved
				if remaining < 0 {
					remaining = 0
				}
				p.RemainingCount = &remaining
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) GetCurrent(ctx context.Context, actor platform.Actor) (SubscriptionDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return SubscriptionDTO{}, subError("UNAUTHENTICATED", 401)
	}
	var id int64
	var planID pgtype.Int8
	var snapshot []byte
	var expiry pgtype.Timestamptz
	var revision int64
	var source string
	err := s.pool.QueryRow(ctx, `SELECT plan_id,snapshot,expires_at,revision,source FROM subscriptions WHERE merchant_id=$1`, actor.MerchantID).Scan(&planID, &snapshot, &expiry, &revision, &source)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionDTO{Channels: []string{}, Active: false, Source: "none", Revision: 0}, nil
	}
	if err != nil {
		return SubscriptionDTO{}, err
	}
	out := SubscriptionDTO{Channels: []string{}, Active: expiry.Valid && expiry.Time.After(s.clock.Now()), Source: source, Revision: revision}
	if expiry.Valid {
		v := expiry.Time
		out.ExpiresAt = &v
	}
	if planID.Valid {
		var p PlanDTO
		var deleted *time.Time
		if err := s.pool.QueryRow(ctx, `SELECT id,revision,name_zh,name_en,price_cents,duration_days,channels,purchase_limit,deleted_at FROM plans WHERE id=$1`, planID.Int64).Scan(&id, &p.Revision, &p.Name.ZhCN, &p.Name.En, &p.PriceCents, &p.DurationDays, &p.Channels, &p.PurchaseLimit, &deleted); err != nil {
			return SubscriptionDTO{}, err
		}
		// Entitlements come from the grant-time snapshot; only the deleted flag tracks the live plan.
		if len(snapshot) > 0 {
			var granted PlanDTO
			if err := json.Unmarshal(snapshot, &granted); err != nil {
				return SubscriptionDTO{}, err
			}
			p = granted
		}
		p.ID = strconv.FormatInt(id, 10)
		p.Deleted = deleted != nil
		out.Plan = &p
		out.Channels = append([]string{}, p.Channels...)
	}
	return out, nil
}

func (s *Service) Purchase(ctx context.Context, actor platform.Actor, in PurchaseRequest, idempotencyKey string) (PurchaseDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return PurchaseDTO{}, subError("UNAUTHENTICATED", 401)
	}
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return PurchaseDTO{}, subError("VALIDATION_ERROR", 400)
	}
	planID, err := strconv.ParseInt(in.PlanID, 10, 64)
	if err != nil || planID <= 0 || in.PlanRevision <= 0 || in.ExpectedSubscriptionRevision < 0 {
		return PurchaseDTO{}, subError("VALIDATION_ERROR", 400)
	}
	if s.billing == nil {
		return PurchaseDTO{}, subError("BILLING_NOT_READY", 422)
	}
	var configuredPID string
	if err := s.pool.QueryRow(ctx, `SELECT billing->>'merchant_pid' FROM site_settings WHERE singleton=true`).Scan(&configuredPID); err != nil {
		return PurchaseDTO{}, err
	}
	var billingID int64
	if configuredPID == "" || s.pool.QueryRow(ctx, `SELECT id FROM merchants WHERE pid=$1 AND banned_at IS NULL`, configuredPID).Scan(&billingID) != nil {
		return PurchaseDTO{}, subError("BILLING_NOT_READY", 422)
	}
	digestData, _ := json.Marshal(in)
	digest := sha256.Sum256(digestData)
	var result PurchaseDTO
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID, billingID}, false); err != nil {
			return err
		}
		var oldID int64
		var oldDigest []byte
		var old PurchaseDTO
		var oldSnapshot []byte
		var oldTrade *string
		var oldPaid, oldFulfilled pgtype.Timestamptz
		err := tx.QueryRow(ctx, `SELECT p.id,p.public_id,p.plan_snapshot,p.purchase_kind,p.state,p.payment_trade_no,p.created_at,p.paid_at,p.expires_at,p.fulfillment_at,p.request_digest FROM purchases p WHERE p.buyer_id=$1 AND p.idempotency_key=$2`, actor.MerchantID, idempotencyKey).Scan(&oldID, &old.ID, &oldSnapshot, &old.PurchaseKind, &old.State, &oldTrade, &old.CreatedAt, &oldPaid, &old.ExpiresAt, &oldFulfilled, &oldDigest)
		if err == nil {
			if subtle.ConstantTimeCompare(oldDigest, digest[:]) != 1 {
				return subError("IDEMPOTENCY_CONFLICT", 409)
			}
			if err := json.Unmarshal(oldSnapshot, &old.PlanSnapshot); err != nil {
				return err
			}
			old.PriceCents = old.PlanSnapshot.PriceCents
			old.TradeNo = oldTrade
			if oldTrade != nil {
				cashier := s.cashierURL(*oldTrade)
				old.CashierURL = &cashier
			}
			if oldPaid.Valid {
				v := oldPaid.Time
				old.PaidAt = &v
			}
			if oldFulfilled.Valid {
				v := oldFulfilled.Time
				old.FulfillmentAt = &v
			}
			result = old
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var billingPID string
		var billingCipher []byte
		var valid bool
		if err := tx.QueryRow(ctx, `SELECT s.billing->>'merchant_pid',ss.encrypted_value FROM site_settings s LEFT JOIN secret_settings ss ON ss.name='billing_key' WHERE s.singleton=true`).Scan(&billingPID, &billingCipher); err != nil {
			return err
		}
		if billingPID != configuredPID || len(billingCipher) == 0 {
			return subError("BILLING_CREDENTIAL_STALE", 409)
		}
		key, err := platform.Decrypt(s.key[:], billingCipher)
		if err != nil {
			return subError("BILLING_CREDENTIAL_STALE", 409)
		}
		var merchantCipher []byte
		if err := tx.QueryRow(ctx, `SELECT key_cipher,banned_at IS NULL FROM merchants WHERE id=$1 AND pid=$2 FOR UPDATE`, billingID, billingPID).Scan(&merchantCipher, &valid); err != nil || !valid {
			return subError("BILLING_CREDENTIAL_STALE", 409)
		}
		merchantKey, err := platform.Decrypt(s.key[:], merchantCipher)
		if err != nil || len(key) != 64 || subtle.ConstantTimeCompare(key, merchantKey) != 1 {
			return subError("BILLING_CREDENTIAL_STALE", 409)
		}
		var plan PlanDTO
		var channels []string
		var deleted *time.Time
		if err := tx.QueryRow(ctx, `SELECT id,revision,name_zh,name_en,price_cents,duration_days,channels,purchase_limit,deleted_at,min_order_cents,max_order_cents,daily_limit_cents FROM plans WHERE id=$1 FOR UPDATE`, planID).Scan(&oldID, &plan.Revision, &plan.Name.ZhCN, &plan.Name.En, &plan.PriceCents, &plan.DurationDays, &channels, &plan.PurchaseLimit, &deleted, &plan.MinOrderCents, &plan.MaxOrderCents, &plan.DailyLimitCents); err != nil {
			return subError("PLAN_UNAVAILABLE", 409)
		}
		if deleted != nil || plan.Revision != in.PlanRevision {
			return subError("PLAN_UNAVAILABLE", 409)
		}
		plan.ID = strconv.FormatInt(planID, 10)
		plan.Channels = channels
		var subRevision int64
		var currentPlan pgtype.Int8
		var currentExpiry pgtype.Timestamptz
		var currentChain pgtype.Text
		err = tx.QueryRow(ctx, `SELECT revision,plan_id,expires_at,active_chain_id FROM subscriptions WHERE merchant_id=$1 FOR UPDATE`, actor.MerchantID).Scan(&subRevision, &currentPlan, &currentExpiry, &currentChain)
		if errors.Is(err, pgx.ErrNoRows) {
			subRevision = 0
		} else if err != nil {
			return err
		}
		if subRevision != in.ExpectedSubscriptionRevision {
			return subError("VERSION_CONFLICT", 409)
		}
		active := currentExpiry.Valid && currentExpiry.Time.After(s.clock.Now()) && currentPlan.Valid
		kind := "new"
		if active && currentPlan.Int64 == planID {
			kind = "renew"
		} else if active && currentPlan.Int64 != planID {
			if !in.ConfirmReplace {
				return subError("VERSION_CONFLICT", 409)
			}
			kind = "switch"
		}
		var pending bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM purchases WHERE buyer_id=$1 AND state='pending' AND expires_at>$2)`, actor.MerchantID, s.clock.Now()).Scan(&pending); err != nil {
			return err
		}
		if pending {
			return subError("PURCHASE_PENDING", 409)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO plan_purchase_counters(merchant_id,plan_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, actor.MerchantID, planID); err != nil {
			return err
		}
		var paid, reserved int
		var limit *int
		if err := tx.QueryRow(ctx, `SELECT c.paid_count,c.reserved_count,p.purchase_limit FROM plan_purchase_counters c JOIN plans p ON p.id=c.plan_id WHERE c.merchant_id=$1 AND c.plan_id=$2 FOR UPDATE`, actor.MerchantID, planID).Scan(&paid, &reserved, &limit); err != nil {
			return err
		}
		if limit != nil && paid+reserved >= *limit {
			return subError("PURCHASE_LIMIT_REACHED", 409)
		}
		var global struct {
			DefaultTTLSeconds int `json:"default_ttl_seconds"`
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT payment FROM site_settings WHERE singleton=true`).Scan(&raw); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &global); err != nil {
			return err
		}
		expires := s.clock.Now().Add(time.Duration(global.DefaultTTLSeconds) * time.Second)
		snapshot, _ := json.Marshal(plan)
		publicID, err := platform.RandomToken(s.random, 18)
		if err != nil {
			return err
		}
		var purchaseID int64
		if err := tx.QueryRow(ctx, `INSERT INTO purchases(public_id,buyer_id,billing_merchant_id,plan_id,plan_snapshot,purchase_kind,expected_subscription_revision,state,idempotency_key,request_digest,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10) RETURNING id,created_at`, publicID, actor.MerchantID, billingID, planID, snapshot, kind, subRevision, idempotencyKey, digest[:], expires).Scan(&purchaseID, &result.CreatedAt); err != nil {
			return err
		}
		order, err := s.billing.CreateSubscriptionOrderTx(ctx, tx, payment.BillingOrderCommand{PurchaseID: purchaseID, PublicPurchaseID: publicID, BuyerID: actor.MerchantID, BillingMerchantID: billingID, Name: plan.Name.ZhCN, AmountCents: plan.PriceCents, Channels: plan.Channels, ExpiresAt: expires})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE purchases SET payment_trade_no=$1 WHERE id=$2`, order.TradeNo, purchaseID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE plan_purchase_counters SET reserved_count=reserved_count+1 WHERE merchant_id=$1 AND plan_id=$2`, actor.MerchantID, planID); err != nil {
			return err
		}
		result.ID = publicID
		result.PlanSnapshot = plan
		result.PurchaseKind = kind
		result.State = "pending"
		result.PriceCents = plan.PriceCents
		result.TradeNo = &order.TradeNo
		cashier := order.CashierURL
		if cashier == "" {
			cashier = s.cashierURL(order.TradeNo)
		}
		result.CashierURL = &cashier
		result.ExpiresAt = expires
		return auditTx(ctx, tx, actor, "subscription.purchase", publicID, map[string]any{"plan_id": planID, "purchase_kind": kind})
	})
	return result, err
}

func (s *Service) CancelPurchase(ctx context.Context, actor platform.Actor, id string) (PurchaseDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return PurchaseDTO{}, subError("UNAUTHENTICATED", 401)
	}
	var out PurchaseDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		var pid, planID int64
		var state string
		var trade *string
		var snap []byte
		var expires, created time.Time
		err := tx.QueryRow(ctx, `SELECT id,plan_id,state,payment_trade_no,plan_snapshot,expires_at,created_at FROM purchases WHERE public_id=$1 AND buyer_id=$2 FOR UPDATE`, id, actor.MerchantID).Scan(&pid, &planID, &state, &trade, &snap, &expires, &created)
		if errors.Is(err, pgx.ErrNoRows) {
			return subError("NOT_FOUND", 404)
		}
		if err != nil {
			return err
		}
		if state != "pending" {
			return subError("VERSION_CONFLICT", 409)
		}
		if trade != nil && s.billing != nil {
			var oid int64
			if err := tx.QueryRow(ctx, `SELECT id FROM orders WHERE trade_no=$1 FOR UPDATE`, *trade).Scan(&oid); err != nil {
				return err
			}
			if err := s.billing.CancelSubscriptionOrderTx(ctx, tx, oid); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE purchases SET state='cancelled',updated_at=$1 WHERE id=$2`, s.clock.Now(), pid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE plan_purchase_counters SET reserved_count=GREATEST(0,reserved_count-1) WHERE merchant_id=$1 AND plan_id=$2`, actor.MerchantID, planID); err != nil {
			return err
		}
		out.ID = id
		out.State = "cancelled"
		out.TradeNo = trade
		out.CreatedAt = created
		out.ExpiresAt = expires
		_ = json.Unmarshal(snap, &out.PlanSnapshot)
		out.PriceCents = out.PlanSnapshot.PriceCents
		return auditTxMerchant(ctx, tx, actor, &actor.MerchantID, "subscription.cancel", id, map[string]any{})
	})
	return out, err
}

func (s *Service) PaidTx(ctx context.Context, tx pgx.Tx, orderID int64, at time.Time) error {
	var purchaseID int64
	var buyer, planID int64
	var kind, state string
	var snapshot []byte
	var publicID string
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT p.id,p.public_id,p.buyer_id,p.plan_id,p.purchase_kind,p.state,p.plan_snapshot,p.expires_at FROM purchases p JOIN orders o ON o.purchase_id=p.id WHERE o.id=$1 FOR UPDATE OF p`, orderID).Scan(&purchaseID, &publicID, &buyer, &planID, &kind, &state, &snapshot, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if state == "paid" {
		return nil
	}
	if state != "pending" || !expires.After(at) {
		return subError("ORDER_NOT_PAYABLE", 409)
	}
	var plan PlanDTO
	if err := json.Unmarshal(snapshot, &plan); err != nil {
		return err
	}
	var currentPlan pgtype.Int8
	var oldChain pgtype.Text
	var oldExpires pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT plan_id,active_chain_id,expires_at FROM subscriptions WHERE merchant_id=$1 FOR UPDATE`, buyer).Scan(&currentPlan, &oldChain, &oldExpires)
	if errors.Is(err, pgx.ErrNoRows) {
		oldChain.Valid = false
	} else if err != nil {
		return err
	}
	chain := ""
	effective := at
	if kind == "renew" && oldChain.Valid && oldExpires.Valid && oldExpires.Time.After(at) {
		chain = oldChain.String
		effective = oldExpires.Time
	} else {
		var e error
		chain, e = platform.RandomToken(s.random, 18)
		if e != nil {
			return e
		}
		if kind == "switch" && oldChain.Valid {
			if _, err := tx.Exec(ctx, `UPDATE subscription_grants SET voided_at=$1 WHERE merchant_id=$2 AND chain_id=$3 AND voided_at IS NULL`, at, buyer, oldChain.String); err != nil {
				return err
			}
		}
	}
	grantKind := kind
	if grantKind == "new" && oldExpires.Valid && !oldExpires.Time.After(at) {
		grantKind = "new"
	}
	duration := time.Duration(plan.DurationDays) * 24 * time.Hour
	if _, err := tx.Exec(ctx, `INSERT INTO subscription_grants(purchase_id,merchant_id,chain_id,kind,duration,effective_at,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7)`, purchaseID, buyer, chain, grantKind, duration, effective, snapshot); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE purchases SET state='paid',paid_at=$1,fulfillment_at=$1,updated_at=$1 WHERE id=$2`, at, purchaseID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE plan_purchase_counters SET reserved_count=GREATEST(0,reserved_count-1),paid_count=paid_count+1 WHERE merchant_id=$1 AND plan_id=$2`, buyer, planID); err != nil {
		return err
	}
	if err := s.recomputeChainTx(ctx, tx, buyer, chain, at); err != nil {
		return err
	}
	actor, ok := platform.ActorFromContext(ctx)
	if !ok {
		actor = platform.Actor{Role: "system"}
	}
	return auditTxMerchant(ctx, tx, actor, &buyer, "subscription.fulfilled", publicID, map[string]any{"purchase_kind": kind})
}

func (s *Service) RefundedTx(ctx context.Context, tx pgx.Tx, orderID int64, at time.Time) error {
	var purchaseID, buyer, planID int64
	var state string
	var chain string
	var err error
	err = tx.QueryRow(ctx, `SELECT p.id,p.buyer_id,p.plan_id,p.state,g.chain_id FROM purchases p JOIN subscription_grants g ON g.purchase_id=p.id JOIN orders o ON o.purchase_id=p.id WHERE o.id=$1 FOR UPDATE OF p,g`, orderID).Scan(&purchaseID, &buyer, &planID, &state, &chain)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if state == "refunded" {
		return nil
	}
	if state != "paid" {
		return subError("ORDER_NOT_PAID", 409)
	}
	if _, err := tx.Exec(ctx, `UPDATE purchases SET state='refunded',updated_at=$1 WHERE id=$2`, at, purchaseID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE subscription_grants SET refunded_at=$1 WHERE purchase_id=$2 AND refunded_at IS NULL`, at, purchaseID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE plan_purchase_counters SET paid_count=GREATEST(0,paid_count) WHERE merchant_id=$1 AND plan_id=$2`, buyer, planID)
	if err != nil {
		return err
	}
	// Only the active chain drives the subscription; a refund on a voided chain (replaced by a
	// switch or a manual override) must leave the current entitlement untouched (design 3.2).
	var activeChain pgtype.Text
	err = tx.QueryRow(ctx, `SELECT active_chain_id FROM subscriptions WHERE merchant_id=$1 FOR UPDATE`, buyer).Scan(&activeChain)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if activeChain.Valid && activeChain.String == chain {
		if err := s.recomputeChainTx(ctx, tx, buyer, chain, at); err != nil {
			return err
		}
	}
	actor, ok := platform.ActorFromContext(ctx)
	if !ok {
		actor = platform.Actor{Role: "system"}
	}
	return auditTxMerchant(ctx, tx, actor, &buyer, "subscription.refunded", strconv.FormatInt(purchaseID, 10), map[string]any{"plan_id": planID})
}

func (s *Service) Grant(ctx context.Context, actor platform.Actor, merchantID int64, in GrantRequest) (SubscriptionDTO, error) {
	if actor.Role != "admin" || merchantID <= 0 {
		return SubscriptionDTO{}, subError("FORBIDDEN", 403)
	}
	planID, err := strconv.ParseInt(in.PlanID, 10, 64)
	if err != nil || planID <= 0 || in.ExpectedRevision < 0 || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 500 || !validChannels(in.Channels) {
		return SubscriptionDTO{}, subError("VALIDATION_ERROR", 400)
	}
	for attempt := 0; attempt < 3; attempt++ {
		billingIDs, err := pendingBillingMerchantIDs(ctx, s.pool, merchantID)
		if err != nil {
			return SubscriptionDTO{}, err
		}
		lockIDs := append([]int64{merchantID}, billingIDs...)
		lockSet := make(map[int64]struct{}, len(lockIDs))
		for _, id := range lockIDs {
			lockSet[id] = struct{}{}
		}
		var out SubscriptionDTO
		err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			if err := sqlc.LockBusinessRows(ctx, tx, lockIDs, false); err != nil {
				return err
			}
			billingIDs, err := pendingBillingMerchantIDs(ctx, tx, merchantID)
			if err != nil {
				return err
			}
			for _, id := range billingIDs {
				if _, ok := lockSet[id]; !ok {
					return errPendingBillingLockSetChanged
				}
			}
			if err := s.CancelPendingPurchasesForBannedTx(ctx, tx, merchantID); err != nil {
				return err
			}
			var revision int64
			var oldChain pgtype.Text
			if err := tx.QueryRow(ctx, `SELECT revision,active_chain_id FROM subscriptions WHERE merchant_id=$1 FOR UPDATE`, merchantID).Scan(&revision, &oldChain); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if revision != in.ExpectedRevision {
				return subError("VERSION_CONFLICT", 409)
			}
			var plan PlanDTO
			var deleted *time.Time
			var channels []string
			if err := tx.QueryRow(ctx, `SELECT id,revision,name_zh,name_en,price_cents,duration_days,channels,purchase_limit,deleted_at,min_order_cents,max_order_cents,daily_limit_cents FROM plans WHERE id=$1`, planID).Scan(new(int64), &plan.Revision, &plan.Name.ZhCN, &plan.Name.En, &plan.PriceCents, &plan.DurationDays, &channels, &plan.PurchaseLimit, &deleted, &plan.MinOrderCents, &plan.MaxOrderCents, &plan.DailyLimitCents); err != nil || deleted != nil {
				return subError("PLAN_UNAVAILABLE", 409)
			}
			for _, ch := range in.Channels {
				if !contains(channels, ch) {
					return subError("VALIDATION_ERROR", 400)
				}
			}
			chain, e := platform.RandomToken(s.random, 18)
			if e != nil {
				return e
			}
			if oldChain.Valid {
				if _, err := tx.Exec(ctx, `UPDATE subscription_grants SET voided_at=$1 WHERE merchant_id=$2 AND chain_id=$3 AND voided_at IS NULL`, s.clock.Now(), merchantID, oldChain.String); err != nil {
					return err
				}
			}
			plan.ID = strconv.FormatInt(planID, 10)
			plan.Channels = canonicalChannels(in.Channels)
			snapshot, _ := json.Marshal(plan)
			expiry := in.ExpiresAt.UTC()
			if _, err := tx.Exec(ctx, `INSERT INTO subscription_grants(merchant_id,chain_id,kind,duration,effective_at,fixed_expires_at,snapshot) VALUES($1,$2,'manual',NULL,$3,$4,$5)`, merchantID, chain, s.clock.Now(), expiry, snapshot); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO subscriptions(merchant_id,plan_id,snapshot,expires_at,revision,active_chain_id,source,updated_at) VALUES($1,$2,$3,$4,1,$5,'manual',$6) ON CONFLICT(merchant_id) DO UPDATE SET plan_id=EXCLUDED.plan_id,snapshot=EXCLUDED.snapshot,expires_at=EXCLUDED.expires_at,revision=subscriptions.revision+1,active_chain_id=EXCLUDED.active_chain_id,source='manual',updated_at=EXCLUDED.updated_at`, merchantID, planID, snapshot, expiry, chain, s.clock.Now())
			if err != nil {
				return err
			}
			mid := merchantID
			if err := auditTxMerchant(ctx, tx, actor, &mid, "subscription.manual_grant", strconv.FormatInt(merchantID, 10), map[string]any{"reason": in.Reason, "expires_at": expiry}); err != nil {
				return err
			}
			out = SubscriptionDTO{Plan: &plan, Channels: plan.Channels, ExpiresAt: &expiry, Active: expiry.After(s.clock.Now()), Source: "manual", Revision: revision + 1}
			return nil
		})
		if errors.Is(err, errPendingBillingLockSetChanged) {
			continue
		}
		return out, err
	}
	return SubscriptionDTO{}, subError("VERSION_CONFLICT", 409)
}

func (s *Service) recomputeChainTx(ctx context.Context, tx pgx.Tx, buyer int64, chain string, at time.Time) error {
	rows, err := tx.Query(ctx, `SELECT kind,COALESCE((extract(epoch FROM duration)*1000000000)::bigint,0),effective_at,fixed_expires_at,snapshot,refunded_at,voided_at FROM subscription_grants WHERE merchant_id=$1 AND chain_id=$2 ORDER BY effective_at,id FOR UPDATE`, buyer, chain)
	if err != nil {
		return err
	}
	type grant struct {
		kind                    string
		duration                time.Duration
		effective               time.Time
		fixed, refunded, voided pgtype.Timestamptz
		snapshot                []byte
	}
	list := []grant{}
	for rows.Next() {
		var g grant
		var duration int64
		if err := rows.Scan(&g.kind, &duration, &g.effective, &g.fixed, &g.snapshot, &g.refunded, &g.voided); err != nil {
			rows.Close()
			return err
		}
		g.duration = time.Duration(duration)
		list = append(list, g)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	var expiry *time.Time
	var chosen []byte
	source := "none"
	for _, g := range list {
		if g.voided.Valid || g.refunded.Valid {
			continue
		}
		if g.fixed.Valid {
			v := g.fixed.Time
			expiry = &v
			chosen = g.snapshot
			source = "manual"
			continue
		}
		if g.duration <= 0 {
			continue
		}
		start := g.effective
		if expiry != nil && expiry.After(start) {
			start = *expiry
		}
		next := start.Add(g.duration)
		expiry = &next
		chosen = g.snapshot
		if source == "none" {
			source = "purchase"
		}
	}
	if expiry != nil && !expiry.After(at) {
		source = "none"
	}
	if expiry == nil {
		if _, err := tx.Exec(ctx, `UPDATE subscriptions SET plan_id=NULL,snapshot=NULL,expires_at=$1,revision=revision+1,active_chain_id=$2,source='none',updated_at=$3 WHERE merchant_id=$4`, at, chain, at, buyer); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return nil
	}
	var plan PlanDTO
	if err := json.Unmarshal(chosen, &plan); err != nil {
		return err
	}
	planID, _ := strconv.ParseInt(plan.ID, 10, 64)
	_, err = tx.Exec(ctx, `INSERT INTO subscriptions(merchant_id,plan_id,snapshot,expires_at,revision,active_chain_id,source,updated_at) VALUES($1,$2,$3,$4,1,$5,$6,$7) ON CONFLICT(merchant_id) DO UPDATE SET plan_id=EXCLUDED.plan_id,snapshot=EXCLUDED.snapshot,expires_at=EXCLUDED.expires_at,revision=subscriptions.revision+1,active_chain_id=EXCLUDED.active_chain_id,source=EXCLUDED.source,updated_at=EXCLUDED.updated_at`, buyer, planID, chosen, expiry, chain, source, at)
	return err
}

func (s *Service) cashierURL(trade string) string { return "/pay/" + trade }
func (s *Service) CloseExpiredPurchases(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT id,buyer_id,plan_id FROM purchases WHERE state='pending' AND expires_at<=$1 ORDER BY expires_at,id LIMIT $2`, s.clock.Now(), limit)
	if err != nil {
		return 0, err
	}
	type item struct{ id, buyer, plan int64 }
	var items []item
	for rows.Next() {
		var i item
		if err := rows.Scan(&i.id, &i.buyer, &i.plan); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, i)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	done := 0
	for _, i := range items {
		err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			if err := sqlc.LockBusinessRows(ctx, tx, []int64{i.buyer}, false); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `UPDATE purchases SET state='expired',updated_at=$1 WHERE id=$2 AND state='pending' AND expires_at<=$1`, s.clock.Now(), i.id)
			if err != nil {
				return err
			}
			if tag.RowsAffected() > 0 {
				_, err = tx.Exec(ctx, `UPDATE plan_purchase_counters SET reserved_count=GREATEST(0,reserved_count-1) WHERE merchant_id=$1 AND plan_id=$2`, i.buyer, i.plan)
				done++
			}
			return err
		})
		if err != nil {
			return done, err
		}
	}
	return done, nil
}

// CancelPendingPurchasesForBannedTx cancels every pending purchase while the
// caller holds the buyer's business lock, preserving the payment lock order.
func (s *Service) CancelPendingPurchasesForBannedTx(ctx context.Context, tx pgx.Tx, buyerID int64) error {
	rows, err := tx.Query(ctx, `SELECT id,plan_id,payment_trade_no FROM purchases WHERE buyer_id=$1 AND state='pending' ORDER BY id FOR UPDATE`, buyerID)
	if err != nil {
		return err
	}
	type item struct {
		id, plan int64
		trade    *string
	}
	var items []item
	for rows.Next() {
		var v item
		if err := rows.Scan(&v.id, &v.plan, &v.trade); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, v := range items {
		if v.trade != nil && s.billing != nil {
			var orderID int64
			if err := tx.QueryRow(ctx, `SELECT id FROM orders WHERE trade_no=$1 FOR UPDATE`, *v.trade).Scan(&orderID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if orderID > 0 {
				if err := s.billing.CancelSubscriptionOrderTx(ctx, tx, orderID); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE purchases SET state='cancelled',updated_at=$1 WHERE id=$2 AND state='pending'`, s.clock.Now(), v.id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE plan_purchase_counters SET reserved_count=GREATEST(0,reserved_count-1) WHERE merchant_id=$1 AND plan_id=$2`, buyerID, v.plan); err != nil {
			return err
		}
	}
	return nil
}

func validatePlan(in PlanCommand) error {
	if strings.TrimSpace(in.Name.ZhCN) == "" || len(in.Name.ZhCN) > 300 || len(in.Name.En) > 300 || in.PriceCents < 1 || in.PriceCents > 999999999 || in.DurationDays < 1 || in.DurationDays > 36500 || !validChannels(in.Channels) {
		return errors.New("invalid plan")
	}
	return nil
}
func validChannels(channels []string) bool {
	if len(channels) == 0 || len(channels) > 3 {
		return false
	}
	seen := map[string]bool{}
	for _, ch := range channels {
		if !platform.ValidPaymentChannel(ch) || seen[ch] {
			return false
		}
		seen[ch] = true
	}
	return true
}
func canonicalChannels(channels []string) []string {
	out := append([]string(nil), channels...)
	sort.Strings(out)
	return out
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func subError(code string, status int) error { return &platform.Error{Code: code, HTTPStatus: status} }
func auditTx(ctx context.Context, tx pgx.Tx, actor platform.Actor, action, id string, metadata map[string]any) error {
	var mid any
	if actor.MerchantID > 0 {
		mid = actor.MerchantID
	}
	return auditTxMerchant(ctx, tx, actor, toInt64Ptr(mid), action, id, metadata)
}
func toInt64Ptr(value any) *int64 {
	if v, ok := value.(int64); ok {
		return &v
	}
	return nil
}
func auditTxMerchant(ctx context.Context, tx pgx.Tx, actor platform.Actor, merchantID *int64, action, id string, metadata map[string]any) error {
	raw, _ := json.Marshal(metadata)
	return platform.RecordAudit(ctx, tx, platform.AuditEvent{Actor: actor, MerchantID: merchantID, Action: action, TargetType: "subscription", TargetID: id, Result: "success", SafeMetadata: raw})
}
