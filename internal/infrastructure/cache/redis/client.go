// Package redis provides the Redis client and its Redis-backed service
// implementations: rate limiting, idempotency, distributed locks, token store.
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient initializes and pings a new Redis client instance.
func NewRedisClient(addr, password string, db int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis: connection ping failed: %w", err)
	}

	return client, nil
}
