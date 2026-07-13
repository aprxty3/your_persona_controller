package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type TokenStore struct {
	client *redis.Client
}

// NewTokenStore constructs a new TokenStore.
func NewTokenStore(client *redis.Client) *TokenStore {
	return &TokenStore{client: client}
}

// StoreResetJTI registers a freshly issued reset_token jti.
func (s *TokenStore) StoreResetJTI(ctx context.Context, jti, userID string, ttl time.Duration) error {
	if err := s.client.Set(ctx, resetJTIKey(jti), userID, ttl).Err(); err != nil {
		return fmt.Errorf("redis: store reset jti: %w", err)
	}
	return nil
}

// ConsumeResetJTI atomically fetches AND deletes the jti.
func (s *TokenStore) ConsumeResetJTI(ctx context.Context, jti string) (userID string, err error) {
	val, err := s.client.GetDel(ctx, resetJTIKey(jti)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("redis: consume reset jti: %w", err)
	}
	return val, nil
}

// DenylistRefreshJTI revokes a single refresh token until its natural expiry.
func (s *TokenStore) DenylistRefreshJTI(ctx context.Context, jti string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil // already expired; nothing to revoke
	}
	if err := s.client.Set(ctx, refreshDenyKey(jti), 1, ttl).Err(); err != nil {
		return fmt.Errorf("redis: denylist refresh jti: %w", err)
	}
	return nil
}

// IsRefreshJTIDenylisted reports whether the refresh token was revoked by logout.
func (s *TokenStore) IsRefreshJTIDenylisted(ctx context.Context, jti string) (bool, error) {
	n, err := s.client.Exists(ctx, refreshDenyKey(jti)).Result()
	if err != nil {
		return false, fmt.Errorf("redis: check refresh denylist: %w", err)
	}
	return n > 0, nil
}

func resetJTIKey(jti string) string {
	return fmt.Sprintf("reset_jti:%s", jti)
}

func refreshDenyKey(jti string) string {
	return fmt.Sprintf("refresh_deny:%s", jti)
}
