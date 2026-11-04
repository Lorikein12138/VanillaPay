package payments

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_CheckoutStateTransitions(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	planID := db.SeedPlan(t, "checkout entitlement", 1000, 3650, []string{"alipay", "wxpay"}, nil)
	db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	created, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
		MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "checkout-contract-001", Name: "checkout", OriginalCents: 250,
		NotifyURL: "https://merchant.example/notify", ReturnURL: "https://merchant.example/return", ClientIP: "198.51.100.33", SourceIP: "192.0.2.33",
		Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID},
	})
	if err != nil {
		t.Fatalf("create real pending order: %v", err)
	}
	t.Run("V60.B.api.get-checkout-trade-no/success", func(t *testing.T) {
		response := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+created.TradeNo, nil, "guest")
		if response.Code != http.StatusOK {
			t.Fatalf("checkout status=%d body=%s", response.Code, response.Body.String())
		}
		var envelope struct {
			Data struct {
				TradeNo           string   `json:"trade_no"`
				Status            string   `json:"status"`
				CanPay            *bool    `json:"can_pay"`
				Reason            *string  `json:"reason"`
				AvailableChannels []string `json:"available_channels"`
				ActualCents       *int64   `json:"actual_cents"`
				QRImageURL        *string  `json:"qr_image_url"`
			} `json:"data"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error != nil || envelope.Data.TradeNo != created.TradeNo || envelope.Data.Status != "pending" || envelope.Data.CanPay == nil || envelope.Data.Reason == nil || *envelope.Data.Reason != "no_channel" || len(envelope.Data.AvailableChannels) != 2 || envelope.Data.ActualCents != nil || envelope.Data.QRImageURL != nil {
			t.Fatalf("unselected checkout DTO=%#v err=%v body=%s", envelope, err, response.Body.String())
		}
	})
	t.Run("V32.B.r01/v01", func(t *testing.T) {
		response := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+created.TradeNo, nil, "guest")
		var envelope struct {
			Data struct {
				ActualCents *int64  `json:"actual_cents"`
				QRImageURL  *string `json:"qr_image_url"`
			} `json:"data"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.Data.ActualCents != nil || envelope.Data.QRImageURL != nil {
			t.Fatalf("unselected checkout exposed amount/QR: status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("V60.B.api.post-checkout-trade-no-channel/success", func(t *testing.T) {
		response := fixture.Request(t, http.MethodPost, "/api/v1/checkout/"+created.TradeNo+"/channel", map[string]any{"channel": "alipay"}, "guest")
		if response.Code != http.StatusOK {
			t.Fatalf("channel selection status=%d body=%s", response.Code, response.Body.String())
		}
		var envelope struct {
			Data struct {
				Channel     string  `json:"channel"`
				ActualCents *int64  `json:"actual_cents"`
				QRImageURL  *string `json:"qr_image_url"`
				CanPay      bool    `json:"can_pay"`
			} `json:"data"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error != nil || envelope.Data.Channel != "alipay" || envelope.Data.ActualCents == nil || envelope.Data.QRImageURL == nil || !envelope.Data.CanPay {
			t.Fatalf("selected checkout DTO=%#v err=%v body=%s", envelope, err, response.Body.String())
		}
		same := fixture.Request(t, http.MethodPost, "/api/v1/checkout/"+created.TradeNo+"/channel", map[string]any{"channel": "alipay"}, "guest")
		var sameEnvelope struct {
			Data struct {
				ActualCents *int64 `json:"actual_cents"`
			} `json:"data"`
		}
		if same.Code != http.StatusOK || json.Unmarshal(same.Body.Bytes(), &sameEnvelope) != nil || sameEnvelope.Data.ActualCents == nil || *sameEnvelope.Data.ActualCents != *envelope.Data.ActualCents {
			t.Fatalf("same channel selection changed locked amount: status=%d body=%s", same.Code, same.Body.String())
		}
		t.Run("V32.B.r02/v01", func(t *testing.T) {
			if sameEnvelope.Data.ActualCents == nil || *sameEnvelope.Data.ActualCents != *envelope.Data.ActualCents {
				t.Fatalf("same channel did not return original locked amount: original=%v replay=%v", envelope.Data.ActualCents, sameEnvelope.Data.ActualCents)
			}
		})
		locked := fixture.Request(t, http.MethodPost, "/api/v1/checkout/"+created.TradeNo+"/channel", map[string]any{"channel": "wxpay"}, "guest")
		var lockedEnvelope struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(locked.Body.Bytes(), &lockedEnvelope); err != nil {
			t.Fatalf("decode locked channel error: %v", err)
		}
		got := ""
		if lockedEnvelope.Error != nil {
			got = lockedEnvelope.Error.Code
		}
		if locked.Code != http.StatusConflict || got != "CHANNEL_LOCKED" {
			t.Fatalf("second channel selection status=%d code=%q body=%s", locked.Code, got, locked.Body.String())
		}
		t.Run("V32.B.r02/v02", func(t *testing.T) {
			if locked.Code != http.StatusConflict || got != "CHANNEL_LOCKED" {
				t.Fatalf("different channel selection status=%d code=%q", locked.Code, got)
			}
		})
		var channel string
		if err := db.Pool.QueryRow(t.Context(), `SELECT channel FROM orders WHERE trade_no=$1`, created.TradeNo).Scan(&channel); err != nil || channel != "alipay" {
			t.Fatalf("locked order channel=%q err=%v", channel, err)
		}
	})
	t.Run("V32.B.r01/v02", func(t *testing.T) {
		var actual *int64
		var asset *string
		if err := db.Pool.QueryRow(t.Context(), `SELECT actual_cents,qr_asset_id FROM orders WHERE trade_no=$1`, created.TradeNo).Scan(&actual, &asset); err != nil || actual == nil || asset == nil || *asset == "" {
			t.Fatalf("selected checkout snapshot actual=%v asset=%v err=%v", actual, asset, err)
		}
	})
	t.Run("V32.B.r03/v01", func(t *testing.T) {
		for _, scenario := range []struct {
			name string
			want string
		}{
			{name: "device_offline", want: "DEVICE_OFFLINE"},
			{name: "qr_disabled", want: "QR_UNAVAILABLE"},
			{name: "expired", want: "ORDER_NOT_PAYABLE"},
		} {
			t.Run(scenario.name, func(t *testing.T) {
				database := testsupport.NewDatabase(t)
				merchantID, merchantKey := database.SeedMerchant(t)
				httpFixture := testsupport.NewHTTPFixture(t, database)
				planID := database.SeedPlan(t, "checkout eligibility "+scenario.name, 1000, 3650, []string{"alipay"}, nil)
				database.PrepareBillingMerchant(t, httpFixture.App, merchantID, merchantKey, planID)
				order, err := httpFixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
					MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "checkout-eligibility-" + scenario.name,
					Name: "checkout eligibility", OriginalCents: 250, NotifyURL: "https://merchant.example/notify", SourceIP: "192.0.2.44",
					Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID},
				})
				if err != nil {
					t.Fatalf("create pending checkout order: %v", err)
				}
				switch scenario.name {
				case "device_offline":
					if _, err := database.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2`, database.Clock.Now().Add(-90*time.Second), merchantID); err != nil {
						t.Fatal("make device exactly offline")
					}
				case "qr_disabled":
					if _, err := database.Pool.Exec(t.Context(), `UPDATE media_assets SET active=false WHERE owner_merchant_id=$1 AND kind='qr'`, merchantID); err != nil {
						t.Fatal("disable merchant QR asset")
					}
				case "expired":
					database.Clock.Set(order.ExpiresAt)
				}
				_, err = httpFixture.App.Services.Payment.SelectChannel(t.Context(), order.TradeNo, "alipay")
				if errorCode(err) != scenario.want {
					t.Fatalf("select channel under %s error=%v, want %s", scenario.name, err, scenario.want)
				}
				var channel *string
				var actual *int64
				if err := database.Pool.QueryRow(t.Context(), `SELECT channel,actual_cents FROM orders WHERE trade_no=$1`, order.TradeNo).Scan(&channel, &actual); err != nil || channel != nil || actual != nil {
					t.Fatalf("rejected channel selection froze amount/channel: channel=%v actual=%v err=%v", channel, actual, err)
				}
			})
		}
	})
	t.Run("V32.B.r04/v01", func(t *testing.T) {
		for _, scenario := range []struct {
			name string
			want string
		}{
			{name: "subscription_inactive", want: "SUBSCRIPTION_INACTIVE"},
			{name: "qr_unavailable", want: "QR_UNAVAILABLE"},
			{name: "device_offline", want: "DEVICE_OFFLINE"},
			{name: "channel_not_allowed", want: "CHANNEL_NOT_ALLOWED"},
		} {
			t.Run(scenario.name, func(t *testing.T) {
				database := testsupport.NewDatabase(t)
				merchantID, merchantKey := database.SeedMerchant(t)
				httpFixture := testsupport.NewHTTPFixture(t, database)
				if scenario.name != "subscription_inactive" {
					planID := database.SeedPlan(t, "checkout create eligibility "+scenario.name, 1000, 3650, []string{"alipay"}, nil)
					database.PrepareBillingMerchant(t, httpFixture.App, merchantID, merchantKey, planID)
					switch scenario.name {
					case "qr_unavailable":
						if _, err := database.Pool.Exec(t.Context(), `UPDATE media_assets SET active=false WHERE owner_merchant_id=$1 AND kind='qr'`, merchantID); err != nil {
							t.Fatal("disable merchant QR asset")
						}
					case "device_offline":
						if _, err := database.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2`, database.Clock.Now().Add(-90*time.Second), merchantID); err != nil {
							t.Fatal("make device exactly offline")
						}
					}
				}
				channel := ""
				if scenario.name == "channel_not_allowed" {
					channel = "wxpay"
				}
				_, err := httpFixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
					MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "checkout-create-eligibility-" + scenario.name,
					Name: "checkout create eligibility", OriginalCents: 250, NotifyURL: "https://merchant.example/notify", SourceIP: "192.0.2.45", Channel: channel,
					Actor: platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID},
				})
				if errorCode(err) != scenario.want {
					t.Fatalf("create under %s error=%v, want %s", scenario.name, err, scenario.want)
				}
				var count int
				if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no=$2`, merchantID, "checkout-create-eligibility-"+scenario.name).Scan(&count); err != nil || count != 0 {
					t.Fatalf("rejected checkout create left %d order rows err=%v", count, err)
				}
			})
		}
	})
	t.Run("V32.B.r05/v01", func(t *testing.T) {
		db.Clock.Set(created.ExpiresAt.Add(1 * time.Second))
		response := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+created.TradeNo, nil, "guest")
		var envelope struct {
			Data struct {
				Status string `json:"status"`
				CanPay bool   `json:"can_pay"`
			} `json:"data"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.Data.Status != "expired" || envelope.Data.CanPay {
			t.Fatalf("expired checkout remained payable without expiry worker: status=%d body=%s", response.Code, response.Body.String())
		}
	})
}

func TestHTTP_DeviceOfflineOrderReconnectAndExpiryVectors(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	planID := db.SeedPlan(t, "offline reconnect checkout", 1000, 3650, []string{"alipay"}, nil)
	token := db.PrepareBillingMerchant(t, fixture.App, merchantID, merchantKey, planID)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	create := func(outTradeNo string) payment.OrderDTO {
		t.Helper()
		order, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: outTradeNo,
			Name: "offline reconnect", OriginalCents: 300, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.151", Actor: actor,
		})
		if err != nil {
			t.Fatalf("create order while device online: %v", err)
		}
		return order
	}
	order := create("offline-before-connect")
	if _, err := db.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2 AND revoked_at IS NULL`, db.Clock.Now().Add(-90*time.Second), merchantID); err != nil {
		t.Fatal("move device beyond 90-second online threshold")
	}
	t.Run("V45.B.r03/v01", func(t *testing.T) {
		_, err := fixture.App.Services.Payment.Create(t.Context(), payment.CreateCommand{
			MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "offline-new-order-denied",
			Name: "offline new order", OriginalCents: 301, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.152", Actor: actor,
		})
		if errorCode(err) != "DEVICE_OFFLINE" {
			t.Fatalf("new order while device offline error=%v, want DEVICE_OFFLINE", err)
		}
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM orders WHERE merchant_id=$1 AND out_trade_no='offline-new-order-denied'`, merchantID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("offline new order left %d rows err=%v", count, err)
		}
	})
	t.Run("V45.B.r03/v02", func(t *testing.T) {
		response := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+order.TradeNo, nil, "guest")
		var envelope struct {
			Data struct {
				Status       string  `json:"status"`
				DeviceOnline bool    `json:"device_online"`
				CanPay       bool    `json:"can_pay"`
				Reason       *string `json:"reason"`
			} `json:"data"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.Data.Status != "pending" || envelope.Data.DeviceOnline || envelope.Data.CanPay || envelope.Data.Reason == nil || *envelope.Data.Reason != "device_offline" {
			t.Fatalf("existing offline checkout response=%d body=%s", response.Code, response.Body.String())
		}
	})
	fixture.DeviceToken = token
	heartbeat := fixture.Request(t, http.MethodPost, "/api/v1/device/heartbeat", map[string]string{"app_version": "1.1"}, "device:"+token)
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("reconnect heartbeat status=%d body=%s", heartbeat.Code, heartbeat.Body.String())
	}
	t.Run("V45.B.r03/v03", func(t *testing.T) {
		selected := fixture.Request(t, http.MethodPost, "/api/v1/checkout/"+order.TradeNo+"/channel", map[string]string{"channel": "alipay"}, "guest")
		if selected.Code != http.StatusOK || !strings.Contains(selected.Body.String(), `"can_pay":true`) {
			t.Fatalf("unexpired pending order was not payable after heartbeat reconnect: status=%d body=%s", selected.Code, selected.Body.String())
		}
	})
	expired := create("offline-reconnect-expired")
	db.Clock.Set(expired.ExpiresAt)
	lateHeartbeat := fixture.Request(t, http.MethodPost, "/api/v1/device/heartbeat", map[string]string{"app_version": "1.2"}, "device:"+token)
	if lateHeartbeat.Code != http.StatusOK {
		t.Fatalf("late heartbeat status=%d body=%s", lateHeartbeat.Code, lateHeartbeat.Body.String())
	}
	t.Run("V45.B.r04/v01", func(t *testing.T) {
		response := fixture.Request(t, http.MethodGet, "/api/v1/checkout/"+expired.TradeNo, nil, "guest")
		var envelope struct {
			Data struct {
				Status string `json:"status"`
				CanPay bool   `json:"can_pay"`
			} `json:"data"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || envelope.Data.Status != "expired" || envelope.Data.CanPay {
			t.Fatalf("expired checkout revived by heartbeat status=%d body=%s", response.Code, response.Body.String())
		}
		if _, err := fixture.App.Services.Payment.SelectChannel(t.Context(), expired.TradeNo, "alipay"); errorCode(err) != "ORDER_NOT_PAYABLE" {
			t.Fatalf("expired order channel selection after reconnect error=%v, want ORDER_NOT_PAYABLE", err)
		}
	})
}
