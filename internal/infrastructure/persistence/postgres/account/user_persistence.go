package account

import (
	"context"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// UserRepository implements account.UserRepository backed by PostgreSQL via GORM.
type UserRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewUserRepository constructs a new UserRepository.
func NewUserRepository(db *gorm.DB, log logger.Logger) account.UserRepository {
	return &UserRepository{db: db, log: log.With("repository", "user")}
}

// Create inserts a new user record.
func (r *UserRepository) Create(ctx context.Context, u *account.User) error {
	m := toUserModel(u)
	return postgres.LogQueryError(r.log, "Create", r.db.WithContext(ctx).Create(&m).Error)
}

// FindByID retrieves a user by their UUID. Returns nil, nil if not found.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*account.User, error) {
	var m postgres.UserModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindByID", err); err != nil {
		return nil, err
	}
	u := toUserEntity(&m)
	return &u, nil
}

// FindByEmail retrieves a user by email address. Returns nil, nil if not found.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*account.User, error) {
	var m postgres.UserModel
	err := r.db.WithContext(ctx).First(&m, "email = ?", email).Error
	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindByEmail", err); err != nil {
		return nil, err
	}
	u := toUserEntity(&m)
	return &u, nil
}

// Update saves all mutable fields of the user.
func (r *UserRepository) Update(ctx context.Context, u *account.User) error {
	m := toUserModel(u)
	return postgres.LogQueryError(r.log, "Update", r.db.WithContext(ctx).Save(&m).Error)
}

// IncrementTokenVersion atomically increments token_version for the given user ID.
func (r *UserRepository) IncrementTokenVersion(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).
		Model(&postgres.UserModel{}).
		Where("id = ?", id).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).
		Error
	return postgres.LogQueryError(r.log, "IncrementTokenVersion", err)
}

// Anonymize scrubs all PII columns in a single UPDATE.
func (r *UserRepository) Anonymize(ctx context.Context, id string, scrubbedEmail string) error {
	now := time.Now()
	err := r.db.WithContext(ctx).
		Unscoped().
		Model(&postgres.UserModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"email":            scrubbedEmail,
			"password_hash":    "", // no valid bcrypt hash — login permanently impossible
			"display_name":     "",
			"age":              0,
			"status":           "",
			"referred_by_code": nil,
			"deleted_at":       now,
			"anonymized_at":    now,
			"token_version":    gorm.Expr("token_version + 1"),
		}).
		Error
	return postgres.LogQueryError(r.log, "Anonymize", err)
}

// UpdateLoginAttempt updates the failed login counter and lockout timestamp.
func (r *UserRepository) UpdateLoginAttempt(ctx context.Context, id string, failedCount int, lockedUntil *time.Time) error {
	updates := map[string]interface{}{
		"failed_login_count": failedCount,
		"locked_until":       lockedUntil,
	}
	err := r.db.WithContext(ctx).
		Model(&postgres.UserModel{}).
		Where("id = ?", id).
		Updates(updates).
		Error
	return postgres.LogQueryError(r.log, "UpdateLoginAttempt", err)
}

func toUserModel(u *account.User) postgres.UserModel {
	return postgres.UserModel{
		ID:               u.ID,
		Email:            u.Email,
		PasswordHash:     u.PasswordHash,
		DisplayName:      u.DisplayName,
		Age:              u.Age,
		Status:           u.Status,
		ReferredByCode:   u.ReferredByCode,
		PreferredLocale:  u.PreferredLocale,
		EmailVerifiedAt:  u.EmailVerifiedAt,
		AnonymizedAt:     u.AnonymizedAt,
		TokenVersion:     u.TokenVersion,
		FailedLoginCount: u.FailedLoginCount,
		LockedUntil:      u.LockedUntil,
	}
}

func toUserEntity(m *postgres.UserModel) account.User {
	var deletedAt *time.Time
	if m.DeletedAt.Valid {
		deletedAt = &m.DeletedAt.Time
	}
	return account.User{
		ID:               m.ID,
		Email:            m.Email,
		PasswordHash:     m.PasswordHash,
		DisplayName:      m.DisplayName,
		Age:              m.Age,
		Status:           m.Status,
		ReferredByCode:   m.ReferredByCode,
		PreferredLocale:  m.PreferredLocale,
		EmailVerifiedAt:  m.EmailVerifiedAt,
		CreatedAt:        m.CreatedAt,
		DeletedAt:        deletedAt,
		AnonymizedAt:     m.AnonymizedAt,
		TokenVersion:     m.TokenVersion,
		FailedLoginCount: m.FailedLoginCount,
		LockedUntil:      m.LockedUntil,
	}
}
