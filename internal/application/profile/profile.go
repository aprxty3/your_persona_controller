package profile

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	referralCodeLength  = 8
	referralCodeCharset = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	maxCodeGenAttempts  = 5
)

// UpdateProfileRequest carries a partial update — nil fields are left untouched.
type UpdateProfileRequest struct {
	UserID          string
	DisplayName     *string
	Age             *int
	Status          *string
	PreferredLocale *string
}

// ProfileResponse reflects the user's profile fields after an update.
type ProfileResponse struct {
	UserID          string `json:"user_id"`
	DisplayName     string `json:"display_name"`
	Age             int    `json:"age"`
	Status          string `json:"status"`
	PreferredLocale string `json:"preferred_locale"`
}

// ReferralCodeResponse carries the caller's referral code.
type ReferralCodeResponse struct {
	Code string `json:"code"`
}

// ProfileUseCase manages Member self-service profile and referral code retrieval.
type ProfileUseCase struct {
	userRepo     account.UserRepository
	referralRepo account.ReferralRepository
	log          logger.Logger
}

// NewProfileUseCase creates a new ProfileUseCase.
func NewProfileUseCase(userRepo account.UserRepository, referralRepo account.ReferralRepository, log logger.Logger) *ProfileUseCase {
	return &ProfileUseCase{
		userRepo:     userRepo,
		referralRepo: referralRepo,
		log:          log.With("usecase", "profile"),
	}
}

// UpdateProfile applies a partial update — only non-nil fields are changed.
func (uc *ProfileUseCase) UpdateProfile(ctx context.Context, req UpdateProfileRequest) (*ProfileResponse, error) {
	u, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		uc.log.Error("update profile failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("update_profile: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Error("update profile failed", "reason", "user_not_found_after_auth", "user_id", req.UserID)
		return nil, fmt.Errorf("update_profile: user %s not found", req.UserID)
	}

	if req.DisplayName != nil {
		if err := application.ValidateRequired("display_name", *req.DisplayName); err != nil {
			return nil, err
		}
		u.DisplayName = *req.DisplayName
	}
	if req.Age != nil {
		if err := application.ValidateAge(*req.Age, 13); err != nil {
			return nil, err
		}
		u.Age = *req.Age
	}
	if req.Status != nil {
		if err := application.ValidateStatus(*req.Status); err != nil {
			return nil, err
		}
		u.Status = *req.Status
	}
	if req.PreferredLocale != nil {
		if err := application.ValidateLocale("preferred_locale", *req.PreferredLocale); err != nil {
			return nil, err
		}
		u.PreferredLocale = *req.PreferredLocale
	}

	if err := uc.userRepo.Update(ctx, u); err != nil {
		uc.log.Error("update profile failed", "step", "persist", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("update_profile: %w", err)
	}

	uc.log.Info("profile updated", "user_id", u.ID)
	return &ProfileResponse{
		UserID:          u.ID,
		DisplayName:     u.DisplayName,
		Age:             u.Age,
		Status:          u.Status,
		PreferredLocale: u.PreferredLocale,
	}, nil
}

// GetReferralCode returns the caller's existing referral code, generating and persisting one on first request.
func (uc *ProfileUseCase) GetReferralCode(ctx context.Context, userID string) (*ReferralCodeResponse, error) {
	existing, err := uc.referralRepo.FindCodeByUserID(ctx, userID)
	if err != nil {
		uc.log.Error("get referral code failed", "step", "lookup_existing", "user_id", userID, "error", err)
		return nil, fmt.Errorf("get_referral_code: lookup existing: %w", err)
	}
	if existing != nil {
		return &ReferralCodeResponse{Code: existing.Code}, nil
	}

	for attempt := 0; attempt < maxCodeGenAttempts; attempt++ {
		code, err := generateReferralCode(referralCodeLength)
		if err != nil {
			uc.log.Error("get referral code failed", "step", "generate_code", "user_id", userID, "error", err)
			return nil, fmt.Errorf("get_referral_code: generate code: %w", err)
		}

		clash, err := uc.referralRepo.FindCodeByCode(ctx, code)
		if err != nil {
			uc.log.Error("get referral code failed", "step", "check_collision", "user_id", userID, "error", err)
			return nil, fmt.Errorf("get_referral_code: check collision: %w", err)
		}
		if clash != nil {
			continue
		}

		newCode := &account.ReferralCode{
			ID:        uuid.New().String(),
			UserID:    userID,
			Code:      code,
			CreatedAt: time.Now(),
		}
		if err := uc.referralRepo.CreateCode(ctx, newCode); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				winner, findErr := uc.referralRepo.FindCodeByUserID(ctx, userID)
				if findErr == nil && winner != nil {
					return &ReferralCodeResponse{Code: winner.Code}, nil
				}
				continue
			}
			uc.log.Error("get referral code failed", "step", "persist", "user_id", userID, "error", err)
			return nil, fmt.Errorf("get_referral_code: persist code: %w", err)
		}

		uc.log.Info("referral code generated", "user_id", userID)
		return &ReferralCodeResponse{Code: code}, nil
	}

	uc.log.Error("get referral code failed", "reason", "exhausted_attempts", "user_id", userID)
	return nil, fmt.Errorf("get_referral_code: exhausted %d attempts generating a unique code", maxCodeGenAttempts)
}

// generateReferralCode produces a cryptographically secure code from referralCodeCharset.
func generateReferralCode(length int) (string, error) {
	b := make([]byte, length)
	charsetSize := big.NewInt(int64(len(referralCodeCharset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, charsetSize)
		if err != nil {
			return "", fmt.Errorf("generate secure random: %w", err)
		}
		b[i] = referralCodeCharset[n.Int64()]
	}
	return string(b), nil
}
