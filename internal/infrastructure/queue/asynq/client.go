// Package asynq provides the Asynq client and the queue-backed service
// implementations (PDF generation enqueue) built on it.
package asynq

import (
	"fmt"

	"github.com/hibiken/asynq"
)

// NewAsynqClient creates and returns an *asynq.Client.
func NewAsynqClient(addr string, password string, db int) (*asynq.Client, error) {
	redisOpt := asynq.RedisClientOpt{
		Addr:     addr,
		Password: password,
		DB:       db,
	}

	client := asynq.NewClient(redisOpt)

	if addr == "" {
		return nil, fmt.Errorf("asynq client: redis address is empty")
	}

	return client, nil
}
