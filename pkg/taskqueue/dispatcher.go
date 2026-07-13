package taskqueue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueuePDF      = "pdf"
	QueueLow      = "low"
)

const (
	TaskSendEmail    = "send:email"
	TaskGeneratePDF  = "generate:pdf"
	TaskAnonymize    = "anonymize:user"
	TaskPurgeGuest   = "purge:guest-ttl"
	TaskDeletionScan = "deletion:scan-expired"
)

// SendEmailPayload is the canonical payload for all send:email tasks.
type SendEmailPayload struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	OTP    string `json:"otp,omitempty"`
	Locale string `json:"locale"`
}

// GeneratePDFPayload is the canonical payload for generate:pdf tasks.
type GeneratePDFPayload struct {
	TestResultID string `json:"test_result_id"`
}

type AnonymizeUserPayload struct {
	UserID            string `json:"user_id"`
	DeletionRequestID string `json:"deletion_request_id"`
}

type Dispatcher interface {
	EnqueueEmail(ctx context.Context, payload SendEmailPayload, queue string) error
	EnqueuePDFGeneration(ctx context.Context, payload GeneratePDFPayload) error
	EnqueueAnonymize(ctx context.Context, payload AnonymizeUserPayload) error
}

// AsynqDispatcher is the concrete implementation using Asynq.
type AsynqDispatcher struct {
	client *asynq.Client
}

// NewAsynqDispatcher creates a Dispatcher backed by Asynq.
func NewAsynqDispatcher(client *asynq.Client) Dispatcher {
	return &AsynqDispatcher{client: client}
}

func (d *AsynqDispatcher) EnqueueEmail(ctx context.Context, payload SendEmailPayload, queue string) error {
	return enqueue(ctx, d.client, TaskSendEmail, payload, queue, 5)
}

func (d *AsynqDispatcher) EnqueuePDFGeneration(ctx context.Context, payload GeneratePDFPayload) error {
	return enqueue(ctx, d.client, TaskGeneratePDF, payload, QueuePDF, 3)
}

func (d *AsynqDispatcher) EnqueueAnonymize(ctx context.Context, payload AnonymizeUserPayload) error {
	return enqueue(ctx, d.client, TaskAnonymize, payload, QueueLow, 5)
}

// enqueue is the single shared implementation for all task types.
func enqueue(ctx context.Context, client *asynq.Client, taskType string, payload any, queueName string, maxRetry int) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	task := asynq.NewTask(taskType, payloadBytes)
	opts := []asynq.Option{
		asynq.Queue(queueName),
		asynq.MaxRetry(maxRetry),
		asynq.Timeout(5 * time.Minute),
	}

	_, err = client.EnqueueContext(ctx, task, opts...)
	return err
}
