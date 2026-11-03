package subscription

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"vanillapay/internal/platform"
)

var errPendingBillingLockSetChanged = fmt.Errorf("pending purchase billing lock set changed")

type purchaseRowsQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func pendingBillingMerchantIDs(ctx context.Context, db purchaseRowsQuerier, buyerID int64) ([]int64, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT billing_merchant_id FROM purchases WHERE buyer_id=$1 AND state='pending' ORDER BY billing_merchant_id`, buyerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) ListPurchases(ctx context.Context, actor platform.Actor, q PurchaseFilter) (PurchasePage, error) {
	if actor.Role != "admin" {
		return PurchasePage{}, subError("FORBIDDEN", 403)
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.Limit < 1 || q.Limit > 100 || q.State != "" && q.State != "pending" && q.State != "paid" && q.State != "expired" && q.State != "cancelled" && q.State != "refunded" {
		return PurchasePage{}, subError("VALIDATION_ERROR", 400)
	}
	var cursorTime *time.Time
	var cursorID *int64
	if q.Cursor != "" {
		at, id, err := s.decodePurchaseCursor(q.Cursor, q)
		if err != nil {
			return PurchasePage{}, subError("VALIDATION_ERROR", 400)
		}
		cursorTime, cursorID = &at, &id
	}
	var page PurchasePage
	total, err := s.repository.CountPurchases(ctx, q)
	if err != nil {
		return page, err
	}
	page.Total = total
	rows, err := s.repository.ListPurchases(ctx, q, cursorTime, cursorID, q.Limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	page.Items = make([]AdminPurchaseDTO, 0, q.Limit)
	var lastAt time.Time
	var lastID int64
	more := false
	for rows.Next() {
		var id, buyerID, billingID int64
		var dto AdminPurchaseDTO
		var snapshot []byte
		var trade *string
		var paid, fulfilled pgtype.Timestamptz
		if err := rows.Scan(&id, &dto.ID, &snapshot, &dto.PurchaseKind, &dto.State, &trade, &dto.CreatedAt, &paid, &dto.ExpiresAt, &fulfilled, &buyerID, &billingID); err != nil {
			return page, err
		}
		if len(page.Items) == q.Limit {
			more = true
			break
		}
		if err := json.Unmarshal(snapshot, &dto.PlanSnapshot); err != nil {
			return page, err
		}
		dto.PriceCents = dto.PlanSnapshot.PriceCents
		dto.TradeNo = trade
		if trade != nil {
			cashier := s.cashierURL(*trade)
			dto.CashierURL = &cashier
		}
		if paid.Valid {
			value := paid.Time
			dto.PaidAt = &value
		}
		if fulfilled.Valid {
			value := fulfilled.Time
			dto.FulfillmentAt = &value
		}
		dto.BuyerID = strconv.FormatInt(buyerID, 10)
		dto.BillingMerchantID = strconv.FormatInt(billingID, 10)
		page.Items = append(page.Items, dto)
		lastAt, lastID = dto.CreatedAt, id
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if more && len(page.Items) > 0 {
		cursor := s.encodePurchaseCursor(lastAt, lastID, q)
		page.NextCursor = &cursor
	}
	return page, nil
}

func purchaseFilterFingerprint(q PurchaseFilter) string {
	start, end, buyer, plan := "*", "*", "*", "*"
	if q.Start != nil {
		start = q.Start.UTC().Format(time.RFC3339Nano)
	}
	if q.End != nil {
		end = q.End.UTC().Format(time.RFC3339Nano)
	}
	if q.BuyerID != nil {
		buyer = strconv.FormatInt(*q.BuyerID, 10)
	}
	if q.PlanID != nil {
		plan = strconv.FormatInt(*q.PlanID, 10)
	}
	digest := sha256.Sum256([]byte(strings.Join([]string{start, end, buyer, plan, q.State}, "|")))
	return hex.EncodeToString(digest[:])
}

func (s *Service) encodePurchaseCursor(at time.Time, id int64, q PurchaseFilter) string {
	payload := at.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(id, 10) + "|" + purchaseFilterFingerprint(q)
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + hex.EncodeToString(mac.Sum(nil))))
}

func (s *Service) decodePurchaseCursor(cursor string, q PurchaseFilter) (time.Time, int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, err
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 || parts[2] != purchaseFilterFingerprint(q) {
		return time.Time{}, 0, fmt.Errorf("invalid cursor")
	}
	provided, err := hex.DecodeString(parts[3])
	if err != nil {
		return time.Time{}, 0, err
	}
	mac := hmac.New(sha256.New, s.key[:])
	_, _ = mac.Write([]byte(strings.Join(parts[:3], "|")))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return time.Time{}, 0, fmt.Errorf("invalid cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, 0, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	return at, id, err
}
