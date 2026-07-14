package account

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// GuestSessionRepository implements account.GuestSessionRepository backed by PostgreSQL via GORM.
type GuestSessionRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewGuestSessionRepository constructs a new GuestSessionRepository.
func NewGuestSessionRepository(db *gorm.DB, log logger.Logger) account.GuestSessionRepository {
	return &GuestSessionRepository{db: db, log: log.With("repository", "guestsession")}
}

// Create inserts a new guest session record.
func (r *GuestSessionRepository) Create(ctx context.Context, s *account.GuestSession) error {
	m := toGuestSessionModel(s)
	return postgres.LogQueryError(r.log, "Create", r.db.WithContext(ctx).Create(&m).Error)
}

// FindBySessionID retrieves a guest session by its UUID. Returns nil, nil if not found.
func (r *GuestSessionRepository) FindBySessionID(ctx context.Context, sessionID string) (*account.GuestSession, error) {
	var m postgres.GuestSessionModel
	err := r.db.WithContext(ctx).First(&m, "session_id = ?", sessionID).Error
	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindBySessionID", err); err != nil {
		return nil, err
	}
	s := toGuestSessionEntity(&m)
	return &s, nil
}

// Update saves all mutable fields of the guest session.
func (r *GuestSessionRepository) Update(ctx context.Context, s *account.GuestSession) error {
	m := toGuestSessionModel(s)
	return postgres.LogQueryError(r.log, "Update", r.db.WithContext(ctx).Save(&m).Error)
}

// FindExpiredUnclaimed retrieves guest sessions that are expired and unclaimed.
func (r *GuestSessionRepository) FindExpiredUnclaimed(ctx context.Context) ([]account.GuestSession, error) {
	var models []postgres.GuestSessionModel
	err := r.db.WithContext(ctx).
		Where("expires_at < NOW() AND claimed_by_user_id IS NULL").
		Find(&models).Error
	if err := postgres.LogQueryError(r.log, "FindExpiredUnclaimed", err); err != nil {
		return nil, err
	}
	sessions := make([]account.GuestSession, len(models))
	for i, m := range models {
		sessions[i] = toGuestSessionEntity(&m)
	}
	return sessions, nil
}

// AnonymizeClaimedByUser scrubs PII on all guest sessions claimed by the user.
func (r *GuestSessionRepository) AnonymizeClaimedByUser(ctx context.Context, userID string) error {
	err := r.db.WithContext(ctx).
		Model(&postgres.GuestSessionModel{}).
		Where("claimed_by_user_id = ?", userID).
		Updates(map[string]interface{}{
			"display_name": "",
			"age":          0,
			"status":       "",
			"ip_hash":      "",
		}).
		Error
	return postgres.LogQueryError(r.log, "AnonymizeClaimedByUser", err)
}

// DeleteBySessionID removes a guest session from the database.
func (r *GuestSessionRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	err := r.db.WithContext(ctx).
		Delete(&postgres.GuestSessionModel{}, "session_id = ?", sessionID).Error
	return postgres.LogQueryError(r.log, "DeleteBySessionID", err)
}

func toGuestSessionModel(s *account.GuestSession) postgres.GuestSessionModel {
	return postgres.GuestSessionModel{
		SessionID:       s.SessionID,
		IPHash:          s.IPHash,
		DisplayName:     s.DisplayName,
		Age:             s.Age,
		Status:          s.Status,
		Locale:          s.Locale,
		ClaimedByUserID: s.ClaimedByUserID,
		CreatedAt:       s.CreatedAt,
		ExpiresAt:       s.ExpiresAt,
	}
}

func toGuestSessionEntity(m *postgres.GuestSessionModel) account.GuestSession {
	return account.GuestSession{
		SessionID:       m.SessionID,
		IPHash:          m.IPHash,
		DisplayName:     m.DisplayName,
		Age:             m.Age,
		Status:          m.Status,
		Locale:          m.Locale,
		ClaimedByUserID: m.ClaimedByUserID,
		CreatedAt:       m.CreatedAt,
		ExpiresAt:       m.ExpiresAt,
	}
}
