package jobs

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	mail "github.com/wneessen/go-mail"
	"github.com/wneessen/go-mail/smtp"
	"vanillapay/internal/mailtmpl"
	"vanillapay/internal/platform"
)

const (
	mailAttemptTimeout = 45 * time.Second
	mailLeaseDuration  = 2 * time.Minute
	mailRetryDelay     = `CASE attempts WHEN 1 THEN interval '1 minute' WHEN 2 THEN interval '5 minutes' WHEN 3 THEN interval '30 minutes' END`
)

func (w *mailWorker) Work(ctx context.Context, _ *river.Job[MailArgs]) error {
	return w.deliverQueued(ctx, 100)
}

type queuedMail struct {
	id        int64
	recipient []byte
	payload   []byte
	template  string
	lease     pgtype.UUID
}

func (w *mailWorker) deliverQueued(ctx context.Context, limit int) error {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	// A crashed worker consumed its attempt. Start its backoff at lease expiry.
	_, err := w.pool.Exec(ctx, `UPDATE mail_deliveries
		SET state='failed',last_error='smtp_delivery_failed',updated_at=now(),
			next_attempt_at=lease_expires_at+`+mailRetryDelay+`,lease_token=NULL,lease_expires_at=NULL
		WHERE state IN ('queued','failed') AND lease_expires_at<=now()`)
	if err != nil {
		return err
	}
	for range limit {
		found, err := w.deliverNext(ctx)
		if err != nil || !found {
			return err
		}
	}
	return nil
}

func (w *mailWorker) deliverNext(ctx context.Context) (bool, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, mailAttemptTimeout)
	defer cancel()
	leaseDuration := pgtype.Interval{Microseconds: mailLeaseDuration.Microseconds(), Valid: true}
	var item queuedMail
	err := w.pool.QueryRow(attemptCtx, `WITH candidate AS (
		SELECT id FROM mail_deliveries WHERE state IN ('queued','failed') AND attempts<4
			AND lease_token IS NULL AND (next_attempt_at IS NULL OR next_attempt_at<=now())
		ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED
	)
	UPDATE mail_deliveries m SET attempts=m.attempts+1,lease_token=gen_random_uuid(),
		lease_expires_at=now()+$1::interval,next_attempt_at=NULL,updated_at=now()
	FROM candidate WHERE m.id=candidate.id
	RETURNING m.id,m.recipient_cipher,m.payload_cipher,m.template,m.lease_token`, leaseDuration).
		Scan(&item.id, &item.recipient, &item.payload, &item.template, &item.lease)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	closeSession, sendErr := w.send(attemptCtx, item)
	if closeSession != nil {
		defer closeSession()
	}
	state, code := "sent", ""
	if sendErr != nil {
		state, code = "failed", "smtp_delivery_failed"
	}
	// Commit DATA acceptance before QUIT, even if the job was canceled.
	updateCtx, cancelUpdate := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelUpdate()
	tag, err := w.pool.Exec(updateCtx, `UPDATE mail_deliveries SET state=$3,last_error=NULLIF($4,''),
		sent_at=CASE WHEN $3='sent' THEN now() ELSE sent_at END,updated_at=now(),
		next_attempt_at=CASE WHEN $3='failed' THEN now()+`+mailRetryDelay+` END,
		lease_token=NULL,lease_expires_at=NULL
	WHERE id=$1 AND lease_token=$2`, item.id, item.lease, state, code)
	if err == nil && tag.RowsAffected() != 1 {
		err = errors.New("mail delivery lease is no longer owned")
	}
	return true, err
}

// send returns graceful cleanup only after the server acknowledges DATA.
func (w *mailWorker) send(ctx context.Context, item queuedMail) (func(), error) {
	recipient, err := platform.Decrypt(w.key[:], item.recipient)
	if err != nil {
		return nil, err
	}
	plain, err := platform.Decrypt(w.key[:], item.payload)
	if err != nil {
		return nil, err
	}
	var payload mailtmpl.Payload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, err
	}
	var siteName string
	var passwordCipher []byte
	var raw []byte
	if err := w.pool.QueryRow(ctx, `SELECT mail,COALESCE(site->>'site_name','') FROM site_settings WHERE singleton=true`).Scan(&raw, &siteName); err != nil {
		return nil, err
	}
	rendered, err := mailtmpl.Render(item.template, payload, siteName, w.publicBaseURL)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Host        string `json:"host"`
		Port        int    `json:"port"`
		Username    string `json:"username"`
		FromAddress string `json:"from_address"`
		FromName    string `json:"from_name"`
		TLSMode     string `json:"tls_mode"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if err := w.pool.QueryRow(ctx, `SELECT encrypted_value FROM secret_settings WHERE name='smtp_password'`).Scan(&passwordCipher); err != nil {
		return nil, err
	}
	password, err := platform.Decrypt(w.key[:], passwordCipher)
	if err != nil {
		return nil, err
	}
	opts := []mail.Option{mail.WithPort(cfg.Port), mail.WithUsername(cfg.Username), mail.WithPassword(string(password)), mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.Host})}
	if cfg.TLSMode == "tls" {
		opts = append(opts, mail.WithSSL())
	} else {
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}
	client, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, err
	}
	msg := mail.NewMsg()
	if err := msg.FromFormat(cfg.FromName, cfg.FromAddress); err != nil {
		return nil, err
	}
	if err := msg.To(string(recipient)); err != nil {
		return nil, err
	}
	msg.Subject(rendered.Subject)
	msg.SetBodyString(mail.TypeTextPlain, rendered.Text)
	msg.AddAlternativeString(mail.TypeTextHTML, rendered.HTML)
	session, err := client.DialToSMTPClientWithContext(ctx)
	if err != nil {
		return nil, err
	}
	// go-mail can spend two extra socket deadlines on greeting and
	// negotiation after dialing. The lease covers both plus the attempt.
	deadline, _ := ctx.Deadline()
	if err := session.UpdateDeadline(time.Until(deadline)); err != nil {
		_ = session.Close()
		return nil, err
	}
	if err := writeSMTPMessage(session, msg); err != nil {
		_ = session.Close()
		return nil, err
	}
	return func() {
		if err := client.CloseWithSMTPClient(session); err != nil {
			_ = session.Close() // A failed QUIT can leave the socket open.
		}
	}, nil
}

func writeSMTPMessage(session *smtp.Client, msg *mail.Msg) error {
	from, err := msg.GetSender(false)
	if err != nil {
		return err
	}
	recipients, err := msg.GetRecipients()
	if err != nil {
		return err
	}
	if err := session.Mail(from); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := session.Rcpt(recipient); err != nil {
			return err
		}
	}
	data, err := session.Data()
	if err != nil {
		return err
	}
	if _, err := msg.WriteTo(data); err != nil {
		return err
	}
	return data.Close() // Waits for the final DATA reply (250 on acceptance).
}
