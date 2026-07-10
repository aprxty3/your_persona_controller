package account

import (
	"context"
	"errors"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// VerificationTokenRepository implements account.VerificationTokenRepository backed by PostgreSQL via GORM.
// The GORM schema (postgres.VerificationTokenModel) is shared/global — see persistence/postgres/models.go.
type VerificationTokenRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewVerificationTokenRepository constructs a new VerificationTokenRepository.
func NewVerificationTokenRepository(db *gorm.DB, log logger.Logger) account.VerificationTokenRepository {
	return &VerificationTokenRepository{db: db, log: log.With("repository", "verificationtoken")}
}

// Create inserts a new verification token record.
func (r *VerificationTokenRepository) Create(ctx context.Context, t *account.VerificationToken) error {
	m := toVerificationTokenModel(t)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Create", "error", err)
		return err
	}
	return nil
}

// FindActiveByUserAndType returns the single active (not expired, not used) token.
// Lookup is scoped to (user_id, type) for index optimization and security.
func (r *VerificationTokenRepository) FindActiveByUserAndType(ctx context.Context, userID string, tokenType account.TokenType) (*account.VerificationToken, error) {
	var m postgres.VerificationTokenModel
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
	t := toVerificationTokenEntity(&m)
	return &t, nil
}

// IncrementAttemptCount atomically increments the attempt_count for a token.
func (r *VerificationTokenRepository) IncrementAttemptCount(ctx context.Context, tokenID string) error {
	err := r.db.WithContext(ctx).
		Model(&postgres.VerificationTokenModel{}).
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
		Model(&postgres.VerificationTokenModel{}).
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
func (r *VerificationTokenRepository) ExpireAllActiveForUser(ctx context.Context, userID string, tokenType account.TokenType) error {
	err := r.db.WithContext(ctx).
		Model(&postgres.VerificationTokenModel{}).
		Where("user_id = ? AND type = ? AND used_at IS NULL AND expires_at > ?", userID, string(tokenType), time.Now()).
		Update("expires_at", time.Now()).
		Error
	if err != nil {
		r.log.Error("query failed", "op", "ExpireAllActiveForUser", "error", err)
	}
	return err
}

func toVerificationTokenModel(t *account.VerificationToken) postgres.VerificationTokenModel {
	return postgres.VerificationTokenModel{
		ID:           t.ID,
		UserID:       t.UserID,
		Token:        t.Token,
		Type:         string(t.Type),
		AttemptCount: t.AttemptCount,
		ExpiresAt:    t.ExpiresAt,
		UsedAt:       t.UsedAt,
	}
}

func toVerificationTokenEntity(m *postgres.VerificationTokenModel) account.VerificationToken {
	return account.VerificationToken{
		ID:           m.ID,
		UserID:       m.UserID,
		Token:        m.Token,
		Type:         account.TokenType(m.Type),
		AttemptCount: m.AttemptCount,
		ExpiresAt:    m.ExpiresAt,
		UsedAt:       m.UsedAt,
		CreatedAt:    m.CreatedAt,
	}
}
