package auth

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
)

// IPRateLimiter is the narrow slice of Redis-backed per-IP throttling
// RegisterUseCase/SessionUseCase need — declared as an interface (rather
// than the concrete *redis.IPRateLimitService) so unit tests can fake it
// without a real Redis connection.
type IPRateLimiter interface {
	Allow(ctx context.Context, scope redis.IPScope, ip string) (allowed bool, retryAfterSeconds int, err error)
}

// OTPRateLimiter is the narrow slice of Redis-backed OTP send throttling
// AccountUseCase needs — same rationale as IPRateLimiter.
type OTPRateLimiter interface {
	CheckAndConsume(ctx context.Context, scope redis.OTPScope, email string) (retryAfterSeconds int, err error)
	SetCooldown(ctx context.Context, scope redis.OTPScope, email string) error
}
