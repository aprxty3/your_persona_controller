package auth

import (
	"context"
	"fmt"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// ResetPasswordRequest carries the single-use reset_token and the new password (FR-H4 step 3/3).
type ResetPasswordRequest struct {
	ResetToken  string
	NewPassword string
}

// ResetPasswordResponse auto-logs the user in after a successful reset.
type ResetPasswordResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// ResetPasswordUseCase consumes a reset_token (atomically, single-use), updates
// the password, and revokes every existing session via token_version increment.
type ResetPasswordUseCase struct {
	db            *gorm.DB
	userRepo      user.Repository
	breachChecker PasswordBreachChecker
	jwtService    *jwtservice.JWTService
	tokenStore    *redis.TokenStore
	log           logger.Logger
}

// NewResetPasswordUseCase constructs a new ResetPasswordUseCase.
func NewResetPasswordUseCase(
	db *gorm.DB,
	userRepo user.Repository,
	breachChecker PasswordBreachChecker,
	jwtService *jwtservice.JWTService,
	tokenStore *redis.TokenStore,
	log logger.Logger,
) *ResetPasswordUseCase {
	return &ResetPasswordUseCase{
		db:            db,
		userRepo:      userRepo,
		breachChecker: breachChecker,
		jwtService:    jwtService,
		tokenStore:    tokenStore,
		log:           log.With("usecase", "reset_password"),
	}
}

// Execute performs the final password reset step and returns a fresh session.
func (uc *ResetPasswordUseCase) Execute(ctx context.Context, req ResetPasswordRequest) (*ResetPasswordResponse, error) {
	if err := application.ValidateRequired("reset_token", req.ResetToken); err != nil {
		return nil, err
	}
	// Shared password policy (FR-H1a) — same gate as registration.
	if err := ValidateNewPassword(ctx, uc.breachChecker, "new_password", req.NewPassword); err != nil {
		uc.log.Warn("reset password rejected", "reason", "password_policy", "error", err)
		return nil, err
	}

	claims, err := uc.jwtService.ParseResetToken(req.ResetToken)
	if err != nil {
		uc.log.Warn("reset password rejected", "reason", "invalid_reset_token", "error", err)
		return nil, application.ErrInvalidToken
	}

	// Single-use consumption — atomic GETDEL. Two parallel requests with the
	// same token: exactly one obtains the jti, the other gets "" and is rejected.
	// Fail-CLOSED on Redis error: this is the replay-protection gate, skipping
	// it would allow unlimited reuse within the 15-minute window.
	consumedUserID, err := uc.tokenStore.ConsumeResetJTI(ctx, claims.ID)
	if err != nil {
		uc.log.Error("reset password failed", "step", "consume_reset_jti", "error", err)
		return nil, fmt.Errorf("reset_password: consume reset jti: %w", err)
	}
	if consumedUserID == "" || consumedUserID != claims.Subject {
		uc.log.Warn("reset password rejected", "reason", "reset_token_consumed_or_mismatched")
		return nil, application.ErrInvalidToken
	}

	u, err := uc.userRepo.FindByID(ctx, claims.Subject)
	if err != nil {
		uc.log.Error("reset password failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("reset_password: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("reset password rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidToken
	}

	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		uc.log.Error("reset password failed", "step", "hash_password", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	// Password update + session revocation + lockout reset are one atomic unit:
	// a new password with old sessions still alive would defeat the reset.
	err = uc.db.Transaction(func(tx *gorm.DB) error {
		txUserRepo := txUserRepository(tx, uc.log)

		u.PasswordHash = hash
		if err := txUserRepo.Update(ctx, u); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		// Revoke ALL existing sessions (access + refresh) — FR-H4.
		if err := txUserRepo.IncrementTokenVersion(ctx, u.ID); err != nil {
			return fmt.Errorf("increment token version: %w", err)
		}
		// A successful reset proves account ownership — clear any login lockout.
		if err := txUserRepo.UpdateLoginAttempt(ctx, u.ID, 0, nil); err != nil {
			return fmt.Errorf("clear login lockout: %w", err)
		}
		return nil
	})
	if err != nil {
		uc.log.Error("reset password failed", "step", "transaction", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	// Auto-login with the NEW token version (old sessions are now all invalid).
	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion+1)
	if err != nil {
		uc.log.Error("reset password failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	uc.log.Info("password reset completed", "user_id", u.ID)
	return &ResetPasswordResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}
