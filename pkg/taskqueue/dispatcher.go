package taskqueue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

// EnqueueTask is a generic wrapper to dispatch background jobs to Redis via Asynq.
func EnqueueTask(client *asynq.Client, taskType string, payload any, queueName string, maxRetry int) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	task := asynq.NewTask(taskType, payloadBytes)

	// DRY configuration for all background tasks
	opts := []asynq.Option{
		asynq.Queue(queueName),
		asynq.MaxRetry(maxRetry),
		asynq.Timeout(5 * time.Minute),
	}

	_, err = client.EnqueueContext(context.Background(), task, opts...)
	return err
}
