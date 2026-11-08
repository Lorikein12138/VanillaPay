package subscription

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"vanillapay/internal/platform"
)

type Handler struct{ Service *Service }

func (h *Handler) Routes() chi.Router { r := chi.NewRouter(); h.RegisterRoutes(r); return r }
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/public/plans", h.PublicPlans)
	r.Get("/merchant/subscription", h.Current)
	r.Post("/merchant/subscription/purchases", h.Purchase)
	r.Get("/merchant/subscription/purchases/{id}", h.GetPurchase)
	r.Post("/merchant/subscription/purchases/{id}/cancel", h.CancelPurchase)
	r.Get("/console/plans", h.ConsolePlans)
	r.Post("/console/plans", h.SavePlan)
	r.Put("/console/plans/{id}", h.SavePlan)
	r.Delete("/console/plans/{id}", h.DeletePlan)
	r.Put("/console/merchants/{id}/subscription", h.Grant)
	r.Get("/console/purchases", h.Purchases)
}

func (h *Handler) Purchases(w http.ResponseWriter, r *http.Request) {
	q := PurchaseFilter{State: r.URL.Query().Get("state"), Cursor: r.URL.Query().Get("cursor")}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			subWrite(w, r, nil, subError("VALIDATION_ERROR", 400), 200)
			return
		}
		q.Limit = limit
	}
	for _, item := range []struct {
		name string
		dst  **time.Time
	}{{"start", &q.Start}, {"end", &q.End}} {
		if raw := r.URL.Query().Get(item.name); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				subWrite(w, r, nil, subError("VALIDATION_ERROR", 400), 200)
				return
			}
			*item.dst = &parsed
		}
	}
	for _, item := range []struct {
		name string
		dst  **int64
	}{{"merchant_id", &q.BuyerID}, {"plan_id", &q.PlanID}} {
		if raw := r.URL.Query().Get(item.name); raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 {
				subWrite(w, r, nil, subError("VALIDATION_ERROR", 400), 200)
				return
			}
			*item.dst = &id
		}
	}
	actor, _ := platform.ActorFromContext(r.Context())
	page, err := h.Service.ListPurchases(r.Context(), actor, q)
	subWrite(w, r, page, err, http.StatusOK)
}

