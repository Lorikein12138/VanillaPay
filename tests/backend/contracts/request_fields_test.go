package contracts

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
)

func TestHTTP_RefundResolutionRequiresBooleanConfirm(t *testing.T) {
	fixture, _ := securityFixture(t)
	db := fixture.DB
	var merchantID int64
	if err := db.Pool.QueryRow(t.Context(), `SELECT id FROM merchants`).Scan(&merchantID); err != nil {
		t.Fatal(err)
	}
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	for _, test := range []struct {
		action, confirm, orderState, refundState, callbackState string
	}{
		{"reject", "false", "paid", "rejected", "pending"},
		{"mark_refunded", "true", "refunded", "completed", "cancelled"},
	} {
		t.Run(test.action, func(t *testing.T) {
			trade := "confirm-" + test.action
			var orderID int64
			err := db.Pool.QueryRow(t.Context(), `INSERT INTO orders(trade_no,merchant_id,out_trade_no,purpose,protocol,name,original_cents,
				allowed_channels_snapshot,notify_url,signing_key_version,signing_key_cipher,merchant_username_snapshot,merchant_pid_snapshot,expires_at,status)
				SELECT $2,id,$2,'external','v1','refund confirmation',100,ARRAY['alipay']::text[],'https://merchant.example/notify',
				key_version,key_cipher,username,pid,$3,'pending' FROM merchants WHERE id=$1 RETURNING id`,
				merchantID, trade, db.Clock.Now().Add(time.Hour)).Scan(&orderID)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.Pool.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if err := fixture.App.Services.Payment.MarkPaidTx(t.Context(), tx, orderID, payment.PaidCommand{Channel: "alipay"}); err != nil {
				_ = tx.Rollback(t.Context())
				t.Fatal(err)
			}
			if err := tx.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte(trade))
			refund, err := fixture.App.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{
				MerchantID: merchantID, TradeNo: trade, AmountCents: 100, RequestDigest: digest[:], Actor: actor,
			})
			if err != nil {
				t.Fatal(err)
			}
			headers := http.Header{"Content-Type": {"application/json"}, "Idempotency-Key": {"confirm-key-" + test.action}}
			send := func(t *testing.T, fields string) *httptest.ResponseRecorder {
				t.Helper()
				body := `{"action":"` + test.action + `","reason":"confirmation test"` + fields + `}`
				return fixture.RequestWithHeaders(t, http.MethodPost, "/api/v1/merchant/refunds/"+refund.ID+"/resolve",
					strings.NewReader(body), "merchant:matrix-merchant", headers)
			}
			type state struct {
				order, refund, callback       string
				orderRow, refundRow, eventRow string
				audits, idempotency           int
			}
			snapshot := func(t *testing.T) state {
				t.Helper()
				var current state
				err := db.Pool.QueryRow(t.Context(), `SELECT o.status,r.status,c.state,to_jsonb(o)::text,to_jsonb(r)::text,to_jsonb(c)::text,
					(SELECT count(*) FROM audit_logs WHERE merchant_id=o.merchant_id),
					(SELECT count(*) FROM idempotency_records WHERE route='merchant.refunds.resolve' AND key=$2)
					FROM orders o JOIN refunds r ON r.order_id=o.id JOIN callback_events c ON c.order_id=o.id
					WHERE r.public_id=$1`, refund.ID, headers.Get("Idempotency-Key")).
					Scan(&current.order, &current.refund, &current.callback, &current.orderRow, &current.refundRow, &current.eventRow,
						&current.audits, &current.idempotency)
				if err != nil {
					t.Fatal(err)
				}
				return current
			}
			before := snapshot(t)
			if before.order != "paid" || before.refund != "pending" || before.callback != "pending" || before.idempotency != 0 {
				t.Fatal("invalid refund fixture state")
			}
			invalid := []struct{ name, fields string }{
				{"missing", ""}, {"null", `,"confirm":null`}, {"string", `,"confirm":"false"`},
			}
			if test.action == "mark_refunded" {
				invalid = append(invalid, struct{ name, fields string }{"false", `,"confirm":false`})
			}
			for _, input := range invalid {
				t.Run(input.name, func(t *testing.T) {
					assertRequestFieldError(t, send(t, input.fields), "confirm")
					if snapshot(t) != before {
						t.Fatal("invalid confirm changed order, refund, callback, audit or idempotency state")
					}
				})
			}
			success := func() json.RawMessage {
				t.Helper()
				response := send(t, `,"confirm":`+test.confirm)
				var envelope struct{ Data json.RawMessage }
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusOK {
					t.Fatalf("explicit confirm=%s: status=%d, error=%v", test.confirm, response.Code, err)
				}
				return envelope.Data
			}
			first := success()
			after := snapshot(t)
			if after.order != test.orderState || after.refund != test.refundState || after.callback != test.callbackState ||
				after.audits != before.audits+1 || after.idempotency != 1 {
				t.Fatalf("resolution state order=%s refund=%s callback=%s audits=%d idempotency=%d",
					after.order, after.refund, after.callback, after.audits-before.audits, after.idempotency)
			}
			if replay := success(); string(replay) != string(first) || snapshot(t) != after {
				t.Fatal("replay changed refund result or persisted state")
			}
		})
	}
}

