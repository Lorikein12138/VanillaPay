package devices

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"vanillapay/internal/device"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_DeviceRemainingAcceptance(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	merchant := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	binding, err := fixture.App.Services.Device.NewBinding(t.Context(), merchant, false)
	if err != nil {
		t.Fatalf("create one-use device binding code: %v", err)
	}
	response := fixture.Request(t, http.MethodPost, "/api/v1/device/bind", map[string]any{
		"code": stringsLower(binding.Code), "device_name": "http acceptance device", "app_version": "5.0.0",
	}, "guest")
	var envelope struct {
		Data  device.CredentialDTO `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.Error != nil || envelope.Data.DeviceToken == "" || envelope.Data.DeviceID == "" {
		t.Fatalf("real HTTP device bind status=%d body=%s", response.Code, response.Body.String())
	}

	t.Run("V42.B.r01/v01", func(t *testing.T) {
		heartbeat := fixture.Request(t, http.MethodPost, "/api/v1/device/heartbeat", map[string]any{"app_version": "5.0.0"}, "device:"+envelope.Data.DeviceToken)
		if heartbeat.Code != http.StatusOK {
			t.Fatalf("device heartbeat status=%d body=%s", heartbeat.Code, heartbeat.Body.String())
		}
		receipt := map[string]any{
			"event_id": "123e4567-e89b-12d3-a456-426614174411", "channel": "alipay", "amount_cents": 100,
			"occurred_at": db.Clock.Now(), "provider_trade_no": "acceptance-receipt-unique",
		}
		first := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", receipt, "device:"+envelope.Data.DeviceToken)
		if first.Code != http.StatusCreated {
			t.Fatalf("first real device receipt status=%d body=%s", first.Code, first.Body.String())
		}
		second := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", receipt, "device:"+envelope.Data.DeviceToken)
		if second.Code != http.StatusOK {
			t.Fatalf("idempotent receipt replay status=%d body=%s", second.Code, second.Body.String())
		}
		var firstResult, secondResult struct {
			Data struct {
				ReceiptID  string `json:"receipt_id"`
				Duplicate  bool   `json:"duplicate"`
				MatchState string `json:"match_state"`
			} `json:"data"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(first.Body.Bytes(), &firstResult) != nil || json.Unmarshal(second.Body.Bytes(), &secondResult) != nil || firstResult.Error != nil || secondResult.Error != nil || firstResult.Data.ReceiptID == "" || secondResult.Data.ReceiptID != firstResult.Data.ReceiptID || secondResult.Data.MatchState != "unmatched" || !secondResult.Data.Duplicate {
			t.Fatalf("receipt replay DTO first=%s second=%s", first.Body.String(), second.Body.String())
		}
	})

	t.Run("V42.B.r03/v01", func(t *testing.T) {
		conflict := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", map[string]any{
			"event_id": "123e4567-e89b-12d3-a456-426614174411", "channel": "alipay", "amount_cents": 101,
			"occurred_at": db.Clock.Now(), "provider_trade_no": "acceptance-receipt-unique",
		}, "device:"+envelope.Data.DeviceToken)
		var result struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if conflict.Code != http.StatusConflict || json.Unmarshal(conflict.Body.Bytes(), &result) != nil || result.Error == nil || result.Error.Code != "EVENT_CONFLICT" {
			t.Fatalf("same event with different receipt content status=%d body=%s", conflict.Code, conflict.Body.String())
		}
	})

	t.Run("V40.B.r05/v01", func(t *testing.T) {
		if err := fixture.App.Services.Device.Revoke(t.Context(), merchant); err != nil {
			t.Fatalf("revoke bound device: %v", err)
		}
		response := fixture.Request(t, http.MethodPost, "/api/v1/device/heartbeat", map[string]any{"app_version": "5.0.0"}, "device:"+envelope.Data.DeviceToken)
		var result struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if response.Code != http.StatusUnauthorized || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Error == nil || result.Error.Code != "DEVICE_UNAUTHORIZED" {
			t.Fatalf("revoked/unknown device token status=%d body=%s", response.Code, response.Body.String())
		}
	})
	t.Run("V42.B.r05/v01", func(t *testing.T) {
		body := map[string]any{"event_id": "123e4567-e89b-12d3-a456-426614174412", "channel": "alipay", "amount_cents": 100, "occurred_at": db.Clock.Now(), "provider_trade_no": "invalid-token-event"}
		response := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", body, "device:not-a-valid-device-token")
		var result struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if response.Code != http.StatusUnauthorized || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Error == nil || result.Error.Code != "DEVICE_UNAUTHORIZED" {
			t.Fatalf("wrong device token status=%d body=%s", response.Code, response.Body.String())
		}
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM receipts WHERE event_id=$1::uuid`, body["event_id"]).Scan(&count); err != nil || count != 0 {
			t.Fatalf("wrong device token persisted %d receipt rows err=%v", count, err)
		}
	})
	t.Run("V42.B.r05/v02", func(t *testing.T) {
		body := map[string]any{"event_id": "123e4567-e89b-12d3-a456-426614174413", "channel": "alipay", "amount_cents": 100, "occurred_at": db.Clock.Now(), "provider_trade_no": "binding-code-is-not-token"}
		response := fixture.Request(t, http.MethodPost, "/api/v1/device/receipts", body, "device:"+binding.Code)
		var result struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if response.Code != http.StatusUnauthorized || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Error == nil || result.Error.Code != "DEVICE_UNAUTHORIZED" {
			t.Fatalf("QR/manual binding code used as Bearer status=%d body=%s", response.Code, response.Body.String())
		}
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM receipts WHERE event_id=$1::uuid`, body["event_id"]).Scan(&count); err != nil || count != 0 {
			t.Fatalf("binding code used as token persisted %d receipt rows err=%v", count, err)
		}
	})
}

func stringsLower(value string) string { return strings.ToLower(value) }
