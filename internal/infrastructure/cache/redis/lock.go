package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// releaseScript performs a compare-and-delete: only removes the key if its value still matches the token that acquired it.
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end
`)

// DistributedLockService implements assessment.DistributedLockService using
// Redis SETNX with a per-acquisition owner token for safe release.
type DistributedLockService struct {
	client *redis.Client
	tokens sync.Map // key -> owner token, populated by AcquireLock, consumed by ReleaseLock
}

// NewDistributedLockService constructs a new DistributedLockService.
func NewDistributedLockService(client *redis.Client) assessment.DistributedLockService {
	return &DistributedLockService{client: client}
}

// AcquireLock attempts to atomically claim the key via SETNX. Returns false (no error) if another request already holds the lock.
func (s *DistributedLockService) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	token := uuid.New().String()

	ok, err := s.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis: acquire lock: %w", err)
	}
	if ok {
		s.tokens.Store(key, token)
	}
	return ok, nil
}

// ReleaseLock releases the lock only if it's still owned by the token this process acquired it with.
func (s *DistributedLockService) ReleaseLock(ctx context.Context, key string) error {
	tokenVal, ok := s.tokens.LoadAndDelete(key)
	if !ok {
		return nil
	}

	if err := releaseScript.Run(ctx, s.client, []string{key}, tokenVal).Err(); err != nil {
		return fmt.Errorf("redis: release lock: %w", err)
	}
	return nil
}
