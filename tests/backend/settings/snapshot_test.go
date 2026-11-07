package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_SettingsGETUsesOneSnapshotDuringPUT(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	admin := bootstrapRecoveryAdmin(t, fixture, "settings-snapshot")
	before := readConsoleSettingsGroups(t, fixture, admin)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	blocker, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(t.Context()) }()
	if _, err := blocker.Exec(ctx, `LOCK TABLE secret_settings IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	var blockerPID int32
	if err := blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}

	const path = "/api/v1/console/settings"
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- fixture.Request(t, http.MethodGet, path, nil, admin) }()

	// The blocked secret query proves GET already read the settings row.
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for {
		var blocked bool
		err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_locks WHERE relation = 'secret_settings'::regclass AND NOT granted
				AND $1 = ANY(pg_blocking_pids(pid))
			)`, blockerPID).Scan(&blocked)
		if err != nil {
			t.Fatalf("wait for GET to reach the secret read: %v", err)
		}
		if blocked {
			break
		}
		select {
		case response := <-result:
			t.Fatalf("GET finished before the secret read was blocked: HTTP %d", response.Code)
		case <-ctx.Done():
			t.Fatal("GET did not reach the secret read")
		case <-poll.C:
		}
	}

	newSite := map[string]any{"site_name": "Concurrent site update", "logo_asset_id": nil}
	write := fixture.Request(t, http.MethodPut, path+"/site", map[string]any{
		"expected_version": before["version"], "value": newSite,
	}, admin)
	assertRecoveryAPIError(t, write, http.StatusOK, "")

	// Change all three markers while the reader is blocked, regardless of query order.
	// The test only needs nonempty bytea values. Billing has no merchant configured.
	if _, err := blocker.Exec(ctx, `
		INSERT INTO secret_settings (name, encrypted_value)
		VALUES ('smtp_password', decode('01', 'hex')), ('turnstile_secret', decode('01', 'hex')), ('billing_key', decode('01', 'hex'))
	`); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	var response *httptest.ResponseRecorder
	select {
	case response = <-result:
	case <-ctx.Done():
		t.Fatal("GET did not finish after the lock was released")
	}
	assertRecoveryAPIError(t, response, http.StatusOK, "")
	var snapshot struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	staleWrite := fixture.Request(t, http.MethodPut, path+"/site", map[string]any{
		"expected_version": snapshot.Data["version"], "value": snapshot.Data["site"],
	}, admin)
	assertRecoveryAPIError(t, staleWrite, http.StatusConflict, "VERSION_CONFLICT")

	for _, field := range []string{"version", "site", "mail", "security", "billing"} {
		if string(snapshot.Data[field]) != string(before[field]) {
			t.Errorf("GET returned %s from a different snapshot", field)
		}
	}
	after := readConsoleSettingsGroups(t, fixture, admin)
	for _, field := range []string{"version", "site", "mail", "security", "billing"} {
		if string(after[field]) == string(before[field]) {
			t.Errorf("fresh GET still returned the original %s", field)
		}
	}
}
