package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Actor struct {
	Role                string
	SubjectID           int64
	MerchantID          int64
	RequestID           string
	IP                  string
	RecoveryOnly        bool
	RecoveryChallengeID int64
}

// AuditEvent contains only deliberately selected, non-secret audit metadata.
type AuditEvent struct {
	Actor        Actor
	MerchantID   *int64
	Action       string
	TargetType   string
	TargetID     string
	Result       string
	IP           string
	SafeMetadata json.RawMessage
}

// Auditor records an event using the caller's transaction.
type Auditor interface {
	RecordTx(context.Context, pgx.Tx, AuditEvent) error
}

type AuditExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type SQLAuditor struct{}

func (SQLAuditor) RecordTx(ctx context.Context, tx pgx.Tx, event AuditEvent) error {
	return RecordAudit(ctx, tx, event)
}

func NormalizeAuditEvent(ctx context.Context, event AuditEvent) AuditEvent {
	if metadata, ok := RequestMetadataFromContext(ctx); ok {
		event.IP = normalizeAuditIP(metadata.IP)
		event.Actor.RequestID = metadata.RequestID
	} else {
		ip := normalizeAuditIP(event.Actor.IP)
		if ip == "" {
			ip = normalizeAuditIP(event.IP)
		}
		event.IP = ip
		if event.Actor.RequestID == "" {
			if actor, ok := ActorFromContext(ctx); ok {
				event.Actor.RequestID = actor.RequestID
			}
		}
	}
	return event
}

func RecordAudit(ctx context.Context, db AuditExecutor, event AuditEvent) error {
	if db == nil {
		return errors.New("audit executor is nil")
	}
	event = NormalizeAuditEvent(ctx, event)
	var actorID any
	if event.Actor.SubjectID > 0 {
		actorID = event.Actor.SubjectID
	}
	var merchantID any
	if event.MerchantID != nil && *event.MerchantID > 0 {
		merchantID = *event.MerchantID
	}
	var ip any
	if event.IP != "" {
		if address, err := netip.ParseAddr(event.IP); err == nil && address.Zone() == "" {
			ip = address.Unmap()
		}
	}
	if len(event.SafeMetadata) == 0 {
		event.SafeMetadata = json.RawMessage(`{}`)
	}
	_, err := db.Exec(ctx, `INSERT INTO audit_logs(actor_role,actor_id,merchant_id,action,target_type,target_id,result,ip,request_id,safe_metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, event.Actor.Role, actorID, merchantID, event.Action, event.TargetType, event.TargetID, event.Result, ip, event.Actor.RequestID, event.SafeMetadata)
	return err
}
