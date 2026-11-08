package device

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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
	r.Post("/device/bind", h.Redeem)
	r.Post("/device/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		identity, _ := platform.DeviceFromContext(r.Context())
		h.Heartbeat(w, r, identity)
	})
	r.Post("/device/receipts", func(w http.ResponseWriter, r *http.Request) {
		identity, _ := platform.DeviceFromContext(r.Context())
		h.Ingest(w, r, identity)
	})
	r.Get("/merchant/device", h.Device)
	r.Post("/merchant/device/bindings", h.NewBinding)
	r.Get("/merchant/device/bindings/{id}", h.Binding)
	r.Delete("/merchant/device", h.Revoke)
	r.Get("/merchant/receipts/{id}", h.OpenReceipt)
}

func (h *Handler) Redeem(w http.ResponseWriter, r *http.Request) {
	var req RedeemRequest
	if !deviceDecode(w, r, &req, 32<<10) {
		return
	}
	result, err := h.Service.Redeem(r.Context(), req)
	deviceWrite(w, r, result, err, http.StatusCreated)
}

func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request, identity DeviceIdentity) {
	var req HeartbeatRequest
	if !deviceDecode(w, r, &req, 32<<10) {
		return
	}
	result, err := h.Service.Heartbeat(r.Context(), identity, req)
	deviceWrite(w, r, result, err, http.StatusOK)
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request, identity DeviceIdentity) {
	var req ReceiptRequest
	if !deviceDecode(w, r, &req, 32<<10) {
		return
	}
	result, err := h.Service.Ingest(r.Context(), identity, req)
	status := http.StatusCreated
	if result.Duplicate {
		status = http.StatusOK
	}
	deviceWrite(w, r, result, err, status)
}

func (h *Handler) Device(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.Device(r.Context(), actor)
	deviceWrite(w, r, result, err, http.StatusOK)
}

func (h *Handler) NewBinding(w http.ResponseWriter, r *http.Request) {
	var req CreateBindingRequest
	if !deviceDecode(w, r, &req, 32<<10) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.NewBinding(r.Context(), actor, req.ReplaceConfirmed)
	deviceWrite(w, r, result, err, http.StatusCreated)
}

func (h *Handler) Binding(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.Binding(r.Context(), actor, chi.URLParam(r, "id"))
	deviceWrite(w, r, result, err, http.StatusOK)
}

func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	var req UnbindRequest
	if !deviceDecode(w, r, &req, 32<<10) {
		return
	}
	if !req.Confirm {
		deviceWrite(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}, http.StatusOK)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	err := h.Service.Revoke(r.Context(), actor)
	deviceWrite(w, r, nil, err, http.StatusOK)
}

func (h *Handler) OpenReceipt(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.OpenReceipt(r.Context(), actor, chi.URLParam(r, "id"))
	deviceWrite(w, r, result, err, http.StatusOK)
}

func deviceDecode(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	if err := platform.DecodeJSONBody(w, r, limit, target); err != nil {
		deviceWrite(w, r, nil, err, http.StatusOK)
		return false
	}
	return true
}

func deviceWrite(w http.ResponseWriter, r *http.Request, data any, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	id := middleware.GetReqID(r.Context())
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

func ParseDeviceID(value string) (int64, error) { return strconv.ParseInt(value, 10, 64) }
