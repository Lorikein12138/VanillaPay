package settings

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	settingsservice "vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_CollectionBoundsResetAuditsAffectedMerchantsAndRollsBackOnAuditFailure(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantA, merchantAKey := db.SeedMerchant(t)
	merchantB, _ := db.SeedMerchant(t)
	merchantC, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	merchants := []int64{merchantA, merchantB, merchantC}
	planID := db.SeedPlan(t, "bounds historical order", 1000, 30, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, application, merchantA, merchantAKey, planID)

	version, bounds := readPaymentBounds(t, db)
	bounds.MaxTTLSeconds = 86400
	bounds.FloatMaxCents = 999
	if _, err := application.Services.Settings.UpdateGroup(ctx, admin, "payment", settingsservice.UpdateGroup{ExpectedVersion: version, Value: marshalPaymentBounds(t, bounds)}); err != nil {
		t.Fatalf("widen global bounds to create valid historical overrides: %v", err)
	}
	for i, merchantID := range merchants {
		actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
		ttl, floatLimit := 3600, 300
		switch i {
		case 0:
			floatLimit = 900
		case 2:
			ttl = 300
		}
		if _, err := application.Services.Settings.SaveCollection(ctx, actor, settingsservice.SaveCollectionRequest{OrderTTLSeconds: &ttl, FloatLimitCents: &floatLimit}); err != nil {
			t.Fatalf("prepare historical collection overrides for merchant %d: %v", merchantID, err)
		}
	}
	order, err := application.Services.Payment.Create(ctx, payment.CreateCommand{
		MerchantID: merchantA, Protocol: "v1", Purpose: "external", OutTradeNo: "bounds-existing-order",
		Name: "Existing order", OriginalCents: 1000, NotifyURL: "https://merchant.example.test/notify",
		ReturnURL: "https://merchant.example.test/return", ClientIP: "192.0.2.70", SourceIP: "192.0.2.70",
		Channel: "alipay", Actor: platform.Actor{Role: "merchant", SubjectID: merchantA, MerchantID: merchantA},
	})
	if err != nil {
		t.Fatalf("create historical order before narrowing collection bounds: %v", err)
	}
	historicalExpiry := order.ExpiresAt

	version, bounds = readPaymentBounds(t, db)
	bounds.MaxTTLSeconds = 1800
	bounds.FloatMaxCents = 500
	if _, err := db.Pool.Exec(ctx, `CREATE FUNCTION reject_bounds_reset_audit_for_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='collection.bounds_reset' THEN RAISE EXCEPTION 'injected bounds audit failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatalf("create bounds audit fault function: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `CREATE TRIGGER reject_bounds_reset_audit_for_test BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_bounds_reset_audit_for_test()`); err != nil {
		t.Fatalf("create bounds audit fault trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_bounds_reset_audit_for_test ON audit_logs`)
		_, _ = db.Pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS reject_bounds_reset_audit_for_test()`)
	})
	if _, err := application.Services.Settings.UpdateGroup(ctx, admin, "payment", settingsservice.UpdateGroup{ExpectedVersion: version, Value: marshalPaymentBounds(t, bounds)}); err == nil {
		t.Fatal("global bounds update succeeded after a merchant reset audit was rejected")
	}
	var rolledBackVersion int64
	var rolledBackPayment []byte
	if err := db.Pool.QueryRow(ctx, `SELECT version,payment FROM site_settings WHERE singleton=true`).Scan(&rolledBackVersion, &rolledBackPayment); err != nil {
		t.Fatal("read global payment settings after rejected audit")
	}
	var afterFailure settingsservice.GroupBounds
	if err := json.Unmarshal(rolledBackPayment, &afterFailure); err != nil {
		t.Fatal("decode payment bounds after rejected audit")
	}
	if rolledBackVersion != version || afterFailure.MaxTTLSeconds != 86400 || afterFailure.FloatMaxCents != 999 {
		t.Fatalf("global payment bounds/version partially committed after failed audit: version=%d bounds=%#v", rolledBackVersion, afterFailure)
	}
	assertCollectionOverride(t, db, merchantA, 3600, 900)
	assertCollectionOverride(t, db, merchantB, 3600, 300)
	if _, err := db.Pool.Exec(ctx, `DROP TRIGGER reject_bounds_reset_audit_for_test ON audit_logs`); err != nil {
		t.Fatalf("remove bounds audit fault trigger: %v", err)
	}
	result, err := application.Services.Settings.UpdateGroup(ctx, admin, "payment", settingsservice.UpdateGroup{ExpectedVersion: version, Value: marshalPaymentBounds(t, bounds)})
	if err != nil || result.AffectedMerchants != 2 {
		t.Fatalf("successful global bounds update = %#v, %v; want two affected merchants", result, err)
	}
	assertCollectionReset(t, db, merchantA, true, true)
	assertCollectionReset(t, db, merchantB, true, false)
	assertCollectionReset(t, db, merchantC, false, false)

	rows, err := db.Pool.Query(ctx, `SELECT merchant_id,action,safe_metadata FROM audit_logs WHERE action='collection.bounds_reset' AND merchant_id=ANY($1::bigint[]) ORDER BY merchant_id`, merchants)
	if err != nil {
		t.Fatalf("read per-merchant reset audits: %v", err)
	}
	defer rows.Close()
	seen := map[int64][]string{}
	for rows.Next() {
		var merchantID int64
		var action string
		var raw []byte
		if err := rows.Scan(&merchantID, &action, &raw); err != nil {
			t.Fatalf("scan bounds reset audit: %v", err)
		}
		var metadata struct {
			Fields  []string `json:"fields"`
			Version int64    `json:"version"`
		}
		if err := json.Unmarshal(raw, &metadata); err != nil {
			t.Fatalf("decode bounds reset metadata: %v", err)
		}
		if action != "collection.bounds_reset" || metadata.Version < 2 || len(metadata.Fields) == 0 {
			t.Fatalf("bounds reset audit = merchant %d action %q metadata %#v", merchantID, action, metadata)
		}
		seen[merchantID] = metadata.Fields
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate bounds reset audits: %v", err)
	}
	if len(seen) != 2 || !sameStrings(seen[merchantA], []string{"float_limit_cents", "order_ttl_seconds"}) || !sameStrings(seen[merchantB], []string{"order_ttl_seconds"}) {
		t.Fatalf("per-merchant reset field audits = %#v", seen)
	}
	var updateMetadata json.RawMessage
	if err := db.Pool.QueryRow(ctx, `SELECT safe_metadata FROM audit_logs WHERE action='settings.update_group' AND target_id='payment' AND result='success' ORDER BY id DESC LIMIT 1`).Scan(&updateMetadata); err != nil {
		t.Fatalf("read global payment update audit: %v", err)
	}
	var update map[string]any
	if err := json.Unmarshal(updateMetadata, &update); err != nil || update["affected_merchants"] != float64(2) {
		t.Fatalf("settings.update_group metadata = %s, err=%v", updateMetadata, err)
	}
	t.Run("V17.B.r05/v01", func(t *testing.T) {
		if result.AffectedMerchants != 2 {
			t.Fatalf("affected merchant report = %d, want 2", result.AffectedMerchants)
		}
		assertCollectionReset(t, db, merchantA, true, true)
		assertCollectionReset(t, db, merchantB, true, false)
		var resetCount int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='collection.bounds_reset' AND merchant_id=ANY($1::bigint[]) AND result='success'`, []int64{merchantA, merchantB}).Scan(&resetCount); err != nil {
			t.Fatal("count affected merchant reset audits")
		}
		if resetCount != 2 {
			t.Fatalf("affected merchant reset audit count = %d, want 2", resetCount)
		}
		var reported string
		if err := db.Pool.QueryRow(ctx, `SELECT safe_metadata->>'affected_merchants' FROM audit_logs WHERE action='settings.update_group' AND target_id='payment' AND result='success' ORDER BY id DESC LIMIT 1`).Scan(&reported); err != nil || reported != "2" {
			t.Fatalf("payment update reported affected_merchants=%q, err=%v; want 2", reported, err)
		}
	})
	t.Run("V17.B.r05/v02", func(t *testing.T) {
		var persisted time.Time
		if err := db.Pool.QueryRow(ctx, `SELECT expires_at FROM orders WHERE trade_no=$1`, order.TradeNo).Scan(&persisted); err != nil {
			t.Fatalf("read historical order expiration: %v", err)
		}
		if !persisted.Equal(historicalExpiry.Truncate(time.Microsecond)) {
			t.Fatalf("historical order expiration changed from %s to %s after bounds update", historicalExpiry, persisted)
		}
	})
	t.Run("V16.B.r01/v11", func(t *testing.T) {
		var resetRows, updateRows int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='collection.bounds_reset' AND merchant_id=ANY($1::bigint[]) AND result='success'`, merchants).Scan(&resetRows); err != nil {
			t.Fatal("count successful per-merchant bounds-reset audit rows")
		}
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='settings.update_group' AND target_id='payment' AND result='success' AND safe_metadata->>'affected_merchants'='2'`).Scan(&updateRows); err != nil {
			t.Fatal("count payment settings update audit rows with affected merchant count")
		}
		if resetRows != 2 || updateRows != 1 {
			t.Fatalf("bounds update audit rows reset=%d group_update=%d", resetRows, updateRows)
		}
	})
}

