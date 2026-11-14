package settings

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct{ DB DBTX }

type SettingsRow struct {
	Version int64
	Groups  map[string]json.RawMessage
}

func (r Repository) Get(ctx context.Context) (SettingsRow, error) {
	var row SettingsRow
	var site, home, footer, mail, security, payment, risk, billing []byte
	err := r.DB.QueryRow(ctx, `SELECT version,site,home,footer_legal,mail,security - 'allowed_hostname',payment,to_jsonb(s)->'risk',billing
		FROM site_settings s WHERE singleton=true`).Scan(&row.Version, &site, &home, &footer, &mail, &security, &payment, &risk, &billing)
	if err != nil {
		return row, err
	}
	if len(risk) == 0 {
		risk = []byte(`{"ip_daily_order_limit":0,"ip_blacklist":[],"name_block_words":[],"checkout_turnstile_enabled":false}`)
	}
	row.Groups = map[string]json.RawMessage{"site": site, "home": home, "footer_legal": footer, "mail": mail, "security": security, "payment": payment, "risk": risk, "billing": billing}
	return row, nil
}
