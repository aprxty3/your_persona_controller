package taskqueue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

// Queue names — per TECHNICAL_DOCUMENTATION Section 6 classification rules.
// CPU-bound and I/O-bound tasks MUST be on different queues.
const (
	QueueCritical = "critical" // OTP emails — highest priority
	QueueDefault  = "default"  // standard background tasks
	QueuePDF      = "pdf"      // CPU-bound PDF generation
	QueueLow      = "low"      // anonymization, purge cron
)

// Task type identifiers — matches TECHNICAL_DOCUMENTATION Section 6 table.
const (
	TaskSendEmail   = "send:email"
	TaskGeneratePDF = "generate:pdf"
	TaskAnonymize   = "anonymize:user"
	TaskPurgeGuest  = "purge:guest-ttl"
)

// SendEmailPayload is the canonical payload for all send:email tasks.
// Locale-aware: the worker uses this locale to select the correct email template (FR-I8).
type SendEmailPayload struct {
	Type   string `json:"type"`             // otp_verification | otp_reset | deletion_confirmed
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	OTP    string `json:"otp,omitempty"`   // present for otp_* types
	Locale string `json:"locale"`
}

// GeneratePDFPayload is the canonical payload for generate:pdf tasks.
type GeneratePDFPayload struct {
	TestResultID string `json:"test_result_id"`
}

// Dispatcher is the DRY interface for enqueuing all background jobs.
// A single implementation wraps *asynq.Client — no per-use-case enqueue helpers.
type Dispatcher interface {
	EnqueueEmail(ctx context.Context, payload SendEmailPayload, queue string) error
	EnqueuePDFGeneration(ctx context.Context, payload GeneratePDFPayload) error
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
	return enqueue(d.client, TaskSendEmail, payload, queue, 5)
}

func (d *AsynqDispatcher) EnqueuePDFGeneration(ctx context.Context, payload GeneratePDFPayload) error {
	return enqueue(d.client, TaskGeneratePDF, payload, QueuePDF, 3)
}

// enqueue is the single shared implementation for all task types.
// DO NOT duplicate this logic per-task — that's what this DRY helper prevents.
func enqueue(client *asynq.Client, taskType string, payload any, queueName string, maxRetry int) error {
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

	_, err = client.EnqueueContext(context.Background(), task, opts...)
	return err
}

// EnqueueTask is the legacy low-level helper kept for backward compatibility.
// Prefer using a Dispatcher instance injected via Wire.
func EnqueueTask(client *asynq.Client, taskType string, payload any, queueName string, maxRetry int) error {
	return enqueue(client, taskType, payload, queueName, maxRetry)
}
