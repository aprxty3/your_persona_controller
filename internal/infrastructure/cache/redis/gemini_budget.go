package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// budgetKeyTTL keeps yesterday's counter around briefly for post-hoc
// inspection, then lets Redis expire it — no purge job needed.
const budgetKeyTTL = 48 * time.Hour

// wib pins the budget day boundary to Asia/Jakarta
var wib = time.FixedZone("WIB", 7*60*60)

// GeminiBudgetService implements assessment.GeminiBudgetService: a Redis
// day-scoped token counter that acts as the aggregate cost guardrail across
// ALL users. budget <= 0 disables the cap entirely (dev default).
type GeminiBudgetService struct {
	client *redis.Client
	budget int64
	log    logger.Logger
}

// NewGeminiBudgetService constructs the service. budget is the daily token
// allowance (0 = cap disabled).
func NewGeminiBudgetService(client *redis.Client, budget int64, log logger.Logger) assessment.GeminiBudgetService {
	return &GeminiBudgetService{client: client, budget: budget, log: log.With("service", "gemini_budget")}
}

// Exceeded reports whether today's consumed tokens have reached the budget.
// Callers fail open on error (graceful-degradation matrix: Redis down must
// never block submits).
func (s *GeminiBudgetService) Exceeded(ctx context.Context) (bool, error) {
	if s.budget <= 0 {
		return false, nil
	}

	used, err := s.client.Get(ctx, budgetKey(time.Now())).Int64()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("redis: get gemini budget counter: %w", err)
	}
	return used >= s.budget, nil
}

// Consume adds tokens to today's counter after a Gemini call and logs the 80%
// and 100% threshold crossings. Each crossing fires exactly once per day by
// construction: only the single INCRBY that moves the counter over a
// threshold observes before < threshold <= after.
func (s *GeminiBudgetService) Consume(ctx context.Context, tokens int) error {
	if s.budget <= 0 || tokens <= 0 {
		return nil
	}

	key := budgetKey(time.Now())
	after, err := s.client.IncrBy(ctx, key, int64(tokens)).Result()
	if err != nil {
		return fmt.Errorf("redis: incr gemini budget counter: %w", err)
	}
	if after == int64(tokens) { // first write of the day owns the TTL
		if err := s.client.Expire(ctx, key, budgetKeyTTL).Err(); err != nil {
			return fmt.Errorf("redis: set gemini budget ttl: %w", err)
		}
	}

	before := after - int64(tokens)
	warnAt := s.budget * 80 / 100
	if before < warnAt && after >= warnAt && after < s.budget {
		s.log.Warn("gemini daily token budget at 80%", "used", after, "budget", s.budget)
	}
	if before < s.budget && after >= s.budget {
		s.log.Error("gemini daily token budget exhausted — subsequent submits fall back to static results until the WIB day rolls over", "used", after, "budget", s.budget)
	}
	return nil
}

// budgetKey names the counter for the WIB calendar day containing now.
func budgetKey(now time.Time) string {
	return buildKey("gemini", "budget", now.In(wib).Format("2006-01-02"))
}
