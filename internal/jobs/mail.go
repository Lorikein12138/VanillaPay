package jobs

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	mail "github.com/wneessen/go-mail"
	"vanillapay/internal/mailtmpl"
	"vanillapay/internal/platform"
)

func (w *mailWorker) Work(ctx context.Context, _ *river.Job[MailArgs]) error {
	return w.deliverQueued(ctx, 100)
}

type queuedMail struct {
	id        int64
	recipient []byte
	payload   []byte
	template  string
}

func (w *mailWorker) deliverQueued(ctx context.Context, limit int) error {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var batch []queuedMail
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,recipient_cipher,payload_cipher,template FROM mail_deliveries WHERE state IN ('queued','failed') AND attempts<4 AND (attempts=0 OR updated_at<=now()-CASE attempts WHEN 1 THEN interval '1 minute' WHEN 2 THEN interval '5 minutes' ELSE interval '30 minutes' END) ORDER BY created_at,id LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
		if err != nil {
			return err
		}
		for rows.Next() {
			var item queuedMail
			if err := rows.Scan(&item.id, &item.recipient, &item.payload, &item.template); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, item := range batch {
			if _, err := tx.Exec(ctx, `UPDATE mail_deliveries SET attempts=attempts+1,updated_at=now() WHERE id=$1`, item.id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, item := range batch {
		err := w.send(ctx, item)
		state, code := "sent", ""
		updateCtx := ctx
		cancel := func() {}
		if err != nil {
			state, code = "failed", "smtp_delivery_failed"
		} else {
			// Persist SMTP acceptance even if the scan was canceled during delivery.
			updateCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		}
		_, dbErr := w.pool.Exec(updateCtx, `UPDATE mail_deliveries SET state=$1,last_error=NULLIF($2,''),sent_at=CASE WHEN $1='sent' THEN now() ELSE sent_at END,updated_at=now() WHERE id=$3`, state, code, item.id)
		cancel()
		if dbErr != nil {
			return dbErr
		}
	}
	return nil
}

func (w *mailWorker) send(ctx context.Context, item queuedMail) error {
	recipient, err := platform.Decrypt(w.key[:], item.recipient)
	if err != nil {
		return err
	}
	plain, err := platform.Decrypt(w.key[:], item.payload)
	if err != nil {
		return err
	}
	var payload mailtmpl.Payload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return err
	}
	var siteName string
	var passwordCipher []byte
	var raw []byte
	if err := w.pool.QueryRow(ctx, `SELECT mail,COALESCE(site->>'site_name','') FROM site_settings WHERE singleton=true`).Scan(&raw, &siteName); err != nil {
		return err
	}
	rendered, err := mailtmpl.Render(item.template, payload, siteName, w.publicBaseURL)
	if err != nil {
		return err
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
		return err
	}
	if err := w.pool.QueryRow(ctx, `SELECT encrypted_value FROM secret_settings WHERE name='smtp_password'`).Scan(&passwordCipher); err != nil {
		return err
	}
	password, err := platform.Decrypt(w.key[:], passwordCipher)
	if err != nil {
		return err
	}
	opts := []mail.Option{mail.WithPort(cfg.Port), mail.WithUsername(cfg.Username), mail.WithPassword(string(password)), mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.Host})}
	if cfg.TLSMode == "tls" {
		opts = append(opts, mail.WithSSL())
	} else {
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}
	client, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		return err
	}
	msg := mail.NewMsg()
	if err := msg.FromFormat(cfg.FromName, cfg.FromAddress); err != nil {
		return err
	}
	if err := msg.To(string(recipient)); err != nil {
		return err
	}
	msg.Subject(rendered.Subject)
	msg.SetBodyString(mail.TypeTextPlain, rendered.Text)
	msg.AddAlternativeString(mail.TypeTextHTML, rendered.HTML)
	smtp, err := client.DialToSMTPClientWithContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.CloseWithSMTPClient(smtp); err != nil {
			_ = smtp.Close() // A failed QUIT can leave the socket open.
		}
	}()
	err = client.SendWithSMTPClient(smtp, msg)
	// go-mail records DATA acceptance before RSET or QUIT can fail.
	if msg.IsDelivered() {
		return nil
	}
	return err
}
