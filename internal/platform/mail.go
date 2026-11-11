package platform

import (
	"context"
	"errors"

	"vanillapay/internal/mailtmpl"
)

var ErrMailerUnavailable = errors.New("mailer is unavailable")

type MailMessage struct {
	To       string
	Template string
	Payload  mailtmpl.Payload
}

// Mailer is the outbound email port.
type Mailer interface {
	Send(context.Context, MailMessage) error
}

// DisabledMailer fails closed until an SMTP adapter is configured.
type DisabledMailer struct{}

func (DisabledMailer) Send(context.Context, MailMessage) error {
	return ErrMailerUnavailable
}
