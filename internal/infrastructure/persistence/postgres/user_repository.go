package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// UserRepository implements user.Repository backed by PostgreSQL via GORM.
type UserRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewUserRepository constructs a new UserRepository.
func NewUserRepository(db *gorm.DB, log logger.Logger) user.Repository {
	return &UserRepository{db: db, log: log.With("repository", "user")}
}

// Create inserts a new user record.
func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	m := toUserModel(u)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Create", "error", err)
		return err
	}
	return nil
}

// FindByID retrieves a user by their UUID. Returns nil, nil if not found.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*user.User, error) {
	var m UserModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindByID", "error", err)
		return nil, err
	}
	u := toUserEntity(&m)
	return &u, nil
}

// FindByEmail retrieves a user by email address. Returns nil, nil if not found.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	var m UserModel
	err := r.db.WithContext(ctx).First(&m, "email = ?", email).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindByEmail", "error", err)
		return nil, err
	}
	u := toUserEntity(&m)
	return &u, nil
}

// Update saves all mutable fields of the user.
func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	m := toUserModel(u)
	if err := r.db.WithContext(ctx).Save(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Update", "error", err)
		return err
	}
	return nil
}

// IncrementTokenVersion atomically increments token_version for the given user ID.
func (r *UserRepository) IncrementTokenVersion(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).
		Model(&UserModel{}).
		Where("id = ?", id).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).
		Error
	if err != nil {
		r.log.Error("query failed", "op", "IncrementTokenVersion", "error", err)
	}
	return err
}

// UpdateLoginAttempt updates the failed login counter and lockout timestamp.
func (r *UserRepository) UpdateLoginAttempt(ctx context.Context, id string, failedCount int, lockedUntil *time.Time) error {
	updates := map[string]interface{}{
		"failed_login_count": failedCount,
		"locked_until":       lockedUntil,
	}
	err := r.db.WithContext(ctx).
		Model(&UserModel{}).
		Where("id = ?", id).
		Updates(updates).
		Error
	if err != nil {
		r.log.Error("query failed", "op", "UpdateLoginAttempt", "error", err)
	}
	return err
}

func toUserModel(u *user.User) UserModel {
	return UserModel{
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

func toUserEntity(m *UserModel) user.User {
	var deletedAt *time.Time
	if m.DeletedAt.Valid {
		deletedAt = &m.DeletedAt.Time
	}
	return user.User{
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
