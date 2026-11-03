package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type auditExecCall struct {
	query string
	args  []any
	err   error
}

func (m *auditExecCall) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	m.query = query
	m.args = append([]any(nil), args...)
	return pgconn.NewCommandTag("INSERT 0 1"), m.err
}

func TestUnit_AuditMetadataAndSQL(t *testing.T) {
	mid := int64(41)
	ctx := WithRequestMetadata(context.Background(), RequestMetadata{IP: "::ffff:203.0.113.8", RequestID: "request-from-http"})
	executor := &auditExecCall{}
	event := AuditEvent{
		Actor:      Actor{Role: "merchant", SubjectID: 41, MerchantID: 41, IP: "192.0.2.99", RequestID: "actor-request"},
		MerchantID: &mid, Action: "order.create", TargetType: "order", TargetID: "trade-public-id", Result: "success",
		IP: "198.51.100.4",
	}
	if err := RecordAudit(ctx, executor, event); err != nil {
		t.Fatal(err)
	}
	if executor.query == "" || !containsAuditIPColumn(executor.query) {
		t.Fatalf("audit SQL does not explicitly include ip: %s", executor.query)
	}
	if got, ok := executor.args[7].(netip.Addr); !ok || got.String() != "203.0.113.8" {
		t.Fatalf("IP argument = %#v, want normalized 203.0.113.8", executor.args[7])
	}
	if got := executor.args[8]; got != "request-from-http" {
		t.Fatalf("request_id argument = %#v", got)
	}
	metadata, ok := executor.args[9].(json.RawMessage)
	if !ok || string(metadata) != "{}" {
		t.Fatalf("empty safe_metadata = %#v, want {}", executor.args[9])
	}
	if executor.args[1] != int64(41) || executor.args[2] != int64(41) {
		t.Fatalf("actor/merchant ownership arguments = %#v/%#v", executor.args[1], executor.args[2])
	}

	unknown := &auditExecCall{}
	if err := RecordAudit(context.Background(), unknown, AuditEvent{Actor: Actor{Role: "admin"}, Action: "auth.login", TargetType: "account", TargetID: "unknown", Result: "failure"}); err != nil {
		t.Fatal(err)
	}
	if unknown.args[1] != nil || unknown.args[2] != nil || unknown.args[7] != nil || unknown.args[8] != "" {
		t.Fatalf("unknown/non-HTTP values were fabricated: %#v", unknown.args)
	}
}

func TestUnit_AuditWriteErrorsPropagate(t *testing.T) {
	want := errors.New("audit unavailable")
	executor := &auditExecCall{err: want}
	err := RecordAudit(context.Background(), executor, AuditEvent{Actor: Actor{Role: "system"}, Action: "test", TargetType: "test", TargetID: "1", Result: "failure"})
	if !errors.Is(err, want) {
		t.Fatalf("RecordAudit error = %v, want wrapped %v", err, want)
	}
}

func TestUnit_AuditMetadataFallbackAndHTTPEmptySource(t *testing.T) {
	actorAddress := &auditExecCall{}
	actorEvent := AuditEvent{
		Actor:  Actor{Role: "merchant", SubjectID: 7, MerchantID: 7, IP: "::ffff:198.51.100.7", RequestID: "actor-request"},
		Action: "collection.save", TargetType: "collection", TargetID: "7", Result: "success",
		IP: "203.0.113.90",
	}
	if err := RecordAudit(context.Background(), actorAddress, actorEvent); err != nil {
		t.Fatal(err)
	}
	if got, ok := actorAddress.args[7].(netip.Addr); !ok || got.String() != "198.51.100.7" || actorAddress.args[8] != "actor-request" {
		t.Fatalf("trusted actor fallback IP/request_id = %#v/%#v", actorAddress.args[7], actorAddress.args[8])
	}

	eventAddress := &auditExecCall{}
	eventEvent := AuditEvent{
		Actor: Actor{Role: "system"}, Action: "job.run", TargetType: "job", TargetID: "1", Result: "success",
		IP: "2001:db8::8",
	}
	ctx := WithActor(context.Background(), Actor{Role: "system", RequestID: "background-actor-request"})
	if err := RecordAudit(ctx, eventAddress, eventEvent); err != nil {
		t.Fatal(err)
	}
	if got, ok := eventAddress.args[7].(netip.Addr); !ok || got.String() != "2001:db8::8" || eventAddress.args[8] != "background-actor-request" {
		t.Fatalf("event IP/context actor request_id fallback = %#v/%#v", eventAddress.args[7], eventAddress.args[8])
	}

	invalid := &auditExecCall{}
	ctx = WithRequestMetadata(context.Background(), RequestMetadata{IP: "fe80::1%eth0", RequestID: "http-empty-source"})
	if err := RecordAudit(ctx, invalid, AuditEvent{
		Actor:  Actor{Role: "admin", SubjectID: 1, IP: "192.0.2.55", RequestID: "actor-must-not-win"},
		Action: "auth.login", TargetType: "account", TargetID: "1", Result: "success", IP: "198.51.100.55",
	}); err != nil {
		t.Fatal(err)
	}
	if invalid.args[7] != nil || invalid.args[8] != "http-empty-source" {
		t.Fatalf("invalid HTTP metadata must win and normalize to SQL NULL: %#v/%#v", invalid.args[7], invalid.args[8])
	}
}

func containsAuditIPColumn(query string) bool {
	return query == "INSERT INTO audit_logs(actor_role,actor_id,merchant_id,action,target_type,target_id,result,ip,request_id,safe_metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)"
}
