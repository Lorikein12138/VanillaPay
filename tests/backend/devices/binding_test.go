package devices

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/device"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_BindingExpiresAtFiveMinutesAndCannotBeConsumedAfterExpiry(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	binding, err := application.Services.Device.NewBinding(context.Background(), actor, false)
	if err != nil || !binding.ExpiresAt.Equal(db.Clock.Now().Add(5*time.Minute)) {
		t.Fatalf("binding = %#v, %v", binding, err)
	}
	db.Clock.Set(binding.ExpiresAt)
	if _, err := application.Services.Device.Redeem(context.Background(), device.RedeemRequest{Code: binding.Code, DeviceName: "late device", AppVersion: "1.0"}); !hasCode(err, "BINDING_EXPIRED") {
		t.Fatalf("expired binding error = %v", err)
	}
	t.Run("V40.B.r02/v01", func(t *testing.T) {
		if _, err := application.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: binding.Code, DeviceName: "late replay", AppVersion: "1.0"}); !hasCode(err, "BINDING_EXPIRED") {
			t.Fatal("binding remained redeemable at the five-minute expiry boundary")
		}
	})
	t.Run("V40.B.r02/v02", func(t *testing.T) {
		db2 := testsupport.NewDatabase(t)
		merchantID2, _ := db2.SeedMerchant(t)
		app2 := db2.NewApplication(t)
		binding2, err := app2.Services.Device.NewBinding(t.Context(), platform.Actor{Role: "merchant", SubjectID: merchantID2, MerchantID: merchantID2}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := app2.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: binding2.Code, DeviceName: "first use", AppVersion: "1"}); err != nil {
			t.Fatalf("redeem new binding first time: %v", err)
		}
		if _, err := app2.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: binding2.Code, DeviceName: "replay", AppVersion: "1"}); !hasCode(err, "BINDING_USED") {
			t.Fatalf("consumed binding replay error=%v, want BINDING_USED", err)
		}
	})
}

func TestDB_ManualCodeAndQRCodeUseSameBindingSession(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	binding, err := app.Services.Device.NewBinding(t.Context(), actor, false)
	if err != nil {
		t.Fatalf("create one binding session: %v", err)
	}
	parsed, err := url.Parse(binding.QRContent)
	if err != nil {
		t.Fatalf("parse QR payload: %v", err)
	}
	qrCode := parsed.Query().Get("code")
	t.Run("V40.B.r01/v01", func(t *testing.T) {
		if parsed.Scheme != "vanillapay" || parsed.Host != "bind" || parsed.Query().Get("v") != "1" || qrCode == "" || qrCode != binding.Code {
			t.Fatalf("manual code and QR payload do not resolve to one binding session: manual=%q QR=%q payload=%q", binding.Code, qrCode, binding.QRContent)
		}
		credential, err := app.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: qrCode, DeviceName: "qr redeemed device", AppVersion: "1"})
		if err != nil || credential.DeviceID == "" {
			t.Fatalf("redeem QR session code: credential=%#v err=%v", credential, err)
		}
		status, err := app.Services.Device.Binding(t.Context(), actor, binding.BindingID)
		if err != nil || status.State != "consumed" || status.Device == nil || status.Device.ID == nil || *status.Device.ID != credential.DeviceID {
			t.Fatalf("QR redemption did not consume the same manual binding session: status=%#v err=%v", status, err)
		}
	})
}
func TestDB_ConcurrentBindingRedemptionConsumesOneSession(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	binding, err := app.Services.Device.NewBinding(t.Context(), actor, false)
	if err != nil {
		t.Fatalf("create binding for concurrent redemption: %v", err)
	}
	barrier := testsupport.NewBarrier(2)
	type result struct {
		credential device.CredentialDTO
		err        error
	}
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for index := 0; index < 2; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			if err := barrier.Wait(t.Context()); err != nil {
				results <- result{err: err}
				return
			}
			credential, err := app.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: binding.Code, DeviceName: "parallel redeem", AppVersion: "1"})
			results <- result{credential: credential, err: err}
		}(index)
	}
	workers.Wait()
	close(results)
	var winners, usedErrors int
	for outcome := range results {
		if outcome.err == nil && outcome.credential.DeviceToken != "" {
			winners++
		} else if hasCode(outcome.err, "BINDING_USED") {
			usedErrors++
		} else {
			t.Fatalf("concurrent redemption returned unexpected outcome: %#v", outcome)
		}
	}
	t.Run("V40.B.r03/v01", func(t *testing.T) {
		var active int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL`, merchantID).Scan(&active); err != nil || winners != 1 || usedErrors != 1 || active != 1 {
			t.Fatalf("parallel bind winners=%d consumed-errors=%d active-devices=%d err=%v, want 1/1/1", winners, usedErrors, active, err)
		}
		var audits int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE merchant_id=$1 AND action='device.bind'`, merchantID).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("parallel bind created %d bind audits err=%v, want one", audits, err)
		}
	})
}

