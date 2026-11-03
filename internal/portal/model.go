package portal

import (
	"encoding/json"
	"time"
)

type PageQuery struct {
	Cursor     string `json:"cursor,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	UnreadOnly bool   `json:"unread_only,omitempty"`
}

type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
	Total      int64   `json:"total"`
}

type LocalizedText struct {
	ZhCN string `json:"zh_CN"`
	En   string `json:"en"`
}

type AnnouncementCommand struct {
	ID       string        `json:"id,omitempty"`
	Revision int64         `json:"revision,omitempty"`
	Title    LocalizedText `json:"title"`
	Body     LocalizedText `json:"body"`
	Popup    bool          `json:"popup"`
}

type AnnouncementDTO struct {
	ID          string        `json:"id"`
	Revision    int64         `json:"revision"`
	Title       LocalizedText `json:"title"`
	Body        LocalizedText `json:"body"`
	Popup       bool          `json:"popup"`
	PublishedAt time.Time     `json:"published_at"`
	Read        bool          `json:"read"`
}

type NotificationDTO struct {
	ID        string              `json:"id"`
	Kind      string              `json:"kind"`
	SourceID  string              `json:"source_id"`
	Title     LocalizedText       `json:"title"`
	Summary   LocalizedText       `json:"summary"`
	CreatedAt time.Time           `json:"created_at"`
	ReadAt    *time.Time          `json:"read_at"`
	Action    *NotificationAction `json:"action"`
}

type NotificationAction struct {
	Page     string `json:"page"`
	EntityID string `json:"entity_id"`
}

type NotificationPage struct {
	Items              []NotificationDTO `json:"items"`
	NextCursor         *string           `json:"next_cursor"`
	Total              int64             `json:"total"`
	UnreadCount        int64             `json:"unread_count"`
	PopupAnnouncements []AnnouncementDTO `json:"popup_announcements"`
}

type ReadRequest struct {
	IDs []string `json:"ids,omitempty"`
	All bool     `json:"all,omitempty"`
}
type ReadResult struct {
	UnreadCount int64 `json:"unread_count"`
}

type StatsQuery struct {
	Start    time.Time
	End      time.Time
	Timezone string
}
type ChannelStats struct {
	AmountCents  int64 `json:"amount_cents"`
	OrderCount   int64 `json:"order_count"`
	PaidCount    int64 `json:"paid_count"`
	PendingCount int64 `json:"pending_count"`
	ExpiredCount int64 `json:"expired_count"`
}
type TrendPoint struct {
	Date         string `json:"date"`
	AmountCents  int64  `json:"amount_cents"`
	PaidCount    int64  `json:"paid_count"`
	PendingCount int64  `json:"pending_count"`
	ExpiredCount int64  `json:"expired_count"`
}
type StatsDTO struct {
	Start                    time.Time               `json:"start"`
	End                      time.Time               `json:"end"`
	Timezone                 string                  `json:"timezone"`
	AmountBasis              string                  `json:"amount_basis"`
	OrderBasis               string                  `json:"order_basis"`
	TotalAmountCents         int64                   `json:"total_amount_cents"`
	TotalOrderCount          int64                   `json:"total_order_count"`
	StatusCounts             map[string]int64        `json:"status_counts"`
	Channels                 map[string]ChannelStats `json:"channels"`
	UnassignedCount          int64                   `json:"unassigned_count"`
	Trend                    []TrendPoint            `json:"trend"`
	MerchantCount            *int64                  `json:"merchant_count,omitempty"`
	ActiveSubscriptionCount  *int64                  `json:"active_subscription_count,omitempty"`
	SubscriptionRevenueCents *int64                  `json:"subscription_revenue_cents,omitempty"`
}

type OrderFilter struct {
	Start      *time.Time
	End        *time.Time
	Status     string
	Channel    string
	Query      string
	MerchantID *int64
	Cursor     string
	Limit      int
}
type OrderDTO struct {
	TradeNo          string     `json:"trade_no"`
	OutTradeNo       string     `json:"out_trade_no"`
	MerchantID       string     `json:"merchant_id"`
	MerchantUsername string     `json:"merchant_username"`
	Purpose          string     `json:"purpose"`
	Protocol         string     `json:"protocol"`
	Channel          *string    `json:"channel"`
	Name             string     `json:"name"`
	ProductID        string     `json:"product_id"`
	Param            string     `json:"param"`
	OriginalCents    int64      `json:"original_cents"`
	ActualCents      *int64     `json:"actual_cents"`
	Status           string     `json:"status"`
	ExpireReason     *string    `json:"expire_reason"`
	IsManual         bool       `json:"is_manual"`
	ManualReason     *string    `json:"manual_reason"`
	CreatedAt        time.Time  `json:"created_at"`
	PaidAt           *time.Time `json:"paid_at"`
	ExpiresAt        time.Time  `json:"expires_at"`
	RefundedAt       *time.Time `json:"refunded_at"`
	CallbackStatus   string     `json:"callback_status"`
	ReceiptID        *string    `json:"receipt_id"`
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

type ReceiptFilter struct {
	Start      *time.Time
	End        *time.Time
	Channel    string
	MatchState string
	Cursor     string
	Limit      int
}
type ReceiptDTO struct {
	ID              string    `json:"id"`
	EventID         string    `json:"event_id"`
	DeviceID        string    `json:"device_id"`
	Channel         string    `json:"channel"`
	AmountCents     int64     `json:"amount_cents"`
	OccurredAt      time.Time `json:"occurred_at"`
	ReceivedAt      time.Time `json:"received_at"`
	ProviderTradeNo *string   `json:"provider_trade_no"`
	PayerHint       *string   `json:"payer_hint"`
	MatchState      string    `json:"match_state"`
	UnmatchedReason *string   `json:"unmatched_reason"`
	TradeNo         *string   `json:"trade_no"`
	RawText         *string   `json:"raw_text,omitempty"`
}

type AuditFilter struct {
	Start      *time.Time
	End        *time.Time
	Action     string
	Result     string
	MerchantID *int64
	ActorRole  string
	Cursor     string
	Limit      int
}
type AuditDTO struct {
	ID           string          `json:"id"`
	ActorRole    string          `json:"actor_role"`
	ActorID      *string         `json:"actor_id"`
	MerchantID   *string         `json:"merchant_id"`
	Action       string          `json:"action"`
	TargetType   string          `json:"target_type"`
	TargetID     string          `json:"target_id"`
	Result       string          `json:"result"`
	IP           *string         `json:"ip"`
	CreatedAt    time.Time       `json:"created_at"`
	RequestID    string          `json:"request_id"`
	SafeMetadata json.RawMessage `json:"safe_metadata"`
}
