package jobs

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	mail "github.com/wneessen/go-mail"
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
		if err != nil {
			state, code = "failed", "smtp_delivery_failed"
		}
		if _, dbErr := w.pool.Exec(ctx, `UPDATE mail_deliveries SET state=$1,last_error=NULLIF($2,''),sent_at=CASE WHEN $1='sent' THEN now() ELSE sent_at END,updated_at=now() WHERE id=$3`, state, code, item.id); dbErr != nil {
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
	var payload struct {
		Title   map[string]string `json:"title"`
		Summary map[string]string `json:"summary"`
		Code    string            `json:"code"`
		Locale  string            `json:"locale"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil {
		return err
	}
	locale := strings.ToLower(payload.Locale)
	if locale != "en" {
		locale = "zh_cn"
	}
	subject := payload.Title[locale]
	body := payload.Summary[locale]
	if body == "" {
		body = payload.Code
	}
	if subject == "" {
		subject = item.template
	}
	if body == "" {
		body = subject
	}
	var host, user, from, fromName, tlsMode string
	var port int
	var passwordCipher []byte
	var raw []byte
	if err := w.pool.QueryRow(ctx, `SELECT mail FROM site_settings WHERE singleton=true`).Scan(&raw); err != nil {
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
	host, user, from, fromName, tlsMode, port = cfg.Host, cfg.Username, cfg.FromAddress, cfg.FromName, cfg.TLSMode, cfg.Port
	if err := w.pool.QueryRow(ctx, `SELECT encrypted_value FROM secret_settings WHERE name='smtp_password'`).Scan(&passwordCipher); err != nil {
		return err
	}
	password, err := platform.Decrypt(w.key[:], passwordCipher)
	if err != nil {
		return err
	}
	opts := []mail.Option{mail.WithPort(port), mail.WithUsername(user), mail.WithPassword(string(password)), mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: host})}
	if tlsMode == "tls" {
		opts = append(opts, mail.WithSSL())
	} else {
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}
	client, err := mail.NewClient(host, opts...)
	if err != nil {
		return err
	}
	msg := mail.NewMsg()
	if err := msg.FromFormat(fromName, from); err != nil {
		return err
	}
	if err := msg.To(string(recipient)); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, body)
	return client.DialAndSendWithContext(ctx, msg)
}
