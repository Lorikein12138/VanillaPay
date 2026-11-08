package portal

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"vanillapay/internal/platform"
)

type Handler struct{ Service *Service }

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/merchant/stats", h.Stats)
	r.Get("/console/stats", h.Stats)
	r.Get("/merchant/receipts", h.Receipts)
	r.Get("/merchant/audit-logs", h.Audits)
	r.Get("/console/audit-logs", h.Audits)
	r.Get("/merchant/notifications", h.Notifications)
	r.Post("/merchant/notifications/read", h.MarkNotificationsRead)
	r.Post("/merchant/announcements/{id}/read", h.MarkAnnouncementRead)
	r.Get("/console/announcements", h.Announcements)
	r.Post("/console/announcements", h.SaveAnnouncement)
	r.Put("/console/announcements/{id}", h.SaveAnnouncement)
	r.Delete("/console/announcements/{id}", h.DeleteAnnouncement)
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	start, startErr := time.Parse(time.RFC3339, r.URL.Query().Get("start"))
	end, endErr := time.Parse(time.RFC3339, r.URL.Query().Get("end"))
	if startErr != nil || endErr != nil {
		portalWrite(w, r, nil, apiError("VALIDATION_ERROR", 400), 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.Stats(r.Context(), actor, StatsQuery{Start: start, End: end, Timezone: r.URL.Query().Get("timezone")})
	portalWrite(w, r, result, err, 200)
}

func (h *Handler) Receipts(w http.ResponseWriter, r *http.Request) {
	q, err := parseReceiptFilter(r)
	if err != nil {
		portalWrite(w, r, nil, err, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	page, err := h.Service.ListReceipts(r.Context(), actor, q)
	portalWrite(w, r, page, err, 200)
}

func (h *Handler) Audits(w http.ResponseWriter, r *http.Request) {
	q, err := parseAuditFilter(r)
	if err != nil {
		portalWrite(w, r, nil, err, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	page, err := h.Service.ListAudits(r.Context(), actor, q)
	portalWrite(w, r, page, err, 200)
}

func (h *Handler) Notifications(w http.ResponseWriter, r *http.Request) {
	q, err := parsePageQuery(r)
	if err != nil {
		portalWrite(w, r, nil, err, 200)
		return
	}
	q.UnreadOnly = r.URL.Query().Get("unread_only") == "true"
	actor, _ := platform.ActorFromContext(r.Context())
	page, err := h.Service.ListNotifications(r.Context(), actor, q)
	portalWrite(w, r, page, err, 200)
}

func (h *Handler) MarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	var body MarkNotificationsReadRequest
	if !portalDecode(w, r, &body) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	req := ReadRequest{All: body.All}
	if body.IDs != nil {
		req.IDs = *body.IDs
	}
	result, err := h.Service.MarkRead(r.Context(), actor, req)
	portalWrite(w, r, result, err, 200)
}

func (h *Handler) MarkAnnouncementRead(w http.ResponseWriter, r *http.Request) {
	var req AnnouncementRevisionRequest
	if !portalDecode(w, r, &req) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	err := h.Service.MarkAnnouncementRead(r.Context(), actor, chi.URLParam(r, "id"), req.Revision)
	portalWrite(w, r, nil, err, 200)
}

func (h *Handler) Announcements(w http.ResponseWriter, r *http.Request) {
	q, err := parsePageQuery(r)
	if err != nil {
		portalWrite(w, r, nil, err, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	page, err := h.Service.ListAnnouncements(r.Context(), actor, q)
	portalWrite(w, r, page, err, 200)
}

func (h *Handler) SaveAnnouncement(w http.ResponseWriter, r *http.Request) {
	var req AnnouncementCommand
	if !portalDecode(w, r, &req) {
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		req.ID = id
	}
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.SaveAnnouncement(r.Context(), actor, req)
	status := http.StatusCreated
	if req.ID != "" {
		status = http.StatusOK
	}
	portalWrite(w, r, dto, err, status)
}

func (h *Handler) DeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	var req AnnouncementRevisionRequest
	if !portalDecode(w, r, &req) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	err := h.Service.DeleteAnnouncement(r.Context(), actor, chi.URLParam(r, "id"), req.Revision)
	portalWrite(w, r, nil, err, 200)
}

func parsePageQuery(r *http.Request) (PageQuery, error) {
	limit, err := parseLimitQuery(r.URL.Query().Get("limit"))
	if err != nil {
		return PageQuery{}, err
	}
	return PageQuery{Cursor: r.URL.Query().Get("cursor"), Limit: limit}, nil
}

func parseReceiptFilter(r *http.Request) (ReceiptFilter, error) {
	start, err := parseTimeQuery(r.URL.Query().Get("start"))
	if err != nil {
		return ReceiptFilter{}, err
	}
	end, err := parseTimeQuery(r.URL.Query().Get("end"))
	if err != nil {
		return ReceiptFilter{}, err
	}
	limit, err := parseLimitQuery(r.URL.Query().Get("limit"))
	if err != nil {
		return ReceiptFilter{}, err
	}
	return ReceiptFilter{Start: start, End: end, Channel: r.URL.Query().Get("channel"), MatchState: r.URL.Query().Get("match_state"), Cursor: r.URL.Query().Get("cursor"), Limit: limit}, nil
}

func parseAuditFilter(r *http.Request) (AuditFilter, error) {
	start, err := parseTimeQuery(r.URL.Query().Get("start"))
	if err != nil {
		return AuditFilter{}, err
	}
	end, err := parseTimeQuery(r.URL.Query().Get("end"))
	if err != nil {
		return AuditFilter{}, err
	}
	limit, err := parseLimitQuery(r.URL.Query().Get("limit"))
	if err != nil {
		return AuditFilter{}, err
	}
	var merchantID *int64
	if raw := r.URL.Query().Get("merchant_id"); raw != "" {
		id, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || id <= 0 {
			return AuditFilter{}, apiError("VALIDATION_ERROR", 400)
		}
		merchantID = &id
	}
	return AuditFilter{Start: start, End: end, Action: r.URL.Query().Get("action"), Result: r.URL.Query().Get("result"), MerchantID: merchantID, ActorRole: r.URL.Query().Get("actor_role"), Cursor: r.URL.Query().Get("cursor"), Limit: limit}, nil
}

func parseTimeQuery(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, apiError("VALIDATION_ERROR", 400)
	}
	return &value, nil
}

func parseLimitQuery(raw string) (int, error) {
	if raw == "" {
		return 20, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 100 {
		return 0, apiError("VALIDATION_ERROR", 400)
	}
	return value, nil
}

func portalDecode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := platform.DecodeJSONBody(w, r, 1<<20, dst); err != nil {
		portalWrite(w, r, nil, err, 400)
		return false
	}
	return true
}

func portalWrite(w http.ResponseWriter, r *http.Request, data any, err error, successStatus int) {
	status := successStatus
	body := map[string]any{"data": data, "error": nil, "request_id": middleware.GetReqID(r.Context())}
	if err != nil {
		status = http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		fields := map[string]string{}
		var api *platform.Error
		if errors.As(err, &api) {
			code = api.Code
			if api.Fields != nil {
				fields = api.Fields
			}
			if api.HTTPStatus != 0 {
				status = api.HTTPStatus
			}
		}
		body = map[string]any{"error": map[string]any{"code": code, "message": code, "fields": fields, "details": map[string]any{}}, "request_id": middleware.GetReqID(r.Context())}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
