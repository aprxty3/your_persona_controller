package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

// RefreshTokenRequest carries the long-lived credential to be exchanged.
type RefreshTokenRequest struct {
	RefreshToken string
}

// RefreshTokenResponse returns a fresh session pair (refresh token rotation).
type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshTokenUseCase exchanges a valid refresh token for a brand-new session
// pair. The presented refresh token is rotated: its jti is denylisted for its
// remaining lifetime, so it cannot be replayed after the exchange.
type RefreshTokenUseCase struct {
	userRepo   user.Repository
	jwtService *jwtservice.JWTService
	tokenStore *redis.TokenStore
	log        logger.Logger
}

// NewRefreshTokenUseCase constructs a new RefreshTokenUseCase.
func NewRefreshTokenUseCase(
	userRepo user.Repository,
	jwtService *jwtservice.JWTService,
	tokenStore *redis.TokenStore,
	log logger.Logger,
) *RefreshTokenUseCase {
	return &RefreshTokenUseCase{
		userRepo:   userRepo,
		jwtService: jwtService,
		tokenStore: tokenStore,
		log:        log.With("usecase", "refresh_token"),
	}
}

// Execute validates the refresh token and issues a rotated session pair.
func (uc *RefreshTokenUseCase) Execute(ctx context.Context, req RefreshTokenRequest) (*RefreshTokenResponse, error) {
	if err := application.ValidateRequired("refresh_token", req.RefreshToken); err != nil {
		return nil, err
	}

	claims, err := uc.jwtService.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		uc.log.Warn("refresh rejected", "reason", "invalid_token", "error", err)
		return nil, application.ErrInvalidToken
	}

	// Per-session logout check. Redis failure = fail-open (AGENTS.md degradation
	// matrix): a revoked-but-unexpired token slipping through during a Redis
	// outage is preferred over blocking every session refresh.
	denied, err := uc.tokenStore.IsRefreshJTIDenylisted(ctx, claims.ID)
	if err != nil {
		uc.log.Warn("refresh denylist check skipped", "reason", "redis_error", "error", err)
	} else if denied {
		uc.log.Warn("refresh rejected", "reason", "token_revoked", "user_id", claims.Subject)
		return nil, application.ErrInvalidToken
	}

	u, err := uc.userRepo.FindByID(ctx, claims.Subject)
	if err != nil {
		uc.log.Error("refresh failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("refresh: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("refresh rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidToken
	}

	// token_version guard: logout-all / reset-password bumps the version and
	// instantly invalidates every previously issued refresh token.
	if claims.TokenVersion != u.TokenVersion {
		uc.log.Warn("refresh rejected", "reason", "token_version_mismatch", "user_id", u.ID)
		return nil, application.ErrTokenVersionMismatch
	}

	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion)
	if err != nil {
		uc.log.Error("refresh failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("refresh: %w", err)
	}

	// Rotation: revoke the presented token for its remaining lifetime.
	if claims.ExpiresAt != nil {
		if err := uc.tokenStore.DenylistRefreshJTI(ctx, claims.ID, time.Until(claims.ExpiresAt.Time)); err != nil {
			uc.log.Warn("failed to denylist rotated refresh token", "user_id", u.ID, "error", err)
		}
	}

	uc.log.Info("session refreshed", "user_id", u.ID)
	return &RefreshTokenResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}
