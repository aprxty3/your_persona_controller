package worker

import (
	"context"
	"encoding/json"
	"testing"

	mailermocks "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/mailer/mocks"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/mock"
)

func newEmailTask(t *testing.T, payload taskqueue.SendEmailPayload) *asynq.Task {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return asynq.NewTask(taskqueue.TaskSendEmail, b)
}

func TestEmailProcessTask_MalformedPayload_ReturnsError(t *testing.T) {
	h := NewEmailHandler(mailermocks.NewMockMailer(t), testLog())

	task := asynq.NewTask(taskqueue.TaskSendEmail, []byte("not json"))
	if err := h.ProcessTask(context.Background(), task); err == nil {
		t.Fatal("expected an error for malformed payload")
	}
}

func TestEmailProcessTask_OTPVerification_CallsSendOTP(t *testing.T) {
	m := mailermocks.NewMockMailer(t)
	m.EXPECT().SendOTP(mock.Anything, "a@example.com", "123456", "otp_verification", "en").Return(nil).Once()

	h := NewEmailHandler(m, testLog())
	task := newEmailTask(t, taskqueue.SendEmailPayload{Type: "otp_verification", UserID: "user-1", Email: "a@example.com", OTP: "123456", Locale: "en"})

	if err := h.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmailProcessTask_OTPReset_CallsSendOTP(t *testing.T) {
	m := mailermocks.NewMockMailer(t)
	m.EXPECT().SendOTP(mock.Anything, "a@example.com", "654321", "otp_reset", "id").Return(nil).Once()

	h := NewEmailHandler(m, testLog())
	task := newEmailTask(t, taskqueue.SendEmailPayload{Type: "otp_reset", UserID: "user-1", Email: "a@example.com", OTP: "654321", Locale: "id"})

	if err := h.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmailProcessTask_DeletionConfirmed_CallsSendDeletionConfirmed(t *testing.T) {
	m := mailermocks.NewMockMailer(t)
	m.EXPECT().SendDeletionConfirmed(mock.Anything, "a@example.com", "en").Return(nil).Once()

	h := NewEmailHandler(m, testLog())
	task := newEmailTask(t, taskqueue.SendEmailPayload{Type: "deletion_confirmed", UserID: "user-1", Email: "a@example.com", Locale: "en"})

	if err := h.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmailProcessTask_UnknownType_SkippedWithoutError(t *testing.T) {
	m := mailermocks.NewMockMailer(t)
	h := NewEmailHandler(m, testLog())
	task := newEmailTask(t, taskqueue.SendEmailPayload{Type: "not_a_real_type", UserID: "user-1", Email: "a@example.com"})

	if err := h.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmailProcessTask_MailerError_Propagated(t *testing.T) {
	m := mailermocks.NewMockMailer(t)
	m.EXPECT().SendOTP(mock.Anything, "a@example.com", "123456", "otp_verification", "en").Return(assertErrWorker).Once()

	h := NewEmailHandler(m, testLog())
	task := newEmailTask(t, taskqueue.SendEmailPayload{Type: "otp_verification", UserID: "user-1", Email: "a@example.com", OTP: "123456", Locale: "en"})

	if err := h.ProcessTask(context.Background(), task); err == nil {
		t.Fatal("expected the mailer error to propagate for asynq retry")
	}
}
