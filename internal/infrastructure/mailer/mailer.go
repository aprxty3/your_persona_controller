package mailer

import (
	"context"
	"fmt"
	"net/smtp"
)

// Mailer defines the contract for sending transactional emails.
type Mailer interface {
	SendEmail(ctx context.Context, to, subject, body string) error
	SendOTP(ctx context.Context, to, otp, purpose, locale string) error
}

// SMTPMailer handles sending emails over SMTP using Go's net/smtp package.
type SMTPMailer struct {
	host     string
	port     int
	username string
	password string
	from     string
}

// NewSMTPMailer creates a new configured SMTPMailer.
func NewSMTPMailer(host string, port int, username, password, from string) *SMTPMailer {
	return &SMTPMailer{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
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
		errChan <- smtp.SendMail(addr, auth, m.from, []string{to}, []byte(msg))
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

// SendOTP formats and sends an OTP email based on the purpose and locale.
func (m *SMTPMailer) SendOTP(ctx context.Context, to, otp, purpose, locale string) error {
	if locale != "en" && locale != "id" {
		locale = "en"
	}

	var subject, body string

	switch purpose {
	case "otp_verification":
		if locale == "id" {
			subject = "Your Persona's - Kode Verifikasi"
			body = fmt.Sprintf("Halo,\n\nKode verifikasi Anda adalah: %s\n\nKode ini berlaku selama 15 menit. Mohon jangan membagikan kode ini kepada siapa pun.\n\nHormat kami,\nTim Your Persona", otp)
		} else {
			subject = "Your Persona's - Verification Code"
			body = fmt.Sprintf("Hello,\n\nYour verification code is: %s\n\nIt is valid for 15 minutes. Please do not share this code with anyone.\n\nBest regards,\nYour Persona Team", otp)
		}
	case "otp_reset":
		if locale == "id" {
			subject = "Your Persona's - Permintaan Reset Password"
			body = fmt.Sprintf("Halo,\n\nKode verifikasi untuk reset password Anda adalah: %s\n\nKode ini berlaku selama 15 menit. Mohon jangan membagikan kode ini kepada siapa pun.\n\nHormat kami,\nTim Your Persona", otp)
		} else {
			subject = "Your Persona's - Password Reset Request"
			body = fmt.Sprintf("Hello,\n\nYour password reset verification code is: %s\n\nIt is valid for 15 minutes. Please do not share this code with anyone.\n\nBest regards,\nYour Persona Team", otp)
		}
	default:
		return fmt.Errorf("smtp mailer: unknown OTP purpose %q", purpose)
	}

	return m.SendEmail(ctx, to, subject, body)
}
