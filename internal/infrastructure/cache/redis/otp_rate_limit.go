package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	otpCooldownTTL   = 60 * time.Second
	otpDailyCapCount = 5
	otpDailyTTL      = 24 * time.Hour
)

// OTPRateLimitService enforces a 60-second cooldown and 5x/day rolling request limit.
type OTPRateLimitService struct {
	client *redis.Client
}

// NewOTPRateLimitService constructs a new OTPRateLimitService.
func NewOTPRateLimitService(client *redis.Client) *OTPRateLimitService {
	return &OTPRateLimitService{client: client}
}

// CheckAndConsume verifies whether the email has exceeded its rate limits.
// Returns (0, nil) if verification succeeds and OTP dispatch is permitted.
func (s *OTPRateLimitService) CheckAndConsume(ctx context.Context, email string) (retryAfterSeconds int, err error) {
	ttl, err := s.client.TTL(ctx, cooldownKey(email)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, fmt.Errorf("redis: get cooldown ttl: %w", err)
	}
	if ttl > 0 {
		return int(ttl.Seconds()), nil
	}

	count, err := s.client.Get(ctx, dailyCountKey(email)).Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, fmt.Errorf("redis: get daily send counter: %w", err)
	}
	if count >= otpDailyCapCount {
		return int(otpDailyTTL.Seconds()), nil
	}

	return 0, nil
}

// SetCooldown writes the 60s restriction key and increments the daily counter.
func (s *OTPRateLimitService) SetCooldown(ctx context.Context, email string) error {
	pipe := s.client.Pipeline()

	pipe.Set(ctx, cooldownKey(email), 1, otpCooldownTTL)
	pipe.Incr(ctx, dailyCountKey(email))
	pipe.ExpireNX(ctx, dailyCountKey(email), otpDailyTTL)

	_, err := pipe.Exec(ctx)
	return err
}

func cooldownKey(email string) string {
	return fmt.Sprintf("otp:cooldown:%s", email)
}

func dailyCountKey(email string) string {
	return fmt.Sprintf("otp:daily:%s", email)
}
