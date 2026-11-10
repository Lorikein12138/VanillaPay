package jobs

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"
	"vanillapay/internal/platform"
)

// QueueMailer persists an encrypted message and returns once the delivery is
// durable. SMTP delivery is performed by the River mail worker.
type QueueMailer struct {
	Pool   *pgxpool.Pool
	Key    [32]byte
	Random io.Reader
}

func (m QueueMailer) Send(ctx context.Context, message platform.MailMessage) error {
	if m.Pool == nil {
		return platform.ErrMailerUnavailable
	}
	random := m.Random
	if random == nil {
		random = rand.Reader
	}
	id, err := platform.RandomToken(random, 18)
	if err != nil {
		return err
	}
	dedupe, err := platform.RandomToken(random, 24)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(message.Payload)
	if err != nil {
		return err
	}
	recipientCipher, err := platform.Encrypt(m.Key[:], []byte(message.To), random)
	if err != nil {
		return err
	}
	payloadCipher, err := platform.Encrypt(m.Key[:], payload, random)
	if err != nil {
		return err
	}
	_, err = m.Pool.Exec(ctx, `INSERT INTO mail_deliveries(public_id,dedupe_key,template,recipient_cipher,payload_cipher,state) VALUES($1,$2,$3,$4,$5,'queued')`, id, dedupe, message.Template, recipientCipher, payloadCipher)
	return err
}
