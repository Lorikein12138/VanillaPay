package payment

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/platform"
)

func createRequestDigest(in CreateCommand) []byte {
	money := in.money
	if money == "" {
		money = formatMoney(in.OriginalCents)
	}
	hash := sha256.New()
	for _, field := range []string{money, in.Name, in.NotifyURL, in.ReturnURL, in.Param, in.ProductID} {
		_, _ = hash.Write(binary.AppendUvarint(nil, uint64(len(field))))
		_, _ = hash.Write([]byte(field))
	}
	return hash.Sum(nil)
}

func (s *Service) reusableOrder(ctx context.Context, tx pgx.Tx, in CreateCommand, digest []byte) (OrderDTO, bool, error) {
	var d OrderDTO
	var oldDigest []byte
	err := tx.QueryRow(ctx, `SELECT trade_no,purpose,protocol,name,product_id,param,original_cents,channel,actual_cents,status,created_at,expires_at,create_request_digest,is_test
		FROM orders WHERE merchant_id=$1 AND out_trade_no=$2 FOR UPDATE`, in.MerchantID, in.OutTradeNo).
		Scan(&d.TradeNo, &d.Purpose, &d.Protocol, &d.Name, &d.ProductID, &d.Param, &d.OriginalCents, &d.Channel, &d.ActualCents, &d.Status, &d.CreatedAt, &d.ExpiresAt, &oldDigest, &d.IsTest)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderDTO{}, false, nil
	}
	if err != nil {
		return OrderDTO{}, false, err
	}
	if d.IsTest != in.test || d.Purpose != "external" || d.Protocol != in.Protocol || d.Status != "pending" || !d.ExpiresAt.After(s.clock.Now()) ||
		!equalDigest(digest, oldDigest) || in.Channel != "" && (d.Channel == nil || *d.Channel != in.Channel) {
		return OrderDTO{}, false, apiErr("DUPLICATE_ORDER", 409)
	}
	d.OutTradeNo = in.OutTradeNo
	d.MerchantID = strconv.FormatInt(in.MerchantID, 10)
	d.CallbackStatus = "none"
	return d, true, nil
}

func (s *Service) closeProtocolOrder(ctx context.Context, actor platform.Actor, trade, out string) error {
	return sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{actor.MerchantID}, false); err != nil {
			return err
		}
		var id int64
		var selectedTrade, status string
		var expires time.Time
		err := tx.QueryRow(ctx, `SELECT id,trade_no,status,expires_at FROM orders WHERE merchant_id=$1 AND purpose='external'
			AND ($2='' OR trade_no=$2) AND ($3='' OR out_trade_no=$3) FOR UPDATE`, actor.MerchantID, trade, out).
			Scan(&id, &selectedTrade, &status, &expires)
		if errors.Is(err, pgx.ErrNoRows) {
			return apiErr("NOT_FOUND", 404)
		}
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if status != "pending" || !expires.After(now) {
			return apiErr("ORDER_NOT_PAYABLE", 409)
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET status='expired',expire_reason='cancelled',updated_at=$2 WHERE id=$1`, id, now); err != nil {
			return err
		}
		if err := scheduleReservationRelease(ctx, tx, id); err != nil {
			return err
		}
		return auditOrderTx(ctx, tx, actor, actor.MerchantID, "order.close", selectedTrade, nil)
	})
}

func scheduleReservationRelease(ctx context.Context, tx pgx.Tx, orderID int64) error {
	_, err := tx.Exec(ctx, `UPDATE amount_reservations SET release_after=(SELECT expires_at+interval '24 hours' FROM orders WHERE id=$1) WHERE order_id=$1`, orderID)
	return err
}
