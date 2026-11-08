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
	TradeNo            string     `json:"trade_no"`
	OutTradeNo         string     `json:"out_trade_no"`
	Name               string     `json:"name"`
	Channel            *string    `json:"channel"`
	Status             string     `json:"status"`
	OriginalCents      int64      `json:"original_cents"`
	ActualCents        *int64     `json:"actual_cents"`
	CreatedAt          time.Time  `json:"created_at"`
	PaidAt             *time.Time `json:"paid_at"`
	NotifyURL          *string    `json:"notify_url"`
	ReturnURL          *string    `json:"return_url"`
	CallbackState      string     `json:"callback_state"`
	CallbackGeneration int        `json:"callback_generation"`
	AllowedChannels    []string   `json:"allowed_channels_snapshot"`
	RefundPending      bool       `json:"refund_pending"`
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
		outTrade := order.OutTradeNo
		if outTrade == "" {
			outTrade = order.TradeNo
		}
		var orderID int64
		channels := order.AllowedChannels
		if channels == nil {
			channels = []string{"alipay", "wxpay", "qqpay"}
		}
		err := tx.QueryRow(ctx, `INSERT INTO orders(trade_no,out_trade_no,merchant_id,merchant_username_snapshot,purpose,protocol,name,channel,status,original_cents,actual_cents,created_at,paid_at,refunded_at,expires_at,notify_url,return_url,allowed_channels_snapshot) VALUES($1,$2,$3,$4,'external','v1',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`, order.TradeNo, outTrade, merchantID, username, order.Name, order.Channel, order.Status, order.OriginalCents, order.ActualCents, order.CreatedAt, order.PaidAt, refundedAt, order.CreatedAt.AddDate(0, 3, 0), order.NotifyURL, order.ReturnURL, channels).Scan(&orderID)
		if err != nil {
			return err
		}
		if order.Status == "pending" && order.Channel != nil && order.ActualCents != nil {
			if _, err := tx.Exec(ctx, `UPDATE orders SET channel_locked_at=created_at WHERE id=$1`, orderID); err != nil {
				return err
			}
		}
		if order.CallbackState != "" {
			var eventID int64
			err := tx.QueryRow(ctx, `INSERT INTO callback_events(public_id,order_id,generation,state,attempt_count,payload_snapshot) VALUES($1,$2,$3,$4,1,jsonb_build_object('trade_no',$5::text)) RETURNING id`, order.TradeNo+"-callback", orderID, max(1, order.CallbackGeneration), order.CallbackState, order.TradeNo).Scan(&eventID)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO callback_attempts(public_id,event_id,attempt_no,started_at,finished_at,http_status,duration_ms,response_excerpt,error_code,url_redacted) VALUES($1,$2,1,$3,$3,500,12,$4,$5,$6)`, order.TradeNo+"-attempt", eventID, order.CreatedAt, "fixture callback failed", "CALLBACK_NOT_CONFIRMED", order.NotifyURL); err != nil {
				return err
			}
		}
		if order.RefundPending {
			digest := sha256.Sum256([]byte(order.TradeNo))
			if _, err := tx.Exec(ctx, `INSERT INTO refunds(public_id,order_id,requested_cents,status,reason,request_digest) VALUES($1,$2,$3,'pending',$4,$5)`, order.TradeNo+"-refund", orderID, order.OriginalCents, "Fixture refund request", digest[:]); err != nil {
				return err
			}
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
