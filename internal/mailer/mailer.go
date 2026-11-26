package mailer

import (
	"context"
	"log/slog"

	"github.com/wneessen/go-mail"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

type SMTP struct {
	Host, User, Password, From string
	Port                       int
}

func (m *SMTP) Send(ctx context.Context, to, subject, body string) error {
	msg := mail.NewMsg()
	if err := msg.From(m.From); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, body)

	opts := []mail.Option{mail.WithPort(m.Port)}
	if m.User != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover), mail.WithUsername(m.User), mail.WithPassword(m.Password), mail.WithTLSPolicy(mail.TLSMandatory))
	} else {
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	}
	c, err := mail.NewClient(m.Host, opts...)
	if err != nil {
		return err
	}
	return c.DialAndSendWithContext(ctx, msg)
}

type Log struct{}

func (Log) Send(_ context.Context, to, subject, body string) error {
	slog.Warn("SMTP не настроен, письмо не отправлено", "to", to, "subject", subject, "body", body)
	return nil
}
