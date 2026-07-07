package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// VerificationTokenRepository implements verificationtoken.Repository backed by PostgreSQL via GORM.
type VerificationTokenRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewVerificationTokenRepository constructs a new VerificationTokenRepository.
func NewVerificationTokenRepository(db *gorm.DB, log logger.Logger) verificationtoken.Repository {
	return &VerificationTokenRepository{db: db, log: log.With("repository", "verificationtoken")}
}

// Create inserts a new verification token record.
func (r *VerificationTokenRepository) Create(ctx context.Context, t *verificationtoken.VerificationToken) error {
	m := toVTModel(t)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Create", "error", err)
		return err
	}
	return nil
}

// FindActiveByUserAndType returns the single active (not expired, not used) token.
// Lookup is scoped to (user_id, type) for index optimization and security.
func (r *VerificationTokenRepository) FindActiveByUserAndType(ctx context.Context, userID string, tokenType verificationtoken.TokenType) (*verificationtoken.VerificationToken, error) {
	var m VerificationTokenModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND expires_at > ? AND used_at IS NULL", userID, string(tokenType), time.Now()).
		Order("created_at DESC").
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindActiveByUserAndType", "error", err)
		return nil, err
	}
	t := toVTEntity(&m)
	return &t, nil
}

// IncrementAttemptCount atomically increments the attempt_count for a token.
func (r *VerificationTokenRepository) IncrementAttemptCount(ctx context.Context, tokenID string) error {
	err := r.db.WithContext(ctx).
		Model(&VerificationTokenModel{}).
		Where("id = ?", tokenID).
		UpdateColumn("attempt_count", gorm.Expr("attempt_count + 1")).
		Error
	if err != nil {
		r.log.Error("query failed", "op", "IncrementAttemptCount", "error", err)
	}
	return err
}

// MarkUsed sets used_at to now, consuming the token permanently.
func (r *VerificationTokenRepository) MarkUsed(ctx context.Context, tokenID string) error {
	now := time.Now()
	err := r.db.WithContext(ctx).
		Model(&VerificationTokenModel{}).
		Where("id = ?", tokenID).
		Update("used_at", now).
		Error
	if err != nil {
		r.log.Error("query failed", "op", "MarkUsed", "error", err)
	}
	return err
}

// ExpireAllActiveForUser force-expires all unused tokens of the given (user_id, type).
// Called prior to OTP generation to ensure the single-valid-token invariant.
func (r *VerificationTokenRepository) ExpireAllActiveForUser(ctx context.Context, userID string, tokenType verificationtoken.TokenType) error {
	err := r.db.WithContext(ctx).
		Model(&VerificationTokenModel{}).
		Where("user_id = ? AND type = ? AND used_at IS NULL AND expires_at > ?", userID, string(tokenType), time.Now()).
		Update("expires_at", time.Now()).
		Error
	if err != nil {
		r.log.Error("query failed", "op", "ExpireAllActiveForUser", "error", err)
	}
	return err
}

func toVTModel(t *verificationtoken.VerificationToken) VerificationTokenModel {
	return VerificationTokenModel{
		ID:           t.ID,
		UserID:       t.UserID,
		Token:        t.Token,
		Type:         string(t.Type),
		AttemptCount: t.AttemptCount,
		ExpiresAt:    t.ExpiresAt,
		UsedAt:       t.UsedAt,
	}
}

func toVTEntity(m *VerificationTokenModel) verificationtoken.VerificationToken {
	return verificationtoken.VerificationToken{
		ID:           m.ID,
		UserID:       m.UserID,
		Token:        m.Token,
		Type:         verificationtoken.TokenType(m.Type),
		AttemptCount: m.AttemptCount,
		ExpiresAt:    m.ExpiresAt,
		UsedAt:       m.UsedAt,
		CreatedAt:    m.CreatedAt,
	}
}