func TestHTTP_RequiredNullableNumbersMustBePresent(t *testing.T) {
	fixture, _ := securityFixture(t)
	t.Run("collection settings", func(t *testing.T) {
		const client = "merchant:matrix-merchant"
		const path = "/api/v1/merchant/collection/settings"
		body := map[string]any{"expected_version": 1, "float_limit_cents": nil, "float_direction": nil}
		assertRequestFieldError(t, fixture.Request(t, http.MethodPut, path, body, client), "order_ttl_seconds")
		delete(body, "float_limit_cents")
		body["order_ttl_seconds"] = nil
		assertRequestFieldError(t, fixture.Request(t, http.MethodPut, path, body, client), "float_limit_cents")
		body["float_limit_cents"] = nil
		if response := fixture.Request(t, http.MethodPut, path, body, client); response.Code != http.StatusOK {
			t.Fatalf("explicit null collection fields: status=%d", response.Code)
		}
	})
	t.Run("plan create and update", func(t *testing.T) {
		const client = "admin:matrix-admin"
		const path = "/api/v1/console/plans"
		body := map[string]any{"name": map[string]string{"zh_CN": "限购", "en": ""}, "price_cents": 100,
			"duration_days": 30, "channels": []string{"alipay"}}
		assertRequestFieldError(t, fixture.Request(t, http.MethodPost, path, body, client), "purchase_limit")
		body["purchase_limit"] = nil
		created := fixture.Request(t, http.MethodPost, path, body, client)
		var result struct {
			Data struct {
				ID       string
				Revision int64
			}
		}
		if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil || created.Code != http.StatusCreated || result.Data.ID == "" {
			t.Fatalf("explicit null purchase limit on create: status=%d error=%v", created.Code, err)
		}
		body["revision"] = result.Data.Revision
		delete(body, "purchase_limit")
		updatePath := path + "/" + result.Data.ID
		assertRequestFieldError(t, fixture.Request(t, http.MethodPut, updatePath, body, client), "purchase_limit")
		body["purchase_limit"] = nil
		if response := fixture.Request(t, http.MethodPut, updatePath, body, client); response.Code != http.StatusOK {
			t.Fatalf("explicit null purchase limit on update: status=%d", response.Code)
		}
	})
}

func assertRequestFieldError(t *testing.T, response *httptest.ResponseRecorder, field string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code   string
			Fields map[string]string
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusBadRequest ||
		envelope.Error.Code != "VALIDATION_ERROR" || envelope.Error.Fields[field] == "" {
		t.Fatalf("expected 400 VALIDATION_ERROR for %s, status=%d code=%s fields=%v error=%v",
			field, response.Code, envelope.Error.Code, envelope.Error.Fields, err)
	}
}
