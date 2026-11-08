package merchant

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"vanillapay/internal/platform"
)

type Handler struct {
	Service       *Service
	PublicBaseURL string
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/console/merchants", h.ListConsole)
	r.Get("/console/merchants/{id}", h.ConsoleDetail)
	r.Post("/console/merchants", h.Create)
	r.Post("/console/merchants/{id}/ban", h.Ban)
	r.Post("/console/merchants/{id}/unban", h.Unban)
	r.Post("/console/merchants/{id}/reset-password", h.ResetPassword)
	r.Get("/merchant/integration", h.Integration)
	r.Post("/merchant/integration/reset-key", h.ResetKey)
	r.Put("/merchant/integration/rsa-public-key", h.RSAPublicKey)
	r.Put("/merchant/integration/ip-whitelist", h.IPWhitelist)
}

func (h *Handler) ListConsole(w http.ResponseWriter, r *http.Request) {
	q := ConsoleMerchantQuery{Query: r.URL.Query().Get("query"), Cursor: r.URL.Query().Get("cursor")}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			merchantWrite(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}, 200)
			return
		}
		q.Limit = limit
	}
	for name, target := range map[string]**bool{"banned": &q.Banned, "subscription_active": &q.SubscriptionActive} {
		if raw := r.URL.Query().Get(name); raw != "" {
			value, err := strconv.ParseBool(raw)
			if err != nil {
				merchantWrite(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}, 200)
				return
			}
			*target = &value
		}
	}
	actor, _ := platform.ActorFromContext(r.Context())
	page, err := h.Service.ListConsole(r.Context(), actor, q)
	merchantWrite(w, r, page, err, 200)
}

func (h *Handler) ConsoleDetail(w http.ResponseWriter, r *http.Request) {
	id, err := merchantID(chi.URLParam(r, "id"))
	if err != nil {
		merchantWrite(w, r, nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	data, err := h.Service.ConsoleDetail(r.Context(), actor, id)
	merchantWrite(w, r, data, err, 200)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateMerchant
	if !merchantDecode(w, r, &req, 1<<20) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.Create(r.Context(), actor, req)
	merchantWrite(w, r, dto, err, http.StatusCreated)
}

func (h *Handler) Ban(w http.ResponseWriter, r *http.Request) {
	var req BanRequest
	if !merchantDecode(w, r, &req, 1<<20) {
		return
	}
	id, err := merchantID(chi.URLParam(r, "id"))
	if err != nil {
		merchantWrite(w, r, nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.Ban(r.Context(), actor, id, req.Reason)
	merchantWrite(w, r, dto, err, 200)
}

func (h *Handler) Unban(w http.ResponseWriter, r *http.Request) {
	if !emptyBody(w, r) {
		return
	}
	id, err := merchantID(chi.URLParam(r, "id"))
	if err != nil {
		merchantWrite(w, r, nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.Unban(r.Context(), actor, id)
	merchantWrite(w, r, dto, err, 200)
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if !merchantDecode(w, r, &req, 1<<20) {
		return
	}
	id, err := merchantID(chi.URLParam(r, "id"))
	if err != nil {
		merchantWrite(w, r, nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	err = h.Service.ResetPassword(r.Context(), actor, id, req.Password, req.Confirmation)
	merchantWrite(w, r, nil, err, 200)
}

func (h *Handler) Integration(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.Integration(r.Context(), actor, h.PublicBaseURL)
	merchantWrite(w, r, dto, err, 200)
}

func (h *Handler) ResetKey(w http.ResponseWriter, r *http.Request) {
	var req ResetKeyRequest
	if !merchantDecode(w, r, &req, 1<<20) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.ResetKey(r.Context(), actor, req.Proof)
	merchantWrite(w, r, dto, err, 200)
}

func (h *Handler) RSAPublicKey(w http.ResponseWriter, r *http.Request) {
	var req RSAKeyRequest
	if !merchantDecode(w, r, &req, 64<<10) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.SetRSAPublicKey(r.Context(), actor, req)
	merchantWrite(w, r, dto, err, 200)
}

func (h *Handler) IPWhitelist(w http.ResponseWriter, r *http.Request) {
	var body IPWhitelistRequest
	if !merchantDecode(w, r, &body, 32<<10) {
		return
	}
	req := IPWhitelist{Enabled: body.Enabled}
	if body.Entries != nil {
		req.Entries = *body.Entries
	}
	actor, _ := platform.ActorFromContext(r.Context())
	err := h.Service.SetIPWhitelist(r.Context(), actor, actor.MerchantID, req)
	merchantWrite(w, r, req, err, 200)
}

func merchantID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid merchant ID")
	}
	return id, nil
}

func merchantDecode(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	if err := platform.DecodeJSONBody(w, r, limit, target); err != nil {
		merchantWrite(w, r, nil, err, 400)
		return false
	}
	return true
}

func emptyBody(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil {
		return true
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil {
		merchantWrite(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}, 400)
		return false
	}
	if len(data) == 0 {
		return true
	}
	if err := platform.DecodeStrictJSON(bytes.NewReader(data), &struct{}{}); err != nil {
		merchantWrite(w, r, nil, err, 400)
		return false
	}
	return true
}

func merchantWrite(w http.ResponseWriter, r *http.Request, data any, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	id := middleware.GetReqID(r.Context())
	if err != nil {
		status = http.StatusInternalServerError
		code, fields := "INTERNAL_ERROR", map[string]string{}
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
