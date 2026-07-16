package mailer

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/i18n"
)

// Mailer defines the contract for sending transactional emails.
type Mailer interface {
	SendEmail(ctx context.Context, to, subject, body string) error
	SendOTP(ctx context.Context, to, otp, purpose, locale string) error
	SendDeletionConfirmed(ctx context.Context, to, locale string) error
}

// SMTPMailer handles sending emails over SMTP using Go's net/smtp package.
type SMTPMailer struct {
	host     string
	port     int
	username string
	password string
	from     string
	catalog  *i18n.Catalog
	sendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// NewSMTPMailer creates a new configured SMTPMailer. catalog supplies the
// locale-aware subject/body copy (see internal/infrastructure/i18n) — the
// same *i18n.Catalog instance should be loaded once at process startup and
// shared, not reloaded per call.
func NewSMTPMailer(host string, port int, username, password, from string, catalog *i18n.Catalog) *SMTPMailer {
	return &SMTPMailer{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		catalog:  catalog,
		sendFunc: smtp.SendMail,
	}
}

// SendEmail sends a plain-text email with the given subject and body.
func (m *SMTPMailer) SendEmail(ctx context.Context, to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", m.host, m.port)

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n"+
		"%s", m.from, to, subject, body)

	var auth smtp.Auth
	if m.username != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- m.sendFunc(addr, auth, m.from, []string{to}, []byte(msg))
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("smtp: send mail to %s: %w", to, err)
		}
		return nil
	}
}

// SendOTP formats and sends an OTP email based on the purpose and locale,
// looking up copy from the message catalog (internal/infrastructure/i18n).
func (m *SMTPMailer) SendOTP(ctx context.Context, to, otp, purpose, locale string) error {
	tmpl, ok := m.catalog.Message(purpose, locale)
	if !ok {
		return fmt.Errorf("smtp mailer: unknown OTP purpose %q", purpose)
	}
	return m.SendEmail(ctx, to, tmpl.Subject, fmt.Sprintf(tmpl.Body, otp))
}

// SendDeletionConfirmed notifies the (snapshot) address that anonymization has completed deletion request.
func (m *SMTPMailer) SendDeletionConfirmed(ctx context.Context, to, locale string) error {
	tmpl, _ := m.catalog.Message("deletion_confirmed", locale)
	return m.SendEmail(ctx, to, tmpl.Subject, tmpl.Body)
}
