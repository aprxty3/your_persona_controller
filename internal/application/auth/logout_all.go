package auth

import (
	"context"
	"fmt"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

// LogoutAllUseCase terminates EVERY session of a user by incrementing
// USER.token_version (FR-H8). All previously issued access AND refresh tokens
// embed the old version and are rejected on their next use — no per-token
// tracking needed.
type LogoutAllUseCase struct {
	userRepo user.Repository
	log      logger.Logger
}

// NewLogoutAllUseCase constructs a new LogoutAllUseCase.
func NewLogoutAllUseCase(userRepo user.Repository, log logger.Logger) *LogoutAllUseCase {
	return &LogoutAllUseCase{
		userRepo: userRepo,
		log:      log.With("usecase", "logout_all"),
	}
}

// Execute bumps token_version, revoking all outstanding sessions at once.
func (uc *LogoutAllUseCase) Execute(ctx context.Context, userID string) error {
	if err := uc.userRepo.IncrementTokenVersion(ctx, userID); err != nil {
		uc.log.Error("logout-all failed", "step", "increment_token_version", "user_id", userID, "error", err)
		return fmt.Errorf("logout_all: increment token version: %w", err)
	}
	uc.log.Info("all sessions revoked", "user_id", userID)
	return nil
}
