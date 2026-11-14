package payment

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"vanillapay/internal/platform"
)

const testPaymentName = "测试支付"

type TestPaymentRequest struct {
	AmountCents int64   `json:"amount_cents"`
	Channel     *string `json:"channel"`
}

type TestPaymentDTO struct {
	TradeNo    string    `json:"trade_no"`
	CashierURL string    `json:"cashier_url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// CreateTestPayment creates a merchant-console test order. It goes through the
// normal order path, so plan limits, channels and QR codes are checked, but it
// has no notify URL and skips the protocol-only API IP allowlist, product name
// block words and notify-domain checks. Older pending test orders of the
// merchant are closed so at most one test order holds an amount reservation.
func (s *Service) CreateTestPayment(ctx context.Context, actor platform.Actor, amountCents int64, channel string) (TestPaymentDTO, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return TestPaymentDTO{}, apiErr("UNAUTHENTICATED", 401)
	}
	if amountCents < 1 || amountCents > MaxAmountCents {
		return TestPaymentDTO{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400, Fields: map[string]string{"amount_cents": "invalid"}}
	}
	suffix, err := randomOpaque(s.random, 12)
	if err != nil {
		return TestPaymentDTO{}, err
	}
	order, err := s.Create(ctx, CreateCommand{
		MerchantID: actor.MerchantID, Protocol: "v1", Purpose: "external",
		OutTradeNo: "TEST" + strings.Map(testOutTradeRune, suffix), Name: testPaymentName,
		OriginalCents: amountCents, Channel: channel, SourceIP: actor.IP, Actor: actor, test: true,
	})
	if err != nil {
		return TestPaymentDTO{}, err
	}
	return TestPaymentDTO{TradeNo: order.TradeNo, ExpiresAt: order.ExpiresAt}, nil
}

// testOutTradeRune keeps the random suffix inside the out_trade_no alphabet.
func testOutTradeRune(r rune) rune {
	if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
		return r
	}
	return '_'
}

// closePendingTestOrdersTx closes the merchant's still-pending test orders the
// same way the protocol close action does. The caller holds the merchant lock.
func (s *Service) closePendingTestOrdersTx(ctx context.Context, tx pgx.Tx, actor platform.Actor, merchantID int64) error {
	now := s.clock.Now()
	rows, err := tx.Query(ctx, `UPDATE orders SET status='expired',expire_reason='cancelled',updated_at=$2
		WHERE merchant_id=$1 AND is_test AND status='pending' RETURNING id,trade_no`, merchantID, now)
	if err != nil {
		return err
	}
	type closed struct {
		id    int64
		trade string
	}
	var orders []closed
	for rows.Next() {
		var c closed
		if err := rows.Scan(&c.id, &c.trade); err != nil {
			rows.Close()
			return err
		}
		orders = append(orders, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range orders {
		if err := scheduleReservationRelease(ctx, tx, c.id); err != nil {
			return err
		}
		if err := auditOrderTx(ctx, tx, actor, merchantID, "order.close", c.trade, map[string]any{"test": true}); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) TestPayment(w http.ResponseWriter, r *http.Request) {
	var req TestPaymentRequest
	if decodePaymentJSON(w, r, &req) != nil {
		return
	}
	channel := ""
	if req.Channel != nil {
		channel = *req.Channel
		if !platform.ValidPaymentChannel(channel) {
			httpJSON(w, r, nil, platform.InvalidJSONField("channel"), 200)
			return
		}
	}
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.CreateTestPayment(r.Context(), actor, req.AmountCents, channel)
	if err == nil {
		dto.CashierURL = h.cashierURL(dto.TradeNo)
	}
	httpJSON(w, r, dto, err, 201)
}
