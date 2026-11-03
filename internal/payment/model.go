package payment

import (
	"context"
	"crypto/rsa"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"vanillapay/internal/platform"
)

type CreateCommand struct {
	MerchantID    int64
	Protocol      string
	Purpose       string
	OutTradeNo    string
	Name          string
	ProductID     string
	Param         string
	OriginalCents int64
	NotifyURL     string
	ReturnURL     string
	ClientIP      string
	SourceIP      string
	BuyerHint     string
	Channel       string
	Channels      []string
	ExpireSeconds *int
	PurchaseID    *int64
	BuyerID       *int64
	Actor         platform.Actor
}

type OrderDTO struct {
	TradeNo        string     `json:"trade_no"`
	OutTradeNo     string     `json:"out_trade_no"`
	MerchantID     string     `json:"merchant_id"`
	Purpose        string     `json:"purpose"`
	Protocol       string     `json:"protocol"`
	Channel        *string    `json:"channel"`
	Name           string     `json:"name"`
	ProductID      string     `json:"product_id"`
	Param          string     `json:"param"`
	OriginalCents  int64      `json:"original_cents"`
	ActualCents    *int64     `json:"actual_cents"`
	Status         string     `json:"status"`
	ExpireReason   *string    `json:"expire_reason"`
	CreatedAt      time.Time  `json:"created_at"`
	PaidAt         *time.Time `json:"paid_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	RefundedAt     *time.Time `json:"refunded_at"`
	CallbackStatus string     `json:"callback_status"`
}

type CheckoutDTO struct {
	TradeNo             string            `json:"trade_no"`
	Name                string            `json:"name"`
	ProductID           string            `json:"product_id"`
	Merchant            map[string]string `json:"merchant"`
	Status              string            `json:"status"`
	Reason              *string           `json:"reason"`
	OriginalCents       int64             `json:"original_cents"`
	ActualCents         *int64            `json:"actual_cents"`
	Channel             *string           `json:"channel"`
	AvailableChannels   []string          `json:"available_channels"`
	QRImageURL          *string           `json:"qr_image_url"`
	ExpiresAt           time.Time         `json:"expires_at"`
	ServerTime          time.Time         `json:"server_time"`
	DeviceOnline        bool              `json:"device_online"`
	CanPay              bool              `json:"can_pay"`
	ReturnURLSigned     *string           `json:"return_url_signed"`
	RequiresExactAmount bool              `json:"requires_exact_amount"`
}

type OrderSelector struct {
	TradeNo    string
	OutTradeNo string
	MerchantID int64
}
type PaidCommand struct {
	Channel         string
	ReceiptID       *int64
	Manual          bool
	ManualReason    string
	ActorID         *int64
	ProviderTradeNo string
	BuyerHint       string
}
type SupplementRequest struct {
	Reason  string `json:"reason"`
	Channel string `json:"channel,omitempty"`
}
type RefundCommand struct {
	MerchantID    int64
	TradeNo       string
	OutRefundNo   string
	AmountCents   int64
	Reason        string
	RequestDigest []byte
	Actor         platform.Actor
}
type RefundDTO struct {
	ID          string     `json:"id"`
	TradeNo     string     `json:"trade_no"`
	OutRefundNo *string    `json:"out_refund_no"`
	AmountCents int64      `json:"amount_cents"`
	Status      string     `json:"status"`
	Reason      *string    `json:"reason"`
	RequestedAt time.Time  `json:"requested_at"`
	HandledAt   *time.Time `json:"handled_at"`
	HandledBy   *string    `json:"handled_by"`
}
type RefundResolution struct {
	Action  string `json:"action"`
	Reason  string `json:"reason"`
	Confirm bool   `json:"confirm"`
}
type RefundAttempt struct {
	ID              string     `json:"id"`
	EventID         string     `json:"event_id"`
	Generation      int        `json:"generation"`
	AttemptNo       int        `json:"attempt_no"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	HTTPStatus      *int       `json:"http_status"`
	DurationMS      *int64     `json:"duration_ms"`
	ResponseExcerpt string     `json:"response_excerpt"`
	ErrorCode       *string    `json:"error_code"`
	NextAttemptAt   *time.Time `json:"next_attempt_at"`
	URLRedacted     string     `json:"url_redacted"`
}
type BillingOrderCommand struct {
	PurchaseID        int64
	PublicPurchaseID  string
	BuyerID           int64
	BillingMerchantID int64
	Name              string
	AmountCents       int64
	Channels          []string
	ExpiresAt         time.Time
}
type BillingOrderResult struct {
	TradeNo    string
	CashierURL string
	OrderID    int64
}
type Fulfillment interface {
	PaidTx(context.Context, pgx.Tx, int64, time.Time) error
	RefundedTx(context.Context, pgx.Tx, int64, time.Time) error
}
type OrderEligibility interface {
	CheckCreateTx(context.Context, pgx.Tx, int64, string, int64) (EligibilitySnapshot, error)
	CheckChannelTx(context.Context, pgx.Tx, int64, string, int64) (EligibilitySnapshot, error)
}
type EligibilitySnapshot struct {
	MerchantUsername       string
	PID                    string
	KeyCipher              []byte
	KeyVersion             int32
	Channels               []string
	QRAssets               map[string]string
	DeviceOnline           bool
	Banned                 bool
	MinAmountCents         int64
	MaxAmountCents         int64
	DefaultTTLSeconds      int
	MaxTTLSeconds          int
	FloatMinCents          int
	FloatMaxCents          int
	FloatDefaultCents      int
	FloatDirection         string
	MerchantTTL            *int
	MerchantFloatLimit     *int
	MerchantFloatDirection *string
}
type V2Signature struct {
	PrivateKey *rsa.PrivateKey
	KeyID      string
}
type V2Request struct {
	Params    url.Values
	PublicKey *rsa.PublicKey
	Now       time.Time
}
