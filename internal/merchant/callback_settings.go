package merchant

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/platform"
)

// CallbackSettings controls merchant-visible fields in asynchronous
// notifications and signed return redirects.
type CallbackSettings struct {
	HideName bool `json:"hide_name"`
}

func (s *Service) CallbackSettings(ctx context.Context, actor platform.Actor) (CallbackSettings, error) {
	var settings CallbackSettings
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return settings, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	err := s.pool.QueryRow(ctx, `SELECT callback_hide_name FROM merchants WHERE id=$1`, actor.MerchantID).Scan(&settings.HideName)
	if errors.Is(err, pgx.ErrNoRows) {
		return settings, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	return settings, err
}

func (s *Service) SetCallbackSettings(ctx context.Context, actor platform.Actor, settings CallbackSettings) (CallbackSettings, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return CallbackSettings{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	id := actor.MerchantID
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{id}, false); err != nil {
			return err
		}
		var previous bool
		if err := tx.QueryRow(ctx, `SELECT callback_hide_name FROM merchants WHERE id=$1`, id).Scan(&previous); err != nil {
			return err
		}
		if previous == settings.HideName {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE merchants SET callback_hide_name=$1,updated_at=$2 WHERE id=$3`, settings.HideName, s.clock.Now(), id); err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, &id, "merchant.update_callback_settings", "merchant", strconv.FormatInt(id, 10), map[string]any{"hide_name": settings.HideName})
	})
	return settings, err
}

func (h *Handler) GetCallbackSettings(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	settings, err := h.Service.CallbackSettings(r.Context(), actor)
	merchantWrite(w, r, settings, err, 200)
}

func (h *Handler) PutCallbackSettings(w http.ResponseWriter, r *http.Request) {
	var settings CallbackSettings
	if !merchantDecode(w, r, &settings, 4<<10) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	saved, err := h.Service.SetCallbackSettings(r.Context(), actor, settings)
	merchantWrite(w, r, saved, err, 200)
}
