package asynq

import (
	"fmt"

	"github.com/hibiken/asynq"
)

// NewAsynqClient creates and returns an *asynq.Client.
// addr is typically "localhost:6379"
func NewAsynqClient(addr string, password string, db int) (*asynq.Client, error) {
	redisOpt := asynq.RedisClientOpt{
		Addr:     addr,
		Password: password,
		DB:       db,
	}

	client := asynq.NewClient(redisOpt)

	// Since asynq.NewClient doesn't ping/connect immediately, we return it directly.
	// But we can check if it creates successfully (which always does unless config is bad).
	if addr == "" {
		return nil, fmt.Errorf("asynq client: redis address is empty")
	}

	return client, nil
}
