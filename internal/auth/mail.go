package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wneessen/go-mail"
	"go.uber.org/zap"

	"github.com/markbeep/hyl/internal/config"
)

// Mailer sends the transactional mails hyl needs. With no SMTP host configured
// it becomes a no-op that logs, so a self-hosted instance stays usable without
// a mail server.
type Mailer struct {
	cfg    config.Config
	log    *zap.Logger
	client *mail.Client
}

// NewMailer builds the SMTP client once at boot.
func NewMailer(cfg config.Config, log *zap.Logger) (*Mailer, error) {
	m := &Mailer{cfg: cfg, log: log}
	if cfg.SMTPHost == "" {
		return m, nil
	}

	opts := []mail.Option{
		mail.WithPort(cfg.SMTPPort),
		mail.WithTimeout(15 * time.Second),
	}
	switch cfg.SMTPTLS {
	case "implicit":
		opts = append(opts, mail.WithSSLPort(true), mail.WithTLSPolicy(mail.TLSMandatory))
	case "none":
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}
	if cfg.SMTPUser != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(cfg.SMTPUser), mail.WithPassword(cfg.SMTPPass))
	}

	client, err := mail.NewClient(cfg.SMTPHost, opts...)
	if err != nil {
		return nil, fmt.Errorf("smtp client: %w", err)
	}
	m.client = client
	return m, nil
}

// Enabled reports whether mail is actually delivered.
func (m *Mailer) Enabled() bool { return m.client != nil }

// Send delivers one plain-text mail. A failure to send never fails the caller's
// request; it is logged so the operator can act on it.
func (m *Mailer) Send(ctx context.Context, to, subject, body string) error {
	if m.client == nil {
		m.log.Warn("smtp is not configured; mail not sent",
			zap.String("to", to), zap.String("subject", subject))
		return nil
	}
	msg := mail.NewMsg()
	if err := msg.From(m.cfg.SMTPFrom); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, body)

	if err := m.client.DialAndSendWithContext(ctx, msg); err != nil {
		m.log.Error("sending mail failed", zap.String("to", to), zap.String("subject", subject), zap.Error(err))
		return err
	}
	return nil
}

// ErrNoMailConfigured is returned by helpers that need a delivery address.
var ErrNoMailConfigured = errors.New("no mail transport configured")
