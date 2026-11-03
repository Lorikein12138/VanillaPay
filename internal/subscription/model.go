package subscription

import (
	"time"
)

type LocalizedName struct {
	ZhCN string `json:"zh_CN"`
	En   string `json:"en"`
}

type PlanDTO struct {
	ID             string        `json:"id"`
	Revision       int64         `json:"revision"`
	Name           LocalizedName `json:"name"`
	PriceCents     int64         `json:"price_cents"`
	DurationDays   int           `json:"duration_days"`
	Channels       []string      `json:"channels"`
	PurchaseLimit  *int          `json:"purchase_limit"`
	Deleted        bool          `json:"deleted"`
	PurchasedCount *int          `json:"purchased_count,omitempty"`
	RemainingCount *int          `json:"remaining_count,omitempty"`
}

type PlanCommand struct {
	ID            string        `json:"id,omitempty"`
	Revision      int64         `json:"revision,omitempty"`
	Name          LocalizedName `json:"name"`
	PriceCents    int64         `json:"price_cents"`
	DurationDays  int           `json:"duration_days"`
	Channels      []string      `json:"channels"`
	PurchaseLimit *int          `json:"purchase_limit"`
}

type SubscriptionDTO struct {
	Plan      *PlanDTO   `json:"plan"`
	Channels  []string   `json:"channels"`
	ExpiresAt *time.Time `json:"expires_at"`
	Active    bool       `json:"active"`
	Source    string     `json:"source"`
	Revision  int64      `json:"revision"`
}

type PurchaseRequest struct {
	PlanID                       string `json:"plan_id"`
	PlanRevision                 int64  `json:"plan_revision"`
	ConfirmReplace               bool   `json:"confirm_replace"`
	ExpectedSubscriptionRevision int64  `json:"expected_subscription_revision"`
}

type PurchaseDTO struct {
	ID            string     `json:"id"`
	PlanSnapshot  PlanDTO    `json:"plan_snapshot"`
	PurchaseKind  string     `json:"purchase_kind"`
	State         string     `json:"state"`
	PriceCents    int64      `json:"price_cents"`
	TradeNo       *string    `json:"trade_no"`
	CashierURL    *string    `json:"cashier_url"`
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	FulfillmentAt *time.Time `json:"fulfillment_at"`
}

type GrantRequest struct {
	PlanID           string    `json:"plan_id"`
	ExpiresAt        time.Time `json:"expires_at"`
	Channels         []string  `json:"channels"`
	ExpectedRevision int64     `json:"expected_revision"`
	Reason           string    `json:"reason"`
}

type DeletedPlanResult struct {
	Deleted                   bool  `json:"deleted"`
	CancelledPendingPurchases int64 `json:"cancelled_pending_purchases"`
}

type PurchaseFilter struct {
	Start   *time.Time
	End     *time.Time
	BuyerID *int64
	PlanID  *int64
	State   string
	Cursor  string
	Limit   int
}

type AdminPurchaseDTO struct {
	PurchaseDTO
	BuyerID           string `json:"buyer_id"`
	BillingMerchantID string `json:"billing_merchant_id"`
}

type PurchasePage struct {
	Items      []AdminPurchaseDTO `json:"items"`
	NextCursor *string            `json:"next_cursor"`
	Total      int64              `json:"total"`
}
