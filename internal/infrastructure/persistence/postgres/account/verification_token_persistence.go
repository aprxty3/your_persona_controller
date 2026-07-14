package account

import (
	"context"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// VerificationTokenRepository implements account.VerificationTokenRepository backed by PostgreSQL via GORM.
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
	return postgres.LogQueryError(r.log, "Create", r.db.WithContext(ctx).Create(&m).Error)
}

// FindActiveByUserAndType returns the single active (not expired, not used) token.
func (r *VerificationTokenRepository) FindActiveByUserAndType(ctx context.Context, userID string, tokenType account.TokenType) (*account.VerificationToken, error) {
	var m postgres.VerificationTokenModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND type = ? AND expires_at > ? AND used_at IS NULL", userID, string(tokenType), time.Now()).
		Order("created_at DESC").
		First(&m).Error
	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindActiveByUserAndType", err); err != nil {
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
	return postgres.LogQueryError(r.log, "IncrementAttemptCount", err)
}

// MarkUsed sets used_at to now, consuming the token permanently.
func (r *VerificationTokenRepository) MarkUsed(ctx context.Context, tokenID string) error {
	now := time.Now()
	err := r.db.WithContext(ctx).
		Model(&postgres.VerificationTokenModel{}).
		Where("id = ?", tokenID).
		Update("used_at", now).
		Error
	return postgres.LogQueryError(r.log, "MarkUsed", err)
}

// ExpireAllActiveForUser force-expires all unused tokens of the given (user_id, type).
func (r *VerificationTokenRepository) ExpireAllActiveForUser(ctx context.Context, userID string, tokenType account.TokenType) error {
	err := r.db.WithContext(ctx).
		Model(&postgres.VerificationTokenModel{}).
		Where("user_id = ? AND type = ? AND used_at IS NULL AND expires_at > ?", userID, string(tokenType), time.Now()).
		Update("expires_at", time.Now()).
		Error
	return postgres.LogQueryError(r.log, "ExpireAllActiveForUser", err)
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
