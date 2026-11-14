package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_RegistrationReviewGatesMerchantOperations(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO legal_versions(kind,revision,content) VALUES('terms',1,'{"zh_CN":"terms","en":"terms"}'),('privacy',1,'{"zh_CN":"privacy","en":"privacy"}')`); err != nil {
		t.Fatalf("seed current legal revisions: %v", err)
	}
	bootstrapCode, err := fixture.App.Services.Auth.RotateBootstrapCode(ctx)
	if err != nil {
		t.Fatal("issue admin bootstrap code")
	}
	adminRole := "admin:registration-review"
	assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", map[string]any{
		"initialization_code": bootstrapCode, "username": "review_admin", "password": fixturePassword, "password_confirmation": fixturePassword,
	}, adminRole), http.StatusCreated, "bootstrap review admin")
	if err := configureTOTPTestSiteverify(t, fixture, ctx); err != nil {
		t.Fatalf("configure local Siteverify: %v", err)
	}

	register := func(username, email string) json.RawMessage {
		t.Helper()
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/email-codes", map[string]any{
			"purpose": "register", "email": email, "target_username": username, "turnstile_token": "ok:register",
		}, "guest"), http.StatusOK, "registration email code for "+username)
		return assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/register", registrationBody(username, email, fixture.Mailer.LastCode()), "guest"), http.StatusCreated, "register "+username)
	}
	login := func(username, password string) *http.Response {
		t.Helper()
		recorder := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"identifier": username, "password": password, "turnstile_token": "ok:login"}, "guest")
		return recorder.Result()
	}
	pendingReview := func(data json.RawMessage) bool {
		t.Helper()
		var envelope struct {
			PendingReview *bool `json:"pending_review"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil || envelope.PendingReview == nil {
			t.Fatalf("registration response lacks pending_review: %s err=%v", data, err)
		}
		return *envelope.PendingReview
	}
	merchantID := func(username string) int64 {
		t.Helper()
		var id int64
		if err := db.Pool.QueryRow(ctx, `SELECT id FROM merchants WHERE username=$1`, username).Scan(&id); err != nil {
			t.Fatalf("read merchant %s: %v", username, err)
		}
		return id
	}
	queuedMail := func(template string) int {
		t.Helper()
		var count int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM mail_deliveries WHERE template=$1`, template).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	t.Run("review off keeps registration immediately usable", func(t *testing.T) {
		if pendingReview(register("review_off_user", "review-off@example.test")) {
			t.Fatal("registration reported pending review while the switch is off")
		}
		if response := login("review_off_user", fixtureChangedPassword); response.StatusCode != http.StatusOK {
			t.Fatalf("login after unreviewed registration status=%d", response.StatusCode)
		}
	})

	if _, err := db.Pool.Exec(ctx, `UPDATE site_settings SET security=security || '{"registration_review":true}'::jsonb WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	var public struct {
		Data struct {
			RegistrationReview bool `json:"registration_review"`
		} `json:"data"`
	}
	publicResponse := fixture.Request(t, http.MethodGet, "/api/v1/public/config", nil, "guest")
	if err := json.Unmarshal(publicResponse.Body.Bytes(), &public); err != nil || !public.Data.RegistrationReview {
		t.Fatalf("public config does not expose registration_review: %s", publicResponse.Body.String())
	}

	const username = "pending_review_user"
	const merchantSession = "merchant:review-subject"
	if !pendingReview(register(username, "pending-review@example.test")) {
		t.Fatal("registration did not report pending review while the switch is on")
	}
	id := merchantID(username)
	idPath := "/api/v1/console/merchants/" + strconv.FormatInt(id, 10)

	t.Run("pending merchant can sign in but cannot operate", func(t *testing.T) {
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"identifier": username, "password": fixturePassword, "turnstile_token": "ok:login"}, "guest"), "INVALID_CREDENTIALS", "wrong password does not reveal review state")
		data := assertIdentityResponse(t, fixture.Request(t, http.MethodPost, "/api/v1/auth/login", map[string]any{"identifier": username, "password": fixtureChangedPassword, "turnstile_token": "ok:login"}, merchantSession), http.StatusOK, "pending merchant login")
		if !strings.Contains(string(data), `"next_path":"/registration-review"`) || !strings.Contains(string(data), `"approval_state":"pending"`) {
			t.Fatalf("pending merchant did not receive the review destination and state")
		}
		assertIdentityError(t, fixture.Request(t, http.MethodGet, "/api/v1/merchant/orders", nil, merchantSession), "ACCOUNT_PENDING_REVIEW", "pending merchant cannot read orders")
		assertIdentityError(t, fixture.Request(t, http.MethodPost, "/api/v1/merchant/test-payment", map[string]any{"amount_cents": 100, "channel": nil}, merchantSession), "ACCOUNT_PENDING_REVIEW", "pending merchant cannot create a test payment")
		_, err := fixture.App.Services.Payment.Create(ctx, payment.CreateCommand{MerchantID: id, Protocol: "v1", Purpose: "external", OutTradeNo: "pending-review-order", Name: "pending", OriginalCents: 100, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.60", Actor: platform.Actor{Role: "merchant", SubjectID: id, MerchantID: id}})
		var apiErr *platform.Error
		if !errors.As(err, &apiErr) || apiErr.Code != "ACCOUNT_BANNED" {
			t.Fatalf("pending merchant order creation err=%v", err)
		}
	})

	t.Run("console lists and filters pending merchants", func(t *testing.T) {
		data := assertIdentityResponse(t, fixture.Request(t, http.MethodGet, "/api/v1/console/merchants?approval_state=pending&limit=20", nil, adminRole), http.StatusOK, "pending merchant list")
		var list struct {
			Items []struct {
				Username      string `json:"username"`
				ApprovalState string `json:"approval_state"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &list); err != nil || len(list.Items) != 1 || list.Items[0].Username != username || list.Items[0].ApprovalState != "pending" {
			t.Fatalf("pending list = %s err=%v", data, err)
		}
		assertIdentityError(t, fixture.Request(t, http.MethodGet, "/api/v1/console/merchants?approval_state=unknown", nil, adminRole), "VALIDATION_ERROR", "unknown approval filter")
		detail := assertIdentityResponse(t, fixture.Request(t, http.MethodGet, idPath, nil, adminRole), http.StatusOK, "pending merchant detail")
		if !strings.Contains(string(detail), `"approval":{"approval_reason":null,"approval_reviewed_at":null,"approval_state":"pending"}`) {
			t.Fatalf("detail approval = %s", detail)
		}
	})

	t.Run("reject requires a reason and reports it to the signed-in merchant", func(t *testing.T) {
		assertIdentityError(t, fixture.Request(t, http.MethodPost, idPath+"/reject", map[string]any{"reason": "   "}, adminRole), "VALIDATION_ERROR", "blank rejection reason")
		assertIdentityError(t, fixture.Request(t, http.MethodPost, idPath+"/reject", map[string]any{"reason": strings.Repeat("理", 501)}, adminRole), "VALIDATION_ERROR", "long rejection reason")
		data := assertIdentityResponse(t, fixture.Request(t, http.MethodPost, idPath+"/reject", map[string]any{"reason": "  资料不完整  "}, adminRole), http.StatusOK, "reject pending merchant")
		var approval struct {
			State  string  `json:"approval_state"`
			Reason *string `json:"approval_reason"`
		}
		if err := json.Unmarshal(data, &approval); err != nil || approval.State != "rejected" || approval.Reason == nil || *approval.Reason != "资料不完整" {
			t.Fatalf("reject response = %s err=%v", data, err)
		}
		if queuedMail("registration_rejected") != 1 {
			t.Fatal("rejection mail was not queued exactly once")
		}
		me := assertIdentityResponse(t, fixture.Request(t, http.MethodGet, "/api/v1/merchant/me", nil, merchantSession), http.StatusOK, "rejected merchant session")
		if !strings.Contains(string(me), `"approval_state":"rejected"`) || !strings.Contains(string(me), `"approval_reason":"资料不完整"`) {
			t.Fatal("existing session did not expose the rejection and reason")
		}
		assertIdentityError(t, fixture.Request(t, http.MethodGet, "/api/v1/merchant/orders", nil, merchantSession), "REGISTRATION_REJECTED", "rejected merchant cannot read orders")
		assertIdentityError(t, fixture.Request(t, http.MethodPost, idPath+"/reject", map[string]any{"reason": "again"}, adminRole), "VERSION_CONFLICT", "reject a non-pending merchant")
		var audits int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='merchant.reject' AND merchant_id=$1 AND NOT safe_metadata ? 'reason'`, id).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("reject audit count=%d err=%v", audits, err)
		}
	})

	t.Run("approve unlocks the existing session and is idempotent", func(t *testing.T) {
		data := assertIdentityResponse(t, fixture.Request(t, http.MethodPost, idPath+"/approve", nil, adminRole), http.StatusOK, "approve rejected merchant")
		if !strings.Contains(string(data), `"approval_state":"approved"`) || !strings.Contains(string(data), `"approval_reason":null`) {
			t.Fatalf("approve response = %s", data)
		}
		if response := login(username, fixtureChangedPassword); response.StatusCode != http.StatusOK {
			t.Fatalf("login after approval status=%d", response.StatusCode)
		}
		assertIdentityResponse(t, fixture.Request(t, http.MethodPost, idPath+"/approve", nil, adminRole), http.StatusOK, "approve again")
		assertIdentityResponse(t, fixture.Request(t, http.MethodGet, "/api/v1/merchant/orders", nil, merchantSession), http.StatusOK, "approved merchant can read orders using the same session")
		notices := assertIdentityResponse(t, fixture.Request(t, http.MethodGet, "/api/v1/merchant/notifications", nil, merchantSession), http.StatusOK, "registration notices")
		for _, kind := range []string{"registration_approved", "registration_rejected"} {
			if strings.Count(string(notices), `"kind":"`+kind+`"`) != 1 {
				t.Fatalf("expected one %s notification", kind)
			}
		}
		if queuedMail("registration_approved") != 1 {
			t.Fatal("approval mail was not queued exactly once")
		}
		assertIdentityError(t, fixture.Request(t, http.MethodPost, idPath+"/reject", map[string]any{"reason": "late"}, adminRole), "VERSION_CONFLICT", "reject an approved merchant")
	})
}
