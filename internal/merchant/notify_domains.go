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

type NotifyDomains struct {
	Enabled bool     `json:"enabled"`
	Entries []string `json:"entries"`
}

func (s *Service) NotifyDomains(ctx context.Context, actor platform.Actor) (NotifyDomains, error) {
	rules := NotifyDomains{Entries: []string{}}
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return rules, &platform.Error{Code: "UNAUTHENTICATED", HTTPStatus: 401}
	}
	err := s.pool.QueryRow(ctx, `SELECT enabled,entries FROM merchant_notify_domains WHERE merchant_id=$1`, actor.MerchantID).Scan(&rules.Enabled, &rules.Entries)
	if errors.Is(err, pgx.ErrNoRows) {
		return rules, nil
	}
	return rules, err
}

func (s *Service) SetNotifyDomains(ctx context.Context, actor platform.Actor, rules NotifyDomains) (NotifyDomains, error) {
	if actor.Role != "merchant" || actor.MerchantID <= 0 {
		return NotifyDomains{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	if len(rules.Entries) > 20 || rules.Enabled && len(rules.Entries) == 0 {
		return NotifyDomains{}, platform.InvalidJSONField("entries")
	}
	entries := make([]string, 0, len(rules.Entries))
	seen := make(map[string]bool)
	for _, value := range rules.Entries {
		host, err := platform.NormalizeNotifyDomain(value)
		if err != nil {
			return NotifyDomains{}, platform.InvalidJSONField("entries")
		}
		if !seen[host] {
			seen[host] = true
			entries = append(entries, host)
		}
	}
	rules.Entries = entries
	id := actor.MerchantID
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{id}, false); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO merchant_notify_domains(merchant_id,enabled,entries) VALUES($1,$2,$3)
			ON CONFLICT(merchant_id) DO UPDATE SET enabled=EXCLUDED.enabled,entries=EXCLUDED.entries,
			version=merchant_notify_domains.version+1,updated_at=$4`, id, rules.Enabled, entries, s.clock.Now()); err != nil {
			return err
		}
		return recordAudit(ctx, tx, actor, &id, "merchant.update_notify_domains", "merchant", strconv.FormatInt(id, 10), map[string]any{"enabled": rules.Enabled, "entry_count": len(entries)})
	})
	return rules, err
}

func (h *Handler) GetNotifyDomains(w http.ResponseWriter, r *http.Request) {
	actor, _ := platform.ActorFromContext(r.Context())
	rules, err := h.Service.NotifyDomains(r.Context(), actor)
	merchantWrite(w, r, rules, err, 200)
}

func (h *Handler) PutNotifyDomains(w http.ResponseWriter, r *http.Request) {
	var rules NotifyDomains
	if !merchantDecode(w, r, &rules, 32<<10) {
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	saved, err := h.Service.SetNotifyDomains(r.Context(), actor, rules)
	merchantWrite(w, r, saved, err, 200)
}
