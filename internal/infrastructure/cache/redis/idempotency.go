package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/dto"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// idempotencyEnvelope is the JSON shape stored in Redis
type idempotencyEnvelope struct {
	PayloadHash string             `json:"payload_hash"`
	Response    dto.SubmitResponse `json:"response"`
}

// IdempotencyService implements assessment.IdempotencyService using Redis.
type IdempotencyService struct {
	client *redis.Client
	log    logger.Logger
}

// NewIdempotencyService constructs a new IdempotencyService.
func NewIdempotencyService(client *redis.Client, log logger.Logger) assessment.IdempotencyService {
	return &IdempotencyService{client: client, log: log.With("service", "idempotency")}
}

// Check returns the cached response if key exists and payloadHash matches.
func (s *IdempotencyService) Check(ctx context.Context, key string, payloadHash string) (*dto.SubmitResponse, error) {
	raw, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis: get idempotency key: %w", err)
	}

	var env idempotencyEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		s.log.Warn("idempotency envelope corrupted, treating as cache miss", "key", key, "error", err)
		return nil, nil
	}

	if env.PayloadHash != payloadHash {
		return nil, application.ErrIdempotencyKeyReused
	}

	resp := env.Response
	return &resp, nil
}

// Save writes the response under key, tagged with payloadHash, for ttl.
func (s *IdempotencyService) Save(ctx context.Context, key string, payloadHash string, response *dto.SubmitResponse, ttl time.Duration) error {
	env := idempotencyEnvelope{PayloadHash: payloadHash, Response: *response}

	raw, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal idempotency envelope: %w", err)
	}

	if err := s.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("redis: save idempotency key: %w", err)
	}
	return nil
}
