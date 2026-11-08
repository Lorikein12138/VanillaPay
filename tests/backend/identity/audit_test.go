package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_AuditSecurityCoverage(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	ctx := context.Background()
	bootstrapCode, err := fixture.App.Services.Auth.RotateBootstrapCode(ctx)
	if err != nil {
		t.Fatal("prepare private admin bootstrap code")
	}
	if _, err := fixture.App.Services.Auth.Bootstrap(ctx, auth.BootstrapRequest{
		InitializationCode:   bootstrapCode,
		Username:             "audit_admin",
		Password:             fixturePassword,
		PasswordConfirmation: fixturePassword,
	}); err != nil {
		t.Fatal("create isolated admin for real login audit requests")
	}
	var version int64
	var current json.RawMessage
	if err := db.Pool.QueryRow(ctx, `SELECT version,security FROM site_settings WHERE singleton=true`).Scan(&version, &current); err != nil {
		t.Fatal("read security settings fixture")
	}
	var security map[string]json.RawMessage
	if err := json.Unmarshal(current, &security); err != nil {
		t.Fatal("decode security settings fixture")
	}
	settingsValue, err := json.Marshal(map[string]any{
		"turnstile_site_key":     "audit-local-site-key",
		"turnstile_secret":       "audit-local-secret",
		"allowed_hostname":       "pay.example.test",
		"restrict_email_domains": false,
		"email_domains":          []string{},
		"ban_contact":            map[string]string{"zh_CN": "support@example.test", "en": "support@example.test"},
	})
	if err != nil {
		t.Fatal("encode isolated Siteverify settings")
	}
	if _, err := fixture.App.Services.Settings.UpdateGroup(ctx, platform.Actor{Role: "admin", SubjectID: 1}, "security", settings.UpdateGroup{ExpectedVersion: version, Value: settingsValue}); err != nil {
		t.Fatalf("configure local Siteverify service: %v", err)
	}

	password := fixturePassword
	var successRequestID, knownFailureRequestID, unknownFailureRequestID, unavailableRequestID string
	t.Run("login_admin_success", func(t *testing.T) {
		success := postAuditLogin(t, fixture, "/api/v1/console/login", "192.0.2.10:41001", map[string]any{
			"identifier": "audit_admin", "password": password, "turnstile_token": "ok:login",
		})
		if success.errorCode() != "" || success.RequestID == "" {
			t.Fatalf("successful admin login envelope = %#v", success)
		}
		successRequestID = success.RequestID
		assertAuditRequest(t, db, success.RequestID, auditExpectation{
			role: "admin", actorID: "1", action: "auth.login", result: "success", ip: "192.0.2.10",
		})
	})

	t.Run("login_known_password_failure", func(t *testing.T) {
		knownFailure := postAuditLogin(t, fixture, "/api/v1/console/login", "192.0.2.11:41002", map[string]any{
			"identifier": "audit_admin", "password": fixtureWrongPassword, "turnstile_token": "ok:login",
		})
		if knownFailure.errorCode() != "INVALID_CREDENTIALS" || knownFailure.RequestID == "" {
			t.Fatalf("known-account failure envelope = %#v", knownFailure)
		}
		knownFailureRequestID = knownFailure.RequestID
		assertAuditRequest(t, db, knownFailure.RequestID, auditExpectation{
			role: "admin", actorID: "1", action: "auth.login", result: "failure", ip: "192.0.2.11", errorCode: "INVALID_CREDENTIALS",
		})
	})

	t.Run("login_unknown_account_failure", func(t *testing.T) {
		unknownFailure := postAuditLogin(t, fixture, "/api/v1/console/login", "192.0.2.12:41003", map[string]any{
			"identifier": "unknown-audit-account@example.test", "password": fixtureWrongPassword, "turnstile_token": "ok:login",
		})
		if unknownFailure.errorCode() != "INVALID_CREDENTIALS" || unknownFailure.RequestID == "" {
			t.Fatalf("unknown-account failure envelope = %#v", unknownFailure)
		}
		unknownFailureRequestID = unknownFailure.RequestID
		assertAuditRequest(t, db, unknownFailure.RequestID, auditExpectation{
			role: "admin", actorID: "", action: "auth.login", result: "failure", ip: "192.0.2.12", errorCode: "INVALID_CREDENTIALS", targetID: "unknown",
		})
	})

	t.Run("login_siteverify_unavailable_failure", func(t *testing.T) {
		fixture.Siteverify.SetMode("unavailable")
		unavailable := postAuditLogin(t, fixture, "/api/v1/console/login", "192.0.2.13:41004", map[string]any{
			"identifier": "audit_admin", "password": password, "turnstile_token": "ok:login",
		})
		if unavailable.errorCode() != "TURNSTILE_UNAVAILABLE" || unavailable.RequestID == "" {
			t.Fatalf("unavailable Siteverify envelope = %#v", unavailable)
		}
		unavailableRequestID = unavailable.RequestID
		assertAuditRequest(t, db, unavailable.RequestID, auditExpectation{
			role: "admin", actorID: "", action: "auth.login", result: "failure", ip: "192.0.2.13", errorCode: "TURNSTILE_UNAVAILABLE", targetID: "unknown",
		})
	})

	t.Run("login_cancelled_context_failure_persists", func(t *testing.T) {
		cancelledMetadata := platform.WithRequestMetadata(context.Background(), platform.RequestMetadata{IP: "192.0.2.14", RequestID: "cancelled-login-audit"})
		cancelledContext, cancel := context.WithCancel(cancelledMetadata)
		cancel()
		_, cancelledErr := fixture.App.Services.Auth.Login(cancelledContext, "admin", auth.LoginRequest{
			Identifier: "audit_admin", Password: password, TurnstileToken: "ok:login",
		})
		if cancelledErr == nil {
			t.Fatal("cancelled login context unexpectedly succeeded")
		}
		assertAuditRequest(t, db, "cancelled-login-audit", auditExpectation{
			role: "admin", actorID: "", action: "auth.login", result: "failure", ip: "192.0.2.14", errorCode: "INTERNAL_ERROR", targetID: "unknown",
		})
	})

	t.Run("V16.B.r01/v01", func(t *testing.T) {
		var successes, failures int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE result='success'),count(*) FILTER (WHERE result='failure') FROM audit_logs WHERE request_id=ANY($1::text[]) AND action='auth.login'`, []string{successRequestID, knownFailureRequestID, unknownFailureRequestID, unavailableRequestID}).Scan(&successes, &failures); err != nil {
			t.Fatalf("count real login success/failure audit rows: %v", err)
		}
		if successes != 1 || failures != 3 {
			t.Fatalf("login audit success/failure counts=%d/%d, want 1/3", successes, failures)
		}
	})
	t.Run("V16.B.r01/v03", func(t *testing.T) {
		assertAuditRequest(t, db, successRequestID, auditExpectation{
			role: "admin", actorID: "1", action: "auth.login", result: "success", ip: "192.0.2.10",
		})
	})
	t.Run("V16.B.r01/v04", func(t *testing.T) {
		assertAuditRequest(t, db, knownFailureRequestID, auditExpectation{
			role: "admin", actorID: "1", action: "auth.login", result: "failure", ip: "192.0.2.11", errorCode: "INVALID_CREDENTIALS",
		})
	})
	t.Run("V16.B.r01/v05", func(t *testing.T) {
		assertAuditRequest(t, db, unknownFailureRequestID, auditExpectation{
			role: "admin", actorID: "", action: "auth.login", result: "failure", ip: "192.0.2.12", errorCode: "INVALID_CREDENTIALS", targetID: "unknown",
		})
	})
	t.Run("V16.B.r01/v06", func(t *testing.T) {
		assertAuditRequest(t, db, unavailableRequestID, auditExpectation{
			role: "admin", actorID: "", action: "auth.login", result: "failure", ip: "192.0.2.13", errorCode: "TURNSTILE_UNAVAILABLE", targetID: "unknown",
		})
	})
	t.Run("V16.B.r01/v07", func(t *testing.T) {
		assertAuditRequest(t, db, "cancelled-login-audit", auditExpectation{
			role: "admin", actorID: "", action: "auth.login", result: "failure", ip: "192.0.2.14", errorCode: "INTERNAL_ERROR", targetID: "unknown",
		})
	})

	merchantOne, _ := db.SeedMerchant(t)
	merchantTwo, _ := db.SeedMerchant(t)
	for index, merchantID := range []int64{merchantOne, merchantTwo} {
		metadata := platform.WithRequestMetadata(ctx, platform.RequestMetadata{IP: "192.0.2." + []string{"21", "22"}[index], RequestID: "audit-merchant-" + []string{"one", "two"}[index]})
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		if _, err := fixture.App.Services.Settings.SaveCollection(metadata, actor, settings.SaveCollectionRequest{ExpectedVersion: 1}); err != nil {
			t.Fatalf("create real merchant collection audit for merchant %d: %v", merchantID, err)
		}
	}

	t.Run("V16.B.r01/v02", func(t *testing.T) {
		var rows int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE merchant_id=ANY($1::bigint[]) AND action='collection.save' AND result='success'`, []int64{merchantOne, merchantTwo}).Scan(&rows); err != nil {
			t.Fatalf("count real merchant settings audit actions: %v", err)
		}
		if rows != 2 {
			t.Fatalf("collection.save audit rows=%d, want one for each merchant", rows)
		}
	})

	t.Run("V16.B.r02/v01", func(t *testing.T) {
		page, err := fixture.App.Services.Portal.ListAudits(ctx, platform.Actor{Role: "merchant", SubjectID: merchantOne, MerchantID: merchantOne}, portal.AuditFilter{MerchantID: &merchantTwo, Action: "collection.save", Result: "success", Limit: 50})
		if err != nil {
			t.Fatalf("list merchant audit rows: %v", err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].MerchantID == nil || *page.Items[0].MerchantID != fmt.Sprint(merchantOne) {
			t.Fatalf("merchant audit filter escaped its own merchant: total=%d items=%#v", page.Total, page.Items)
		}
	})

	t.Run("V16.B.r02/v02", func(t *testing.T) {
		page, err := fixture.App.Services.Portal.ListAudits(ctx, platform.Actor{Role: "admin", SubjectID: 1}, portal.AuditFilter{MerchantID: &merchantTwo, Action: "collection.save", Result: "success", ActorRole: "merchant", Limit: 50})
		if err != nil {
			t.Fatalf("list platform audit rows with admin filters: %v", err)
		}
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].MerchantID == nil || *page.Items[0].MerchantID != fmt.Sprint(merchantTwo) || page.Items[0].Action != "collection.save" || page.Items[0].Result != "success" || page.Items[0].ActorRole != "merchant" {
			t.Fatalf("admin audit filters were not applied: total=%d items=%#v", page.Total, page.Items)
		}
	})

	t.Run("V16.B.r03/v01", func(t *testing.T) {
		var metadata []byte
		if err := db.Pool.QueryRow(ctx, `SELECT safe_metadata FROM audit_logs WHERE request_id=$1 AND action='auth.login' AND result='failure'`, knownFailureRequestID).Scan(&metadata); err != nil {
			t.Fatalf("read login failure safe metadata: %v", err)
		}
		for _, secret := range []string{password, fixtureWrongPassword, "audit-local-secret", "ok:login", "audit_admin"} {
			if strings.Contains(string(metadata), secret) {
				t.Fatalf("login failure safe metadata contains sensitive value %q", secret)
			}
		}
	})

	t.Run("V16.B.r03/v02", func(t *testing.T) {
		for _, requestID := range []string{knownFailureRequestID, unknownFailureRequestID, unavailableRequestID, "cancelled-login-audit"} {
			var count int
			if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE request_id=$1 AND result='failure' AND action='auth.login'`, requestID).Scan(&count); err != nil {
				t.Fatalf("check failure audit request_id %q: %v", requestID, err)
			}
			if requestID == "" || count != 1 {
				t.Fatalf("failure audit request_id %q count=%d, want one correlated row", requestID, count)
			}
		}
	})
}

type auditLoginEnvelope struct {
	RequestID string `json:"request_id"`
	Error     *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func (e auditLoginEnvelope) errorCode() string {
	if e.Error == nil {
		return ""
	}
	return strings.TrimSpace(e.Error.Code)
}

func postAuditLogin(t *testing.T, fixture *testsupport.HTTPFixture, path, remote string, body map[string]any) auditLoginEnvelope {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal("encode real authentication request")
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	response := fixture.RequestWithRemoteAddr(t, http.MethodPost, path, strings.NewReader(string(encoded)), "guest", headers, remote)
	var envelope auditLoginEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode login audit response (HTTP %d): %v: %s", response.Code, err, response.Body.String())
	}
	if envelope.Error != nil {
		envelope.Error.Code = strings.TrimSpace(envelope.Error.Code)
	}
	wantStatus := http.StatusOK
	switch envelope.errorCode() {
	case "INVALID_CREDENTIALS":
		wantStatus = http.StatusUnauthorized
	case "TURNSTILE_UNAVAILABLE":
		wantStatus = http.StatusServiceUnavailable
	}
	if response.Code != wantStatus {
		t.Fatalf("login route status=%d want=%d body=%s", response.Code, wantStatus, response.Body.String())
	}
	return envelope
}

type auditExpectation struct {
	role      string
	actorID   string
	action    string
	result    string
	ip        string
	errorCode string
	targetID  string
}

func assertAuditRequest(t *testing.T, db *testsupport.Database, requestID string, want auditExpectation) {
	t.Helper()
	var role, actorID, action, targetID, result, ip, gotRequestID string
	var metadata json.RawMessage
	err := db.Pool.QueryRow(context.Background(), `
		SELECT actor_role,COALESCE(actor_id::text,''),action,target_id,result,COALESCE(ip::text,''),request_id,safe_metadata
		FROM audit_logs WHERE request_id=$1 AND action=$2`, requestID, want.action).
		Scan(&role, &actorID, &action, &targetID, &result, &ip, &gotRequestID, &metadata)
	if err != nil {
		t.Fatalf("read production audit row for request %q: %v", requestID, err)
	}
	if role != want.role || actorID != want.actorID || action != want.action || result != want.result || gotRequestID != requestID {
		t.Fatalf("audit identity for %q = role=%q actor=%q action=%q result=%q request=%q", requestID, role, actorID, action, result, gotRequestID)
	}
	gotIP, err := netip.ParsePrefix(ip)
	if err != nil || gotIP.Addr().Unmap().String() != want.ip || gotIP.Bits() != gotIP.Addr().BitLen() {
		t.Fatalf("audit IP for %q = %q, want %q (parse error %v)", requestID, ip, want.ip, err)
	}
	if want.targetID != "" && targetID != want.targetID {
		t.Fatalf("audit target for %q = %q, want %q", requestID, targetID, want.targetID)
	}
	var safe map[string]any
	if err := json.Unmarshal(metadata, &safe); err != nil {
		t.Fatalf("decode audit safe_metadata for %q: %v", requestID, err)
	}
	if want.errorCode != "" && safe["error_code"] != want.errorCode {
		t.Fatalf("failure audit error_code = %#v, want %q", safe["error_code"], want.errorCode)
	}
	for _, forbidden := range []string{fixtureWrongPassword, fixtureWrongPassword, "unknown-audit-account@example.test", fixturePassword, "audit-local-secret"} {
		if strings.Contains(string(metadata), forbidden) {
			t.Fatalf("audit metadata for %q contains sensitive input %q", requestID, forbidden)
		}
	}
}
