package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	otpCooldownTTL   = 60 * time.Second
	otpDailyCapCount = 5
	otpDailyTTL      = 24 * time.Hour
)

// OTPRateLimitService enforces:
//  1. 60-second cooldown per email (no resend before 60s)
//  2. Max 5 sends per email per day (rolling 24h window)
type OTPRateLimitService struct {
	client *redis.Client
}

func NewOTPRateLimitService(client *redis.Client) *OTPRateLimitService {
	return &OTPRateLimitService{client: client}
}

// cooldownKey returns the Redis key for 60-second cooldown.
func cooldownKey(email string) string {
	return fmt.Sprintf("otp:cooldown:%s", email)
}

// dailyCountKey returns the Redis key for daily send count.
func dailyCountKey(email string) string {
	return fmt.Sprintf("otp:daily:%s", email)
}

// CheckAndConsume checks both cooldown and daily cap.
// Returns (retryAfterSeconds, err) where retryAfterSeconds > 0 means rate-limited.
// Returns (0, nil) when the caller is allowed to send.
// DOES NOT set the cooldown key — caller must call SetCooldown after successfully sending.
func (s *OTPRateLimitService) CheckAndConsume(ctx context.Context, email string) (retryAfterSeconds int, err error) {
	// Check cooldown
	ttl, err := s.client.TTL(ctx, cooldownKey(email)).Result()
	if err != nil && err != redis.Nil {
		return 0, fmt.Errorf("redis otp: check cooldown: %w", err)
	}
	if ttl > 0 {
		return int(ttl.Seconds()), nil // still in cooldown
	}

	// Check daily cap
	count, err := s.client.Get(ctx, dailyCountKey(email)).Int()
	if err != nil && err != redis.Nil {
		return 0, fmt.Errorf("redis otp: check daily count: %w", err)
	}
	if count >= otpDailyCapCount {
		return int(otpDailyTTL.Seconds()), nil // cap exceeded, return approximate retry
	}

	return 0, nil // allowed
}

// SetCooldown sets the 60-second cooldown AND increments the daily counter.
// Call this only after the email has been successfully enqueued/sent.
func (s *OTPRateLimitService) SetCooldown(ctx context.Context, email string) error {
	pipe := s.client.Pipeline()

	// Set 60-second cooldown key
	pipe.Set(ctx, cooldownKey(email), 1, otpCooldownTTL)

	// Increment daily counter (INCR creates the key if not exists)
	pipe.Incr(ctx, dailyCountKey(email))

	// Set TTL on daily counter key only if it doesn't already have one
	// We use EXPIRE with NX option (Redis 7+) — sets TTL only if key has no expiry
	pipe.ExpireNX(ctx, dailyCountKey(email), otpDailyTTL)

	_, err := pipe.Exec(ctx)
	return err
}
