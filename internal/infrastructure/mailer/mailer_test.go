package mailer

import (
	"context"
	"errors"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/i18n"
)

func newTestMailer(t *testing.T, sendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error) *SMTPMailer {
	t.Helper()
	catalog, err := i18n.LoadCatalog()
	if err != nil {
		t.Fatalf("failed to load real locale catalog: %v", err)
	}
	m := NewSMTPMailer("smtp.example.com", 587, "user", "pass", "noreply@example.com", catalog)
	m.sendFunc = sendFunc
	return m
}

func TestSendEmail_Success(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte
	m := newTestMailer(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotAddr, gotFrom, gotTo, gotMsg = addr, from, to, msg
		return nil
	})

	err := m.SendEmail(context.Background(), "recipient@example.com", "Hello", "World body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAddr != "smtp.example.com:587" {
		t.Errorf("expected addr smtp.example.com:587, got %q", gotAddr)
	}
	if gotFrom != "noreply@example.com" {
		t.Errorf("expected from noreply@example.com, got %q", gotFrom)
	}
	if len(gotTo) != 1 || gotTo[0] != "recipient@example.com" {
		t.Errorf("expected to=[recipient@example.com], got %v", gotTo)
	}
	msgStr := string(gotMsg)
	if !strings.Contains(msgStr, "Subject: Hello") {
		t.Errorf("expected message to contain subject line, got: %s", msgStr)
	}
	if !strings.Contains(msgStr, "World body") {
		t.Errorf("expected message to contain body, got: %s", msgStr)
	}
}

func TestSendEmail_SMTPError_Propagated(t *testing.T) {
	sentinelErr := errors.New("smtp: connection refused")
	m := newTestMailer(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return sentinelErr
	})

	err := m.SendEmail(context.Background(), "recipient@example.com", "Hello", "World")
	if err == nil {
		t.Fatal("expected an error to be returned")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected wrapped sentinel error, got: %v", err)
	}
}

// SendEmail must respect context cancellation rather than blocking forever
// on a hung SMTP dial/handshake.
func TestSendEmail_ContextCanceled_ReturnsContextError(t *testing.T) {
	blockUntil := make(chan struct{})
	defer close(blockUntil)

	m := newTestMailer(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		<-blockUntil
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := m.SendEmail(ctx, "recipient@example.com", "Hello", "World")
	if err == nil {
		t.Fatal("expected a context deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestSendEmail_NoUsername_NoAuth(t *testing.T) {
	var gotAuth smtp.Auth
	catalog, err := i18n.LoadCatalog()
	if err != nil {
		t.Fatalf("failed to load real locale catalog: %v", err)
	}
	m := NewSMTPMailer("smtp.example.com", 587, "", "", "noreply@example.com", catalog)
	m.sendFunc = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotAuth = a
		return nil
	}

	if err := m.SendEmail(context.Background(), "recipient@example.com", "Hello", "World"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != nil {
		t.Errorf("expected nil auth when username is empty, got %v", gotAuth)
	}
}

func TestSendOTP_UsesCatalogTemplateAndFormatsCode(t *testing.T) {
	var gotSubject, gotBody string
	m := newTestMailer(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		msgStr := string(msg)
		for _, line := range strings.Split(msgStr, "\r\n") {
			if strings.HasPrefix(line, "Subject: ") {
				gotSubject = strings.TrimPrefix(line, "Subject: ")
			}
		}
		gotBody = msgStr
		return nil
	})

	err := m.SendOTP(context.Background(), "recipient@example.com", "123456", "otp_verification", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotSubject == "" {
		t.Fatal("expected a non-empty subject from the catalog")
	}
	if !strings.Contains(gotBody, "123456") {
		t.Errorf("expected the OTP code to be interpolated into the body, got: %s", gotBody)
	}
}

func TestSendOTP_UnknownPurpose_ReturnsError(t *testing.T) {
	m := newTestMailer(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		t.Fatal("sendFunc should not be called for an unknown purpose")
		return nil
	})

	err := m.SendOTP(context.Background(), "recipient@example.com", "123456", "not_a_real_purpose", "en")
	if err == nil {
		t.Fatal("expected an error for an unknown OTP purpose")
	}
}

func TestSendOTP_UnsupportedLocale_ResolvesToEN(t *testing.T) {
	var gotBody string
	m := newTestMailer(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotBody = string(msg)
		return nil
	})

	if err := m.SendOTP(context.Background(), "recipient@example.com", "999999", "otp_verification", "fr"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotBody, "999999") {
		t.Errorf("expected fallback-to-EN template to still format the OTP code, got: %s", gotBody)
	}
}

func TestSendDeletionConfirmed_SendsExpectedTemplate(t *testing.T) {
	var gotBody string
	m := newTestMailer(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotBody = string(msg)
		return nil
	})

	if err := m.SendDeletionConfirmed(context.Background(), "recipient@example.com", "en"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotBody, "Subject:") {
		t.Errorf("expected a subject line, got: %s", gotBody)
	}
}

func TestSendOTP_LocaleContentDiffersBetweenENAndID(t *testing.T) {
	var enBody, idBody string
	mEN := newTestMailer(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		enBody = string(msg)
		return nil
	})
	mID := newTestMailer(t, func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		idBody = string(msg)
		return nil
	})

	if err := mEN.SendOTP(context.Background(), "a@example.com", "111111", "otp_verification", "en"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mID.SendOTP(context.Background(), "a@example.com", "111111", "otp_verification", "id"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enBody == idBody {
		t.Fatal("expected EN and ID locale bodies to differ in copy")
	}
}
