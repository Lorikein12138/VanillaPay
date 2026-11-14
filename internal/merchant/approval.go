package merchant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	sqlc "vanillapay/internal/db/sqlc"
	"vanillapay/internal/mailtmpl"
	"vanillapay/internal/platform"
)

type RejectRegistrationRequest struct {
	Reason string `json:"reason"`
}

type ApprovalDTO struct {
	ApprovalState      string     `json:"approval_state"`
	ApprovalReason     *string    `json:"approval_reason"`
	ApprovalReviewedAt *time.Time `json:"approval_reviewed_at"`
}

func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	if !emptyBody(w, r) {
		return
	}
	id, err := merchantID(chi.URLParam(r, "id"))
	if err != nil {
		merchantWrite(w, r, nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.ReviewRegistration(r.Context(), actor, id, true, "")
	merchantWrite(w, r, dto, err, 200)
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	var req RejectRegistrationRequest
	if !merchantDecode(w, r, &req, 1<<20) {
		return
	}
	id, err := merchantID(chi.URLParam(r, "id"))
	if err != nil {
		merchantWrite(w, r, nil, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}, 200)
		return
	}
	actor, _ := platform.ActorFromContext(r.Context())
	dto, err := h.Service.ReviewRegistration(r.Context(), actor, id, false, req.Reason)
	merchantWrite(w, r, dto, err, 200)
}

// ReviewRegistration approves a pending or rejected merchant, or rejects a pending one.
// Approving an already approved merchant is a no-op so retries are safe.
func (s *Service) ReviewRegistration(ctx context.Context, actor platform.Actor, merchantID int64, approve bool, reason string) (ApprovalDTO, error) {
	if actor.Role != "admin" {
		return ApprovalDTO{}, &platform.Error{Code: "FORBIDDEN", HTTPStatus: 403}
	}
	if merchantID <= 0 {
		return ApprovalDTO{}, &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
	}
	reason = strings.TrimSpace(reason)
	if !approve && (reason == "" || utf8.RuneCountInString(reason) > 500) {
		return ApprovalDTO{}, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400, Fields: map[string]string{"reason": "invalid"}}
	}
	var result ApprovalDTO
	err := sqlc.InTx(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := sqlc.LockBusinessRows(ctx, tx, []int64{merchantID}, false); err != nil {
			return err
		}
		var state, email, locale string
		if err := tx.QueryRow(ctx, `SELECT approval_state,email_normalized,COALESCE(preferences->>'locale','zh-CN') FROM merchants WHERE id=$1`, merchantID).Scan(&state, &email, &locale); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &platform.Error{Code: "NOT_FOUND", HTTPStatus: 404}
			}
			return err
		}
		if approve && state == "approved" {
			return tx.QueryRow(ctx, `SELECT approval_state,approval_reason,approval_reviewed_at FROM merchants WHERE id=$1`, merchantID).Scan(&result.ApprovalState, &result.ApprovalReason, &result.ApprovalReviewedAt)
		}
		if !approve && state != "pending" {
			return &platform.Error{Code: "VERSION_CONFLICT", HTTPStatus: 409}
		}
		next, action, template := "approved", "merchant.approve", mailtmpl.RegistrationApproved
		var storedReason *string
		if !approve {
			next, action, template = "rejected", "merchant.reject", mailtmpl.RegistrationRejected
			storedReason = &reason
		}
		now := s.clock.Now()
		if err := tx.QueryRow(ctx, `UPDATE merchants SET approval_state=$1,approval_reason=$2,approval_reviewed_at=$3,updated_at=$3 WHERE id=$4 RETURNING approval_state,approval_reason,approval_reviewed_at`, next, storedReason, now, merchantID).Scan(&result.ApprovalState, &result.ApprovalReason, &result.ApprovalReviewedAt); err != nil {
			return err
		}
		dedupe := fmt.Sprintf("registration:%d:%s:%d", merchantID, next, now.UnixNano())
		if err := s.queueMailTx(ctx, tx, email, template, dedupe, mailtmpl.Payload{Locale: locale, Reason: reason}); err != nil {
			return err
		}
		if err := s.recordRegistrationNotification(ctx, tx, merchantID, template, dedupe, reason); err != nil {
			return err
		}
		metadata := map[string]any{"from": state}
		if !approve {
			metadata["reason_length"] = utf8.RuneCountInString(reason)
		}
		return recordAudit(ctx, tx, actor, &merchantID, action, "merchant", fmt.Sprint(merchantID), metadata)
	})
	return result, err
}

func (s *Service) queueMailTx(ctx context.Context, tx pgx.Tx, recipient, template, dedupe string, payload mailtmpl.Payload) error {
	recipientCipher, err := platform.Encrypt(s.key[:], []byte(recipient), s.random)
	if err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	payloadCipher, err := platform.Encrypt(s.key[:], data, s.random)
	if err != nil {
		return err
	}
	id, err := platform.RandomToken(s.random, 18)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO mail_deliveries(public_id,dedupe_key,template,recipient_cipher,payload_cipher,state) VALUES($1,$2,$3,$4,$5,'queued') ON CONFLICT(dedupe_key) DO NOTHING`, id, dedupe, template, recipientCipher, payloadCipher)
	return err
}

func (s *Service) recordRegistrationNotification(ctx context.Context, tx pgx.Tx, merchantID int64, kind, dedupe, reason string) error {
	title := map[string]string{"zh_CN": "注册审核已通过", "en": "Registration approved"}
	summary := map[string]string{"zh_CN": "你现在可以使用商户后台和收款功能。", "en": "You can now use your merchant dashboard and accept payments."}
	if kind == mailtmpl.RegistrationRejected {
		title = map[string]string{"zh_CN": "注册审核未通过", "en": "Registration declined"}
		summary = map[string]string{"zh_CN": reason, "en": reason}
	}
	payload, err := json.Marshal(map[string]any{"title": title, "summary": summary})
	if err != nil {
		return err
	}
	id, err := platform.RandomToken(s.random, 18)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO notifications(public_id,merchant_id,kind,source_id,dedupe_key,safe_payload) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(dedupe_key) DO NOTHING`, id, merchantID, kind, fmt.Sprint(merchantID), dedupe, payload)
	return err
}
