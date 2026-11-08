package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"vanillapay/internal/platform"
)

// Only this development loopback executable registers these fixture endpoints.
func registerSubscriptionFixtures(mux *http.ServeMux, pool *pgxpool.Pool, key [32]byte) {
	var billingOnce sync.Once
	var billingErr error
	var ids []string
	mux.HandleFunc("POST /__e2e/subscription", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PID    string `json:"pid"`
			Active bool   `json:"active"`
		}
		if platform.DecodeJSONBody(w, r, 2048, &input) != nil {
			http.Error(w, "invalid subscription fixture", http.StatusBadRequest)
			return
		}
		billingOnce.Do(func() {
			billingErr = seedBilling(r.Context(), pool, key)
			if billingErr != nil {
				return
			}
			for _, name := range [][2]string{{"基础套餐", "Basic"}, {"进阶套餐", "Plus"}} {
				var id int64
				billingErr = pool.QueryRow(r.Context(), `INSERT INTO plans(name_zh,name_en,price_cents,duration_days,channels,purchase_limit) VALUES($1,$2,1200,30,ARRAY['alipay','wxpay'],3) RETURNING id`, name[0], name[1]).Scan(&id)
				if billingErr != nil {
					return
				}
				ids = append(ids, strconv.FormatInt(id, 10))
			}
		})
		if billingErr != nil {
			http.Error(w, "billing fixture failed", http.StatusInternalServerError)
			return
		}
		var merchantID int64
		if pool.QueryRow(r.Context(), `SELECT id FROM merchants WHERE pid=$1`, input.PID).Scan(&merchantID) != nil {
			http.Error(w, "merchant fixture not found", http.StatusBadRequest)
			return
		}
		if _, err := pool.Exec(r.Context(), `UPDATE devices SET last_seen_at=now() WHERE merchant_id=(SELECT id FROM merchants WHERE pid=(SELECT billing->>'merchant_pid' FROM site_settings WHERE singleton=true)) AND revoked_at IS NULL`); err != nil {
			http.Error(w, "billing heartbeat fixture failed", http.StatusInternalServerError)
			return
		}
		if input.Active {
			if _, err := pool.Exec(r.Context(), `INSERT INTO subscriptions(merchant_id,plan_id,snapshot,expires_at,source) SELECT $1,id,jsonb_build_object('id',id::text,'revision',revision,'name',jsonb_build_object('zh_CN',name_zh,'en',name_en),'channels',channels,'price_cents',price_cents,'duration_days',duration_days,'purchase_limit',purchase_limit,'deleted',false),now()+interval '30 days','manual' FROM plans WHERE id=$2`, merchantID, ids[0]); err != nil {
				http.Error(w, "active subscription fixture failed", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"plan_ids": ids})
	})
	mux.HandleFunc("POST /__e2e/notifications", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PID        string `json:"pid"`
			TradeNo    string `json:"trade_no"`
			UnsafeText bool   `json:"unsafe_text"`
		}
		if platform.DecodeJSONBody(w, r, 2048, &input) != nil {
			http.Error(w, "invalid notification fixture", http.StatusBadRequest)
			return
		}
		for _, page := range []string{"/dashboard/orders", "/dashboard/collection"} {
			id, err := platform.RandomToken(rand.Reader, 18)
			if err != nil {
				http.Error(w, "notification identity failed", http.StatusInternalServerError)
				return
			}
			kind := "device_offline"
			title := map[string]string{"zh_CN": "监控设备已离线", "en": "Monitoring device offline"}
			summary := map[string]string{"zh_CN": "监控设备已超过离线阈值，请检查设备网络和应用运行状态。", "en": "The device has exceeded its offline threshold. Check its network and application."}
			if page == "/dashboard/orders" {
				kind = "refund_request"
				title = map[string]string{"zh_CN": "退款申请待处理", "en": "Refund request pending"}
				summary = map[string]string{"zh_CN": "订单 " + input.TradeNo + " 收到退款申请，请查看订单详情并及时处理。", "en": "Order " + input.TradeNo + " has a refund request. Review the order and process it promptly."}
			}
			if input.UnsafeText {
				summary = map[string]string{"zh_CN": "<b>仅按纯文本显示</b>", "en": "<b>Plain text only</b>"}
			}
			payload, err := json.Marshal(map[string]any{"title": title, "summary": summary, "action": map[string]string{"page": page, "entity_id": input.TradeNo}})
			if err != nil {
				http.Error(w, "notification payload failed", http.StatusInternalServerError)
				return
			}
			if _, err := pool.Exec(r.Context(), `INSERT INTO notifications(public_id,merchant_id,kind,source_id,dedupe_key,safe_payload) SELECT $1,id,$4,$1,$1,$2 FROM merchants WHERE pid=$3`, id, payload, input.PID, kind); err != nil {
				http.Error(w, "notification fixture failed", http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /__e2e/announcements", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PID        string `json:"pid"`
			UnsafeText bool   `json:"unsafe_text"`
		}
		if platform.DecodeJSONBody(w, r, 2048, &input) != nil {
			http.Error(w, "invalid announcement fixture", http.StatusBadRequest)
			return
		}
		if err := seedAnnouncement(r.Context(), pool, input.PID, input.UnsafeText); err != nil {
			http.Error(w, "announcement fixture failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func seedBilling(ctx context.Context, pool *pgxpool.Pool, key [32]byte) error {
	merchant, err := seedMerchant(ctx, pool, key, false, false, "")
	if err != nil {
		return err
	}
	var merchantID int64
	var cipher []byte
	if err := pool.QueryRow(ctx, `SELECT id,key_cipher FROM merchants WHERE pid=$1`, merchant["pid"]).Scan(&merchantID, &cipher); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subscriptions(merchant_id,snapshot,expires_at,source) VALUES($1,'{"channels":["alipay","wxpay"]}',now()+interval '10 years','manual')`, merchantID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `UPDATE site_settings SET billing=jsonb_set(billing,'{merchant_pid}',to_jsonb($1::text)) WHERE singleton=true`, merchant["pid"]); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `INSERT INTO secret_settings(name,encrypted_value) VALUES('billing_key',$1) ON CONFLICT(name) DO UPDATE SET encrypted_value=excluded.encrypted_value`, cipher); err != nil {
		return err
	}
	for _, channel := range []string{"alipay", "wxpay"} {
		id, err := platform.RandomToken(rand.Reader, 18)
		if err != nil {
			return err
		}
		payload, err := platform.Encrypt(key[:], []byte("https://qr.alipay.com/fixture"), rand.Reader)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `INSERT INTO media_assets(id,owner_merchant_id,kind,channel,file_path,sha256,mime,qr_payload_cipher,active) VALUES($1,$2,'qr',$3,$4,$5,'image/png',$6,true)`, id, merchantID, channel, "assets/"+id+".png", make([]byte, 32), payload); err != nil {
			return err
		}
	}
	return seedOverviewDevice(ctx, pool, merchant["pid"], "online", "Billing device")
}

func seedAnnouncement(ctx context.Context, pool *pgxpool.Pool, pid string, unsafeText bool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(710061)`); err != nil {
		return err
	}
	id, err := platform.RandomToken(rand.Reader, 18)
	if err != nil {
		return err
	}
	title, err := json.Marshal(map[string]string{"zh_CN": "平台收款服务使用说明", "en": "Using the platform payment service"})
	if err != nil {
		return err
	}
	text := map[string]string{"zh_CN": "为保证收款顺利，请保持监控设备在线，并定期检查收款码状态。\n如遇订单未匹配，可在收款记录中查看原因并联系平台支持。", "en": "Keep your monitoring device online and check your collection codes regularly to ensure smooth payments.\nIf a receipt is unmatched, review its reason in the receipts list and contact platform support."}
	if unsafeText {
		text = map[string]string{"zh_CN": "<script>不执行</script>\n<b>原始文本</b>", "en": "<script>Never execute</script>\n<b>Raw text</b>"}
	}
	body, err := json.Marshal(text)
	if err != nil {
		return err
	}
	var internal int64
	if err := tx.QueryRow(ctx, `INSERT INTO announcements(public_id,title,body,popup,published_at) VALUES($1,$2,$3,true,$4) RETURNING id`, id, title, body, time.Now()).Scan(&internal); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO announcement_reads(merchant_id,announcement_id,revision) SELECT id,$1,1 FROM merchants WHERE pid<>$2`, internal, pid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
