package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"vanillapay/internal/platform"
)

type overviewOrder struct {
	TradeNo       string     `json:"trade_no"`
	Name          string     `json:"name"`
	Channel       *string    `json:"channel"`
	Status        string     `json:"status"`
	OriginalCents int64      `json:"original_cents"`
	ActualCents   *int64     `json:"actual_cents"`
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at"`
}

// Only registered by the loopback E2E executable, never the production router.
func registerOverviewFixtures(mux *http.ServeMux, pool *pgxpool.Pool) {
	mux.HandleFunc("POST /__e2e/orders", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PID    string          `json:"pid"`
			Orders []overviewOrder `json:"orders"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&input) != nil || len(input.Orders) == 0 || len(input.Orders) > 100 {
			http.Error(w, "invalid order fixtures", http.StatusBadRequest)
			return
		}
		if err := seedOverviewOrders(r.Context(), pool, input.PID, input.Orders); err != nil {
			http.Error(w, "order fixtures failed", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /__e2e/device", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PID   string `json:"pid"`
			State string `json:"state"`
			Name  string `json:"name"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil || (input.State != "unbound" && input.State != "online" && input.State != "offline") {
			http.Error(w, "invalid device fixture", http.StatusBadRequest)
			return
		}
		if err := seedOverviewDevice(r.Context(), pool, input.PID, input.State, input.Name); err != nil {
			http.Error(w, "device fixture failed", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func seedOverviewOrders(ctx context.Context, pool *pgxpool.Pool, pid string, orders []overviewOrder) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // Commit closes the transaction; rollback handles fixture errors.
	var merchantID int64
	var username string
	if err := tx.QueryRow(ctx, `SELECT id,username FROM merchants WHERE pid=$1`, pid).Scan(&merchantID, &username); err != nil {
		return err
	}
	for _, order := range orders {
		if order.CreatedAt.IsZero() || order.TradeNo == "" {
			return errors.New("missing fixture order identity or time")
		}
		var refundedAt *time.Time
		if order.Status == "refunded" {
			refundedAt = order.PaidAt
		}
		_, err := tx.Exec(ctx, `INSERT INTO orders(trade_no,out_trade_no,merchant_id,merchant_username_snapshot,purpose,protocol,name,channel,status,original_cents,actual_cents,created_at,paid_at,refunded_at,expires_at) VALUES($1,$1,$2,$3,'external','v1',$4,$5,$6,$7,$8,$9,$10,$11,$12)`, order.TradeNo, merchantID, username, order.Name, order.Channel, order.Status, order.OriginalCents, order.ActualCents, order.CreatedAt, order.PaidAt, refundedAt, order.CreatedAt.AddDate(0, 3, 0))
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func seedOverviewDevice(ctx context.Context, pool *pgxpool.Pool, pid, state, name string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // Roll back partial fixture replacement on error.
	var merchantID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM merchants WHERE pid=$1`, pid).Scan(&merchantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE devices SET revoked_at=now() WHERE merchant_id=$1 AND revoked_at IS NULL`, merchantID); err != nil {
		return err
	}
	if state != "unbound" {
		id, err := platform.RandomToken(rand.Reader, 32)
		if err != nil {
			return err
		}
		token, err := platform.RandomToken(rand.Reader, 32)
		if err != nil {
			return err
		}
		hash := sha256.Sum256([]byte(token))
		lastSeen := time.Now().UTC()
		if state == "offline" {
			lastSeen = lastSeen.Add(-10 * time.Minute)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO devices(public_id,merchant_id,token_hash,name,last_seen_at,ever_online) VALUES($1,$2,$3,$4,$5,true)`, id, merchantID, hash[:], name, lastSeen); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
