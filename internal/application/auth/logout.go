package auth

import (
	"context"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

// LogoutRequest carries the refresh token of the session being terminated.
type LogoutRequest struct {
	UserID       string // from the access token (auth middleware)
	RefreshToken string
}

// LogoutUseCase terminates ONE session: the presented refresh token is
// denylisted until its natural expiry, so it can never mint new access tokens.
// The current access token is short-lived (≤15 min) and dies on its own —
// per-request denylist checks for access tokens are deliberately skipped (KISS,
// consistent with the PRD's decision to avoid full token-family revocation).
type LogoutUseCase struct {
	jwtService *jwtservice.JWTService
	tokenStore *redis.TokenStore
	log        logger.Logger
}

// NewLogoutUseCase constructs a new LogoutUseCase.
func NewLogoutUseCase(jwtService *jwtservice.JWTService, tokenStore *redis.TokenStore, log logger.Logger) *LogoutUseCase {
	return &LogoutUseCase{
		jwtService: jwtService,
		tokenStore: tokenStore,
		log:        log.With("usecase", "logout"),
	}
}

// Execute revokes the presented refresh token. Logout is idempotent: an
// already-invalid/expired refresh token is treated as success — there is
// nothing left to revoke, which is exactly the state the caller wants.
func (uc *LogoutUseCase) Execute(ctx context.Context, req LogoutRequest) error {
	if err := application.ValidateRequired("refresh_token", req.RefreshToken); err != nil {
		return err
	}

	claims, err := uc.jwtService.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		uc.log.Info("logout no-op", "reason", "token_already_invalid", "user_id", req.UserID)
		return nil
	}

	// Refuse to revoke somebody else's token — the refresh token must belong
	// to the authenticated caller.
	if claims.Subject != req.UserID {
		uc.log.Warn("logout rejected", "reason", "subject_mismatch", "user_id", req.UserID)
		return application.ErrInvalidToken
	}

	if claims.ExpiresAt != nil {
		if err := uc.tokenStore.DenylistRefreshJTI(ctx, claims.ID, time.Until(claims.ExpiresAt.Time)); err != nil {
			uc.log.Warn("failed to denylist refresh token on logout", "user_id", req.UserID, "error", err)
			// Fail-open: worst case the refresh token stays valid until expiry.
		}
	}

	uc.log.Info("user logged out", "user_id", req.UserID)
	return nil
}
