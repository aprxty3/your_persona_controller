package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/google/uuid"
)

// CreateGuestSessionRequest holds validated input from the onboarding form.
type CreateGuestSessionRequest struct {
	DisplayName string
	Age         int
	Status      string // student|worker|freelancer|unemployed|other
	Locale      string // en|id
	IPAddress   string // raw client IP (hashed and never stored directly)
}

// CreateGuestSessionResponse is returned on successful session creation.
type CreateGuestSessionResponse struct {
	SessionID         string
	ExpiresAt         time.Time
	RetryAfterSeconds int `json:"-"` // set only when Execute itself returned application.ErrRateLimited
}

// CreateGuestSessionUseCase orchestrates guest session lifecycle creation.
type CreateGuestSessionUseCase struct {
	repo          account.GuestSessionRepository
	ipRateLimiter IPRateLimiter
	log           logger.Logger
}

// NewCreateGuestSessionUseCase creates a new CreateGuestSessionUseCase.
func NewCreateGuestSessionUseCase(repo account.GuestSessionRepository, ipRateLimiter IPRateLimiter, log logger.Logger) *CreateGuestSessionUseCase {
	return &CreateGuestSessionUseCase{repo: repo, ipRateLimiter: ipRateLimiter, log: log.With("usecase", "create_guest_session")}
}

// Execute handles the generation of a 14-day persistent guest session.
func (uc *CreateGuestSessionUseCase) Execute(ctx context.Context, req CreateGuestSessionRequest) (*CreateGuestSessionResponse, error) {

	if allowed, retryAfter, err := uc.ipRateLimiter.Allow(ctx, redis.ScopeGuestSessionIP, req.IPAddress); err != nil {
		uc.log.Warn("guest session ip rate limit check skipped", "reason", "redis_error", "error", err)
	} else if !allowed {
		uc.log.Warn("guest session creation rejected", "reason", "rate_limited", "retry_after_seconds", retryAfter)
		return &CreateGuestSessionResponse{RetryAfterSeconds: retryAfter}, application.ErrRateLimited
	}

	if err := application.ValidateRequired("display_name", req.DisplayName); err != nil {
		return nil, err
	}
	if err := application.ValidateAge(req.Age, application.MinimumAge); err != nil {
		return nil, err
	}
	if err := application.ValidateStatus(req.Status); err != nil {
		return nil, err
	}
	if err := application.ValidateLocale("locale", req.Locale); err != nil {
		return nil, err
	}

	sessionID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(application.GuestDataRetention)

	session := &account.GuestSession{
		SessionID:   sessionID,
		IPHash:      hashIP(req.IPAddress),
		DisplayName: req.DisplayName,
		Age:         req.Age,
		Status:      req.Status,
		Locale:      req.Locale,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	}

	if err := uc.repo.Create(ctx, session); err != nil {
		uc.log.Error("create guest session failed", "step", "repo_create", "error", err)
		return nil, fmt.Errorf("create_guest_session: repo create: %w", err)
	}

	uc.log.Info("guest session created", "session_id", sessionID)
	return &CreateGuestSessionResponse{
		SessionID: sessionID,
		ExpiresAt: expiresAt,
	}, nil
}

// hashIP computes a SHA-256 digest of the client IP address.
func hashIP(ip string) string {
	h := sha256.Sum256([]byte(ip))
	return fmt.Sprintf("%x", h)
}