func readPaymentBounds(t *testing.T, db *testsupport.Database) (int64, settingsservice.GroupBounds) {
	t.Helper()
	var version int64
	var raw []byte
	if err := db.Pool.QueryRow(context.Background(), `SELECT version,payment FROM site_settings WHERE singleton=true`).Scan(&version, &raw); err != nil {
		t.Fatalf("read payment settings: %v", err)
	}
	var bounds settingsservice.GroupBounds
	if err := json.Unmarshal(raw, &bounds); err != nil {
		t.Fatalf("decode payment settings: %v", err)
	}
	return version, bounds
}

func marshalPaymentBounds(t *testing.T, bounds settingsservice.GroupBounds) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(bounds)
	if err != nil {
		t.Fatalf("encode payment settings: %v", err)
	}
	return raw
}

func assertCollectionOverride(t *testing.T, db *testsupport.Database, merchantID int64, wantTTL, wantFloat int) {
	t.Helper()
	var ttl, floatLimit int
	if err := db.Pool.QueryRow(context.Background(), `SELECT order_ttl_seconds,float_limit_cents FROM merchant_collection_settings WHERE merchant_id=$1`, merchantID).Scan(&ttl, &floatLimit); err != nil {
		t.Fatalf("read historical overrides for merchant %d: %v", merchantID, err)
	}
	if ttl != wantTTL || floatLimit != wantFloat {
		t.Fatalf("merchant %d overrides after failed update = %d/%d, want %d/%d", merchantID, ttl, floatLimit, wantTTL, wantFloat)
	}
}

func assertCollectionReset(t *testing.T, db *testsupport.Database, merchantID int64, wantTTLNull, wantFloatNull bool) {
	t.Helper()
	var ttl *int
	var floatLimit *int
	if err := db.Pool.QueryRow(context.Background(), `SELECT order_ttl_seconds,float_limit_cents FROM merchant_collection_settings WHERE merchant_id=$1`, merchantID).Scan(&ttl, &floatLimit); err != nil {
		t.Fatalf("read reconciled overrides for merchant %d: %v", merchantID, err)
	}
	if (ttl == nil) != wantTTLNull || (floatLimit == nil) != wantFloatNull {
		t.Fatalf("merchant %d reset overrides = %v/%v, want null=%t/%t", merchantID, ttl, floatLimit, wantTTLNull, wantFloatNull)
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
