package auth

import (
	"encoding/json"
	"strings"
	"testing"

	"vanillapay/internal/platform"
)

func TestUnit_LoginFailureAuditRedaction(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		actor      platform.Actor
		wantRole   string
		wantID     int64
		wantMID    bool
		wantTarget string
	}{
		{name: "unknown merchant principal is null", role: "merchant", actor: platform.Actor{Role: "merchant", MerchantID: 91}, wantRole: "merchant", wantTarget: "unknown"},
		{name: "known merchant account remains identifiable", role: "merchant", actor: platform.Actor{Role: "merchant", SubjectID: 91, MerchantID: 91}, wantRole: "merchant", wantID: 91, wantMID: true, wantTarget: "91"},
		{name: "unknown administrator principal is null", role: "admin", actor: platform.Actor{Role: "admin", SubjectID: -1}, wantRole: "admin", wantTarget: "unknown"},
		{name: "unrecognized role is safely normalized", role: "system-root", actor: platform.Actor{Role: "system-root", SubjectID: 91, MerchantID: 91}, wantRole: "system", wantTarget: "91", wantID: 91},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := loginFailureEvent(test.role, "auth.login", "INVALID_CREDENTIALS", test.actor)
			if event.Result != "failure" || event.Action != "auth.login" || event.TargetType != "account" || event.TargetID != test.wantTarget || event.Actor.Role != test.wantRole || event.Actor.SubjectID != test.wantID {
				t.Fatalf("failure event identity = %#v", event)
			}
			if (event.MerchantID != nil) != test.wantMID || (event.MerchantID != nil && *event.MerchantID != test.actor.MerchantID) {
				t.Fatalf("failure event merchant ownership = %#v", event.MerchantID)
			}
			var metadata map[string]string
			if err := json.Unmarshal(event.SafeMetadata, &metadata); err != nil {
				t.Fatal(err)
			}
			if len(metadata) != 1 || metadata["error_code"] != "INVALID_CREDENTIALS" {
				t.Fatalf("failure metadata = %#v", metadata)
			}
			serialized, _ := json.Marshal(event.SafeMetadata)
			for _, forbidden := range []string{"password", "identifier", "totp", "recovery", "cookie", "token"} {
				if strings.Contains(strings.ToLower(string(serialized)), forbidden) {
					t.Fatalf("sensitive field name %q appeared in event: %s", forbidden, serialized)
				}
			}
		})
	}
}