func (h *Handler) PublicPlans(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.ListPlans(r.Context(), false, platform.Actor{Role: "public"})
	subWrite(w, r, map[string]any{"items": items}, err, 200)
}
func (h *Handler) Current(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	current, err := h.Service.GetCurrent(r.Context(), actor)
	if err != nil {
		subWrite(w, r, nil, err, 200)
		return
	}
	plans, err := h.Service.ListPlans(r.Context(), false, actor)
	if err != nil {
		subWrite(w, r, nil, err, 200)
		return
	}
	var pending *PurchaseDTO
	var rawID string
	var raw []byte
	var state, kind string
	var trade *string
	var created, expires time.Time
	err = h.Service.pool.QueryRow(r.Context(), `SELECT public_id,plan_snapshot,state,purchase_kind,payment_trade_no,created_at,expires_at FROM purchases WHERE buyer_id=$1 AND state='pending' ORDER BY created_at DESC LIMIT 1`, actor.MerchantID).Scan(&rawID, &raw, &state, &kind, &trade, &created, &expires)
	if err == nil {
		var snapshot PlanDTO
		_ = json.Unmarshal(raw, &snapshot)
		p := PurchaseDTO{ID: rawID, PlanSnapshot: snapshot, PurchaseKind: kind, State: state, PriceCents: snapshot.PriceCents, TradeNo: trade, CreatedAt: created, ExpiresAt: expires}
		pending = &p
	} else if !errors.Is(err, pgx.ErrNoRows) {
		subWrite(w, r, nil, err, 200)
		return
	}
	var ready bool
	var pid string
	var keyConfigured bool
	_ = h.Service.pool.QueryRow(r.Context(), `SELECT billing->>'merchant_pid' FROM site_settings WHERE singleton=true`).Scan(&pid)
	_ = h.Service.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM secret_settings WHERE name='billing_key' AND encrypted_value IS NOT NULL)`).Scan(&keyConfigured)
	ready = pid != "" && keyConfigured
	subWrite(w, r, map[string]any{"current": current, "plans": plans, "pending_purchase": pending, "billing_ready": ready}, nil, 200)
}
func (h *Handler) Purchase(w http.ResponseWriter, r *http.Request) {
	var req PurchaseRequest
	if !subDecode(w, r, &req) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.Purchase(r.Context(), actor, req, r.Header.Get("Idempotency-Key"))
	subWrite(w, r, result, err, 201)
}
func (h *Handler) GetPurchase(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	var dto PurchaseDTO
	var snapshot []byte
	var paid, fulfilled pgtype.Timestamptz
	var trade *string
	err := h.Service.pool.QueryRow(r.Context(), `SELECT public_id,plan_snapshot,purchase_kind,state,payment_trade_no,created_at,paid_at,expires_at,fulfillment_at FROM purchases WHERE public_id=$1 AND buyer_id=$2`, chi.URLParam(r, "id"), actor.MerchantID).Scan(&dto.ID, &snapshot, &dto.PurchaseKind, &dto.State, &trade, &dto.CreatedAt, &paid, &dto.ExpiresAt, &fulfilled)
	if err != nil {
		subWrite(w, r, nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}, 200)
		return
	}
	_ = json.Unmarshal(snapshot, &dto.PlanSnapshot)
	dto.PriceCents = dto.PlanSnapshot.PriceCents
	dto.TradeNo = trade
	if trade != nil {
		u := h.Service.cashierURL(*trade)
		dto.CashierURL = &u
	}
	if paid.Valid {
		v := paid.Time
		dto.PaidAt = &v
	}
	if fulfilled.Valid {
		v := fulfilled.Time
		dto.FulfillmentAt = &v
	}
	subWrite(w, r, dto, nil, 200)
}
func (h *Handler) CancelPurchase(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.CancelPurchase(r.Context(), actor, chi.URLParam(r, "id"))
	subWrite(w, r, result, err, 200)
}
func (h *Handler) ConsolePlans(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	items, err := h.Service.ListPlans(r.Context(), r.URL.Query().Get("include_deleted") == "true", actor)
	subWrite(w, r, map[string]any{"items": items, "next_cursor": nil, "total": len(items)}, err, 200)
}
func (h *Handler) SavePlan(w http.ResponseWriter, r *http.Request) {
	var req PlanCommand
	if !subDecode(w, r, &req) {
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		req.ID = id
	}
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.SavePlan(r.Context(), actor, req)
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}
	subWrite(w, r, result, err, status)
}
func (h *Handler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Revision int64 `json:"revision"`
	}
	if !subDecode(w, r, &req) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.DeletePlan(r.Context(), actor, chi.URLParam(r, "id"), req.Revision)
	subWrite(w, r, result, err, 200)
}
func (h *Handler) Grant(w http.ResponseWriter, r *http.Request) {
	var req GrantRequest
	if !subDecode(w, r, &req) {
		return
	}
	merchantID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		subWrite(w, r, nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.Grant(r.Context(), actor, merchantID, req)
	subWrite(w, r, result, err, 200)
}

func subDecode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(target) != nil || dec.Decode(new(any)) != io.EOF {
		subWrite(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}, 400)
		return false
	}
	return true
}
func subWrite(w http.ResponseWriter, r *http.Request, data any, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	id := middleware.GetReqID(r.Context())
	if err != nil {
		status = 500
		code := "INTERNAL_ERROR"
		fields := map[string]string{}
		var api *platform.Error
		if errors.As(err, &api) {
			code = api.Code
			if api.Fields != nil {
				fields = api.Fields
			}
			if api.HTTPStatus > 0 {
				status = api.HTTPStatus
			}
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": code, "fields": fields, "details": map[string]any{}}, "request_id": id})
		return
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "error": nil, "request_id": id})
}
