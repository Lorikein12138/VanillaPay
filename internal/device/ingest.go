package device

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func validDeviceChannel(value string) bool {
	return value == "alipay" || value == "wxpay" || value == "qqpay"
}

func (s *Service) Ingest(ctx context.Context, device DeviceIdentity, req ReceiptRequest) (ReceiptResult, error) {
	if device.DeviceID <= 0 || device.MerchantID <= 0 {
		return ReceiptResult{}, deviceErr("DEVICE_UNAUTHORIZED", 401)
	}
	actor := platform.Actor{Role: "device", SubjectID: device.DeviceID, MerchantID: device.MerchantID}
	if metadata, ok := platform.RequestMetadataFromContext(ctx); ok {
		actor.IP = metadata.IP
		actor.RequestID = metadata.RequestID
	}
	ctx = platform.WithActor(ctx, actor)
	if !uuidPattern.MatchString(req.EventID) || !validDeviceChannel(req.Channel) || req.AmountCents <= 0 || req.AmountCents > payment.MaxAmountCents || req.OccurredAt.IsZero() || len(req.ProviderTradeNo) > 128 || len(req.PayerHint) > 200 || len([]rune(req.RawText)) > 4096 {
		return ReceiptResult{}, deviceErr("VALIDATION_ERROR", 400)
	}
	digestData, _ := json.Marshal(req)
	digest := sha256.Sum256(digestData)
	rawCipher := []byte(nil)
	var err error
	if req.RawText != "" {
		rawCipher, err = platform.Encrypt(s.key[:], []byte(req.RawText), s.random)
		if err != nil {
			return ReceiptResult{}, err
		}
	}
	receiptID, err := platform.RandomToken(s.random, 18)
	if err != nil {
		return ReceiptResult{}, err
	}
	var result ReceiptResult
	now := s.clock.Now()
	err = sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{device.MerchantID}, false); err != nil {
			return err
		}
		var tokenHash []byte
		if err := tx.QueryRow(ctx, `SELECT token_hash FROM devices WHERE id=$1 AND merchant_id=$2 AND revoked_at IS NULL FOR UPDATE`, device.DeviceID, device.MerchantID).Scan(&tokenHash); err != nil {
			return deviceErr("DEVICE_UNAUTHORIZED", 401)
		}
		var existingID string
		var existingHash []byte
		var existingState, existingReason string
		var existingTrade pgtype.Text
		err := tx.QueryRow(ctx, `SELECT public_id,raw_text_hash,match_state,COALESCE(unmatched_reason,''),o.trade_no FROM receipts r LEFT JOIN orders o ON o.id=r.order_id WHERE r.merchant_id=$1 AND r.event_id=$2 FOR UPDATE OF r`, device.MerchantID, req.EventID).Scan(&existingID, &existingHash, &existingState, &existingReason, &existingTrade)
		if err == nil {
			if len(existingHash) != len(digest) || subtle.ConstantTimeCompare(existingHash, digest[:]) != 1 {
				return deviceErr("EVENT_CONFLICT", 409)
			}
			result = ReceiptResult{ReceiptID: existingID, Duplicate: true, MatchState: existingState, ServerTime: now}
			if existingReason != "" {
				result.UnmatchedReason = &existingReason
			}
			if existingTrade.Valid {
				v := existingTrade.String
				result.TradeNo = &v
			}
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if req.ProviderTradeNo != "" {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM receipts WHERE merchant_id=$1 AND channel=$2 AND provider_trade_no=$3)`, device.MerchantID, req.Channel, req.ProviderTradeNo).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return deviceErr("EVENT_CONFLICT", 409)
			}
		}
		var banned bool
		if err := tx.QueryRow(ctx, `SELECT banned_at IS NOT NULL FROM merchants WHERE id=$1 FOR UPDATE`, device.MerchantID).Scan(&banned); err != nil {
			return err
		}
		var receiptIDNum int64
		if err := tx.QueryRow(ctx, `INSERT INTO receipts(public_id,merchant_id,device_id,event_id,channel,amount_cents,occurred_at,received_at,provider_trade_no,payer_hint,raw_text_cipher,raw_text_hash,match_state,unmatched_reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),$11,$12,'unmatched',$13) RETURNING id`, receiptID, device.MerchantID, device.DeviceID, req.EventID, req.Channel, req.AmountCents, req.OccurredAt, now, req.ProviderTradeNo, req.PayerHint, rawCipher, digest[:], "no_matching_order").Scan(&receiptIDNum); err != nil {
			return err
		}
		reason := "no_matching_order"
		var orderID, tradeMerchant int64
		var tradeNo string
		if banned {
			reason = "merchant_banned"
		} else {
			err := tx.QueryRow(ctx, `SELECT o.id,o.merchant_id,o.trade_no FROM orders o WHERE o.merchant_id=$1 AND o.channel=$2 AND o.actual_cents=$3 AND o.status='pending' AND o.expires_at>$4 AND o.created_at<=$5 AND o.channel_locked_at<=$5 AND $5<o.expires_at AND $6<o.expires_at AND $6<=$4::timestamptz+interval '30 seconds' ORDER BY o.created_at,o.id LIMIT 1 FOR UPDATE`, device.MerchantID, req.Channel, req.AmountCents, now, req.OccurredAt, req.OccurredAt).Scan(&orderID, &tradeMerchant, &tradeNo)
			if err == nil {
				if s.payments == nil {
					return errors.New("payment receipt port is not connected")
				}
				if err := s.payments.ApplyPaidReceiptTx(ctx, tx, orderID, receiptIDNum, req.Channel, req.ProviderTradeNo, req.PayerHint); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE receipts SET match_state='matched',unmatched_reason=NULL,order_id=$1 WHERE id=$2`, orderID, receiptIDNum); err != nil {
					return err
				}
				reason = ""
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			} else {
				var expired bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM orders WHERE merchant_id=$1 AND channel=$2 AND actual_cents=$3 AND status='pending' AND (expires_at<=$4 OR $5>=expires_at))`, device.MerchantID, req.Channel, req.AmountCents, now, req.OccurredAt).Scan(&expired); err == nil && expired {
					reason = "expired"
				}
			}
		}
		result = ReceiptResult{ReceiptID: receiptID, MatchState: "unmatched", ServerTime: now}
		if reason != "" {
			result.UnmatchedReason = &reason
		}
		if reason == "" {
			result.MatchState = "matched"
			result.TradeNo = &tradeNo
		}
		return nil
	})
	return result, err
}
