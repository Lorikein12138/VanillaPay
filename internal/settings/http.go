package settings

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"vanillapay/internal/platform"
)

type Handler struct {
	Service       *Service
	SecureCookies bool
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	r.Get("/media/{asset_id}", h.OpenAsset)
	return r
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/public/config", h.PublicConfig)
	r.Get("/public/legal/{kind}", h.Legal)
	r.Get("/console/settings", h.GetSettings)
	r.Put("/console/settings/{group}", h.UpdateGroup)
	r.Post("/console/settings/mail/test", h.QueueMailTest)
	r.Get("/console/settings/mail/test/{id}", h.MailTestStatus)
	r.Post("/console/assets/logo", h.UploadLogo)
	r.Get("/merchant/collection/codes", h.QRSlots)
	r.Put("/merchant/collection/codes/{channel}", h.UploadQR)
	r.Patch("/merchant/collection/codes/{channel}", h.SetQREnabled)
	r.Put("/merchant/collection/settings", h.SaveCollection)
}

func (h *Handler) PublicConfig(w http.ResponseWriter, r *http.Request) {
	data, err := h.Service.GetPublic(r.Context())
	settingsResult(w, r, data, err, http.StatusOK)
}
func (h *Handler) Legal(w http.ResponseWriter, r *http.Request) {
	data, err := h.Service.GetLegal(r.Context(), chi.URLParam(r, "kind"))
	settingsResult(w, r, data, err, http.StatusOK)
}
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	row, err := h.Service.GetGroup(r.Context(), actor)
	if err != nil {
		settingsResult(w, r, nil, err, http.StatusOK)
		return
	}
	groups := row.Groups
	settingsResult(w, r, map[string]any{"version": row.Version, "site": groups["site"], "home": groups["home"], "footer_legal": groups["footer_legal"], "mail": groups["mail"], "security": groups["security"], "payment": groups["payment"], "billing": groups["billing"]}, nil, http.StatusOK)
}
func (h *Handler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	var req UpdateGroup
	if !settingsDecode(w, r, &req, 300<<10) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.UpdateGroup(r.Context(), actor, chi.URLParam(r, "group"), req)
	if actor.RecoveryOnly && err == nil {
		name := "vp_recovery"
		if h.SecureCookies {
			name = "__Host-" + name
		}
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, Secure: h.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
	settingsResult(w, r, result, err, http.StatusOK)
}
func (h *Handler) SaveCollection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SaveCollectionRequest
		OrderTTLSeconds json.RawMessage `json:"order_ttl_seconds"`
		FloatLimitCents json.RawMessage `json:"float_limit_cents"`
	}
	if !settingsDecode(w, r, &body, 1<<20) {
		return
	}
	req := body.SaveCollectionRequest
	fields := map[string]string{}
	if err := json.Unmarshal(body.OrderTTLSeconds, &req.OrderTTLSeconds); err != nil {
		fields["order_ttl_seconds"] = "invalid"
	}
	if err := json.Unmarshal(body.FloatLimitCents, &req.FloatLimitCents); err != nil {
		fields["float_limit_cents"] = "invalid"
	}
	if len(fields) != 0 {
		settingsResult(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400, Fields: fields}, http.StatusOK)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.SaveCollection(r.Context(), actor, req)
	settingsResult(w, r, result, err, http.StatusOK)
}
func (h *Handler) QRSlots(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	codes, err := h.Service.QRSlots(r.Context(), actor)
	settingsResult(w, r, map[string]any{"codes": codes}, err, http.StatusOK)
}
func (h *Handler) UploadQR(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes+(1<<16))
	file, _, err := r.FormFile("file")
	if err != nil {
		settingsResult(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}, http.StatusOK)
		return
	}
	defer func() { _ = file.Close() }() // Cleanup must not replace the operation result.
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.UploadQR(r.Context(), actor, chi.URLParam(r, "channel"), file)
	settingsResult(w, r, result, err, http.StatusCreated)
}
func (h *Handler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes+(1<<16))
	file, _, err := r.FormFile("file")
	if err != nil {
		settingsResult(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}, http.StatusOK)
		return
	}
	defer func() { _ = file.Close() }() // Cleanup must not replace the operation result.
	actor, _ := platform.ActorFromContext(r.Context())
	data, err := h.Service.UploadLogo(r.Context(), actor, file)
	settingsResult(w, r, data, err, http.StatusCreated)
}
func (h *Handler) QueueMailTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Recipient string `json:"recipient"`
	}
	if !settingsDecode(w, r, &req, 32<<10) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	data, err := h.Service.QueueMailTest(r.Context(), actor, req.Recipient)
	settingsResult(w, r, data, err, http.StatusAccepted)
}
func (h *Handler) MailTestStatus(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	data, err := h.Service.MailTestStatus(r.Context(), actor, chi.URLParam(r, "id"))
	settingsResult(w, r, data, err, http.StatusOK)
}
func (h *Handler) SetQREnabled(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !settingsDecode(w, r, &req, 1<<20) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	result, err := h.Service.SetQREnabled(r.Context(), actor, chi.URLParam(r, "channel"), req.Enabled)
	settingsResult(w, r, result, err, http.StatusOK)
}
func (h *Handler) OpenAsset(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	trade := r.URL.Query().Get("trade_no")
	expiry := r.URL.Query().Get("expires")
	signature := r.URL.Query().Get("signature")
	asset, err := h.Service.OpenAsset(r.Context(), chi.URLParam(r, "asset_id"), actor.MerchantID, trade, expiry, signature)
	if err != nil {
		settingsResult(w, r, nil, err, http.StatusOK)
		return
	}
	defer func() { _ = asset.Reader.Close() }() // Cleanup must not replace the operation result.
	w.Header().Set("Content-Type", asset.MIME)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, asset.Reader)
}

func settingsDecode(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		settingsResult(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}, http.StatusOK)
		return false
	}
	if dec.Decode(new(any)) != io.EOF {
		settingsResult(w, r, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400}, http.StatusOK)
		return false
	}
	return true
}
func settingsResult(w http.ResponseWriter, r *http.Request, data any, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = middleware.GetReqID(r.Context())
	}
	if err != nil {
		status = http.StatusInternalServerError
		var api *platform.Error
		if errors.As(err, &api) {
			if api.HTTPStatus != 0 {
				status = api.HTTPStatus
			}
			code := api.Code
			fields := api.Fields
			if fields == nil {
				fields = map[string]string{}
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": code, "fields": fields, "details": map[string]any{}}, "request_id": requestID})
			return
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "INTERNAL_ERROR", "message": "INTERNAL_ERROR", "fields": map[string]string{}, "details": map[string]any{}}, "request_id": requestID})
		return
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "error": nil, "request_id": requestID})
}
