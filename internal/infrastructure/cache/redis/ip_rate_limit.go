package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// IPScope namespaces per-IP request counters by the endpoint category being protected
type IPScope string

const (
	ScopeLoginIP    IPScope = "login"
	ScopeRegisterIP IPScope = "register"
)

const (
	loginIPLimit     = 20
	loginIPWindow    = 15 * time.Minute
	registerIPLimit  = 10
	registerIPWindow = 15 * time.Minute
)

// IPRateLimitService enforces a fixed-window request cap per (scope, IP).
type IPRateLimitService struct {
	client *redis.Client
}

// NewIPRateLimitService constructs a new IPRateLimitService.
func NewIPRateLimitService(client *redis.Client) *IPRateLimitService {
	return &IPRateLimitService{client: client}
}

// Allow increments the counter for (scope, ip)
func (s *IPRateLimitService) Allow(ctx context.Context, scope IPScope, ip string) (allowed bool, retryAfterSeconds int, err error) {
	if ip == "" {
		return true, 0, nil
	}

	limit, window := limitFor(scope)
	key := ipKey(scope, ip)

	count, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return false, 0, fmt.Errorf("redis: incr ip counter: %w", err)
	}
	if count == 1 {
		if err := s.client.Expire(ctx, key, window).Err(); err != nil {
			return false, 0, fmt.Errorf("redis: set ip counter ttl: %w", err)
		}
	}
	if count > int64(limit) {
		ttl, err := s.client.TTL(ctx, key).Result()
		if err != nil || ttl < 0 {
			ttl = window
		}
		return false, int(ttl.Seconds()), nil
	}

	return true, 0, nil
}

func limitFor(scope IPScope) (int, time.Duration) {
	switch scope {
	case ScopeLoginIP:
		return loginIPLimit, loginIPWindow
	case ScopeRegisterIP:
		return registerIPLimit, registerIPWindow
	default:
		return registerIPLimit, registerIPWindow
	}
}

func ipKey(scope IPScope, ip string) string {
	return fmt.Sprintf("iprate:%s:%s", scope, ip)
}