func TestDB_DeviceReplacementRequiresConfirmationAndRevokesOldToken(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	initialBinding, err := app.Services.Device.NewBinding(t.Context(), actor, false)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := app.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: initialBinding.Code, DeviceName: "initial", AppVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("V40.B.r04/v01", func(t *testing.T) {
		if _, err := app.Services.Device.NewBinding(t.Context(), actor, false); !hasCode(err, "DEVICE_ALREADY_BOUND") {
			t.Fatalf("replacement without confirmation error=%v, want DEVICE_ALREADY_BOUND", err)
		}
		if _, err := app.Services.Device.Authenticate(t.Context(), initial.DeviceToken); err != nil {
			t.Fatalf("unconfirmed replacement invalidated existing device: %v", err)
		}
	})
	confirmed, err := app.Services.Device.NewBinding(t.Context(), actor, true)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := app.Services.Device.Redeem(t.Context(), device.RedeemRequest{Code: confirmed.Code, DeviceName: "replacement", AppVersion: "2"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("V40.B.r04/v02", func(t *testing.T) {
		if _, err := app.Services.Device.Authenticate(t.Context(), initial.DeviceToken); !hasCode(err, "DEVICE_UNAUTHORIZED") {
			t.Fatalf("old device token remains valid after confirmed replacement: %v", err)
		}
		if _, err := app.Services.Device.Authenticate(t.Context(), replacement.DeviceToken); err != nil {
			t.Fatalf("replacement device token is not active: %v", err)
		}
		var active int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL`, merchantID).Scan(&active); err != nil || active != 1 {
			t.Fatalf("confirmed replacement active devices=%d err=%v, want one", active, err)
		}
	})
}

func TestDB_DeviceBindingLifecycleAuditUsesRequestMetadataAndRedactsSecrets(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	ctx := platform.WithRequestMetadata(context.Background(), platform.RequestMetadata{IP: "::ffff:192.0.2.31", RequestID: "device-binding-audit"})
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID, IP: "198.51.100.99", RequestID: "actor-must-not-win"}
	binding, err := application.Services.Device.NewBinding(ctx, actor, false)
	if err != nil {
		t.Fatalf("issue device binding: %v", err)
	}
	credential, err := application.Services.Device.Redeem(ctx, device.RedeemRequest{Code: binding.Code, DeviceName: "audit fixture", AppVersion: "5.0.0"})
	if err != nil {
		t.Fatalf("redeem device binding: %v", err)
	}
	if err := application.Services.Device.Revoke(ctx, actor); err != nil {
		t.Fatalf("revoke bound device: %v", err)
	}

	rows, err := db.Pool.Query(ctx, `SELECT actor_role,COALESCE(actor_id::text,''),COALESCE(merchant_id::text,''),action,target_id,result,COALESCE(ip::text,''),request_id,safe_metadata FROM audit_logs WHERE request_id=$1 AND action=ANY($2::text[]) ORDER BY id`, "device-binding-audit", []string{"device.binding_issue", "device.bind", "device.revoke"})
	if err != nil {
		t.Fatalf("read device lifecycle audit rows: %v", err)
	}
	defer rows.Close()
	var seen []string
	for rows.Next() {
		var role, actorID, auditMerchant, action, targetID, result, ip, requestID string
		var raw json.RawMessage
		if err := rows.Scan(&role, &actorID, &auditMerchant, &action, &targetID, &result, &ip, &requestID, &raw); err != nil {
			t.Fatalf("scan device lifecycle audit row: %v", err)
		}
		merchantIDText := strconv.FormatInt(merchantID, 10)
		if result != "success" || auditMerchant != merchantIDText || ip != "192.0.2.31/32" || requestID != "device-binding-audit" || targetID == "" {
			t.Fatalf("device lifecycle audit metadata = role=%q actor=%q merchant=%q action=%q target=%q result=%q ip=%q request=%q", role, actorID, auditMerchant, action, targetID, result, ip, requestID)
		}
		if action == "device.bind" && (role != "device" || actorID == "" || targetID != credential.DeviceID) {
			t.Fatalf("bind audit must use the trusted device identity: role=%q actor=%q target=%q", role, actorID, targetID)
		}
		if action != "device.bind" && (role != "merchant" || actorID != merchantIDText) {
			t.Fatalf("merchant device management audit actor = %q/%q for %s", role, actorID, action)
		}
		var metadata map[string]any
		if err := json.Unmarshal(raw, &metadata); err != nil {
			t.Fatalf("decode audit metadata: %v", err)
		}
		encoded := string(raw)
		for _, secret := range []string{binding.Code, credential.DeviceToken} {
			if strings.Contains(encoded, secret) {
				t.Fatalf("device audit metadata exposed binding or bearer secret for %s", action)
			}
		}
		seen = append(seen, action)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate device lifecycle audits: %v", err)
	}
	if strings.Join(seen, ",") != "device.binding_issue,device.bind,device.revoke" {
		t.Fatalf("device lifecycle audit actions = %v", seen)
	}
	t.Run("V16.B.r01/v09", func(t *testing.T) {
		var count int
		var exposed bool
		if err := db.Pool.QueryRow(ctx, `SELECT count(*),COALESCE(bool_or(safe_metadata::text LIKE $2 OR safe_metadata::text LIKE $3),false) FROM audit_logs WHERE request_id=$1 AND action=ANY($4::text[])`, "device-binding-audit", "%"+binding.Code+"%", "%"+credential.DeviceToken+"%", []string{"device.binding_issue", "device.bind", "device.revoke"}).Scan(&count, &exposed); err != nil {
			t.Fatal("verify device lifecycle audit count and redaction")
		}
		if count != 3 || exposed {
			t.Fatalf("device lifecycle audit count=%d exposed_sensitive_value=%t", count, exposed)
		}
	})
}

func hasCode(err error, code string) bool {
	apiErr := &platform.Error{}
	ok := errors.As(err, &apiErr)
	return ok && apiErr.Code == code
}
