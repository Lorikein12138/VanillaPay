package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"vanillapay/internal/platform"
)

type checkoutCode struct {
	URL string `json:"url"`
	PNG []byte `json:"png"`
}

// Only main.go's development + loopback test executable registers this helper.
// It prepares eligibility, not payment: orders use real signed v1 requests and
// payments use real device receipts. No checkout/payment route is mocked.
func registerCheckoutFixtures(mux *http.ServeMux, pool *pgxpool.Pool, key [32]byte, dataDir string) {
	mux.HandleFunc("POST /__e2e/checkout", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Codes   map[string]checkoutCode `json:"codes,omitempty"`
			TradeNo string                  `json:"trade_no,omitempty"`
			State   string                  `json:"state,omitempty"`
		}
		if platform.DecodeJSONBody(w, r, 131072, &input) != nil {
			http.Error(w, "invalid checkout fixture", http.StatusBadRequest)
			return
		}
		if input.TradeNo != "" {
			var err error
			switch input.State {
			case "expired":
				_, err = pool.Exec(r.Context(), `UPDATE orders SET expires_at=now()-interval '1 second' WHERE trade_no=$1`, input.TradeNo)
			case "offline":
				_, err = pool.Exec(r.Context(), `UPDATE devices SET last_seen_at=now()-interval '10 minutes' WHERE merchant_id=(SELECT merchant_id FROM orders WHERE trade_no=$1)`, input.TradeNo)
			default:
				http.Error(w, "invalid checkout state", http.StatusBadRequest)
				return
			}
			if err != nil {
				http.Error(w, "checkout state failed", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		fixture, err := seedCheckout(r.Context(), pool, key, dataDir, input.Codes)
		if err != nil {
			http.Error(w, "checkout fixture failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(fixture)
	})
}

func seedCheckout(ctx context.Context, pool *pgxpool.Pool, key [32]byte, dataDir string, codes map[string]checkoutCode) (map[string]string, error) {
	merchant, err := seedMerchant(ctx, pool, key, false, false, "")
	if err != nil {
		return nil, err
	}
	var merchantID int64
	var cipher []byte
	if err := pool.QueryRow(ctx, `SELECT id,key_cipher FROM merchants WHERE pid=$1`, merchant["pid"]).Scan(&merchantID, &cipher); err != nil {
		return nil, err
	}
	plain, err := platform.Decrypt(key[:], cipher)
	if err != nil {
		return nil, err
	}
	deviceToken, err := platform.RandomToken(rand.Reader, 32)
	if err != nil {
		return nil, err
	}
	id, err := platform.RandomToken(rand.Reader, 18)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "assets"), 0700); err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Distinct normalized names keep fixture identities unique; only the display
	// snapshot is localized for screenshots.
	if _, err := tx.Exec(ctx, `UPDATE merchants SET username='示例小店' WHERE id=$1`, merchantID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO subscriptions(merchant_id,snapshot,expires_at,source) VALUES($1,'{"channels":["alipay","wxpay"]}',now()+interval '1 year','manual')`, merchantID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_collection_settings(merchant_id,float_limit_cents,float_direction) VALUES($1,10,'down')`, merchantID); err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(deviceToken))
	if _, err := tx.Exec(ctx, `INSERT INTO devices(public_id,merchant_id,token_hash,name,last_seen_at,ever_online) VALUES($1,$2,$3,'示例收款设备',now(),true)`, id, merchantID, hash[:]); err != nil {
		return nil, err
	}
	for _, channel := range []string{"alipay", "wxpay"} {
		code := codes[channel]
		if config, err := png.DecodeConfig(bytes.NewReader(code.PNG)); err != nil || config.Width != 300 || config.Height != 300 || code.URL == "" {
			return nil, errors.New("invalid checkout fixture code for " + channel)
		}
		payload, err := platform.Encrypt(key[:], []byte(code.URL), rand.Reader)
		if err != nil {
			return nil, err
		}
		assetID := id
		if channel != "alipay" {
			assetID += "-" + channel
		}
		relative := "assets/" + assetID + ".png"
		if err := os.WriteFile(filepath.Join(dataDir, filepath.FromSlash(relative)), code.PNG, 0600); err != nil {
			return nil, err
		}
		checksum := sha256.Sum256(code.PNG)
		if _, err := tx.Exec(ctx, `INSERT INTO media_assets(id,owner_merchant_id,kind,channel,file_path,sha256,mime,qr_payload_cipher,active) VALUES($1,$2,'qr',$3,$4,$5,'image/png',$6,true)`, assetID, merchantID, channel, relative, checksum[:], payload); err != nil {
			return nil, err
		}
	}
	// Occupy the original amount so the real allocator must choose 29.99.
	var occupied int64
	if err := tx.QueryRow(ctx, `INSERT INTO orders(trade_no,out_trade_no,merchant_id,merchant_username_snapshot,purpose,protocol,name,channel,status,original_cents,actual_cents,amount_offset_cents,channel_locked_at,qr_asset_id,allowed_channels_snapshot,expires_at) VALUES($1,$1,$2,'示例小店','external','v1','已有订单','alipay','pending',3000,3000,0,now(),$3,ARRAY['alipay'],now()+interval '1 hour') RETURNING id`, "occupied-"+id, merchantID, id).Scan(&occupied); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO amount_reservations(merchant_id,channel,actual_cents,order_id) VALUES($1,'alipay',3000,$2)`, merchantID, occupied); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]string{"pid": merchant["pid"], "username": merchant["username"], "password": merchant["password"], "v1_key": string(plain), "device_token": deviceToken, "created_at": time.Now().UTC().Format(time.RFC3339Nano)}, nil
}
