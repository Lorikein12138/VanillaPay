package subscriptions

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"vanillapay/internal/platform"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_ManualGrantStartsANewImmutableEntitlementChain(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "manual", 1000, 30, []string{"alipay", "wxpay"}, nil)
	application := db.NewApplication(t)
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	firstExpiry := db.Clock.Now().Add(30 * 24 * time.Hour)
	first, err := application.Services.Subscription.Grant(context.Background(), admin, merchantID, subscription.GrantRequest{PlanID: itoa(planID), ExpiresAt: firstExpiry, Channels: []string{"alipay"}, ExpectedRevision: 0, Reason: "initial grant"})
	if err != nil || first.Source != "manual" || first.Revision != 1 {
		t.Fatalf("first manual grant = %#v, %v", first, err)
	}
	secondExpiry := db.Clock.Now().Add(60 * 24 * time.Hour)
	second, err := application.Services.Subscription.Grant(context.Background(), admin, merchantID, subscription.GrantRequest{PlanID: itoa(planID), ExpiresAt: secondExpiry, Channels: []string{"wxpay"}, ExpectedRevision: first.Revision, Reason: "manual override"})
	if err != nil || second.Revision != 2 || second.ExpiresAt == nil || !second.ExpiresAt.Equal(secondExpiry) {
		t.Fatalf("second manual grant = %#v, %v", second, err)
	}
	var active, voided int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE voided_at IS NULL),count(*) FILTER (WHERE voided_at IS NOT NULL) FROM subscription_grants WHERE merchant_id=$1`, merchantID).Scan(&active, &voided); err != nil {
		t.Fatal(err)
	}
	if active != 1 || voided != 1 {
		t.Fatalf("manual grant chains active=%d voided=%d", active, voided)
	}
	t.Run("V25.B.r03/v01", func(t *testing.T) {
		var expiresAt time.Time
		var encodedChannels []byte
		var audits int
		if err := db.Pool.QueryRow(t.Context(), `SELECT expires_at,snapshot->'channels' FROM subscriptions WHERE merchant_id=$1`, merchantID).Scan(&expiresAt, &encodedChannels); err != nil {
			t.Fatal("read manually overridden entitlement")
		}
		var channels []string
		if err := json.Unmarshal(encodedChannels, &channels); err != nil {
			t.Fatalf("decode manually overridden entitlement channels: %v", err)
		}
		if !expiresAt.Equal(secondExpiry.Truncate(time.Microsecond)) || len(channels) != 1 || channels[0] != "wxpay" || second.Source != "manual" {
			t.Fatalf("manual override expiry=%v channels=%v grant=%#v", expiresAt, channels, second)
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='subscription.manual_grant' AND merchant_id=$1`, merchantID).Scan(&audits); err != nil || audits != 2 {
			t.Fatalf("manual grant audit count=%d err=%v, want both overrides", audits, err)
		}
	})
	t.Run("V25.B.r03/v02", func(t *testing.T) {
		_, err := application.Services.Subscription.Grant(t.Context(), admin, merchantID, subscription.GrantRequest{
			PlanID: itoa(planID), ExpiresAt: db.Clock.Now().Add(90 * 24 * time.Hour), Channels: []string{"alipay"}, ExpectedRevision: first.Revision, Reason: "stale revision must conflict",
		})
		if !hasErrorCode(err, "VERSION_CONFLICT") {
			t.Fatalf("stale manual grant revision error=%v, want VERSION_CONFLICT", err)
		}
	})
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }
