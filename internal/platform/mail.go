package platform

import (
	"context"
	"errors"
)

var ErrMailerUnavailable = errors.New("mailer is unavailable")

type MailMessage struct {
	To      string
	Subject string
	Text    string
	HTML    string
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
