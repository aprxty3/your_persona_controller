package postgres

import (
	"context"
	"errors"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/guestsession"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// GuestSessionRepository implements guestsession.Repository backed by PostgreSQL via GORM.
type GuestSessionRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewGuestSessionRepository constructs a new GuestSessionRepository.
func NewGuestSessionRepository(db *gorm.DB, log logger.Logger) guestsession.Repository {
	return &GuestSessionRepository{db: db, log: log.With("repository", "guestsession")}
}

// Create inserts a new guest session record.
func (r *GuestSessionRepository) Create(ctx context.Context, s *guestsession.GuestSession) error {
	m := toGuestSessionModel(s)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Create", "error", err)
		return err
	}
	return nil
}

// FindBySessionID retrieves a guest session by its UUID. Returns nil, nil if not found.
func (r *GuestSessionRepository) FindBySessionID(ctx context.Context, sessionID string) (*guestsession.GuestSession, error) {
	var m GuestSessionModel
	err := r.db.WithContext(ctx).First(&m, "session_id = ?", sessionID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindBySessionID", "error", err)
		return nil, err
	}
	s := toGuestSessionEntity(&m)
	return &s, nil
}

// Update saves all mutable fields of the guest session.
func (r *GuestSessionRepository) Update(ctx context.Context, s *guestsession.GuestSession) error {
	m := toGuestSessionModel(s)
	if err := r.db.WithContext(ctx).Save(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Update", "error", err)
		return err
	}
	return nil
}

// FindExpiredUnclaimed retrieves guest sessions that are expired and unclaimed.
func (r *GuestSessionRepository) FindExpiredUnclaimed(ctx context.Context) ([]guestsession.GuestSession, error) {
	var models []GuestSessionModel
	err := r.db.WithContext(ctx).
		Where("expires_at < NOW() AND claimed_by_user_id IS NULL").
		Find(&models).Error
	if err != nil {
		r.log.Error("query failed", "op", "FindExpiredUnclaimed", "error", err)
		return nil, err
	}
	sessions := make([]guestsession.GuestSession, len(models))
	for i, m := range models {
		sessions[i] = toGuestSessionEntity(&m)
	}
	return sessions, nil
}

// DeleteBySessionID removes a guest session from the database.
func (r *GuestSessionRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	err := r.db.WithContext(ctx).
		Delete(&GuestSessionModel{}, "session_id = ?", sessionID).Error
	if err != nil {
		r.log.Error("query failed", "op", "DeleteBySessionID", "error", err)
	}
	return err
}

func toGuestSessionModel(s *guestsession.GuestSession) GuestSessionModel {
	return GuestSessionModel{
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

func toGuestSessionEntity(m *GuestSessionModel) guestsession.GuestSession {
	return guestsession.GuestSession{
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
