package postgres

import (
	"context"
	"errors"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/referral"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// ReferralRepository implements referral.Repository backed by PostgreSQL via GORM.
type ReferralRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewReferralRepository constructs a ReferralRepository.
func NewReferralRepository(db *gorm.DB, log logger.Logger) referral.Repository {
	return &ReferralRepository{db: db, log: log.With("repository", "referral")}
}

// CreateCode creates a new referral code for a user.
func (r *ReferralRepository) CreateCode(ctx context.Context, code *referral.ReferralCode) error {
	m := ReferralCodeModel{
		ID:        code.ID,
		UserID:    code.UserID,
		Code:      code.Code,
		CreatedAt: code.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		r.log.Error("query failed", "op", "CreateCode", "error", err)
		return err
	}
	return nil
}

// FindCodeByUserID returns the referral code owned by the given user, or nil if none.
func (r *ReferralRepository) FindCodeByUserID(ctx context.Context, userID string) (*referral.ReferralCode, error) {
	var m ReferralCodeModel
	err := r.db.WithContext(ctx).First(&m, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindCodeByUserID", "error", err)
		return nil, err
	}
	return &referral.ReferralCode{
		ID:        m.ID,
		UserID:    m.UserID,
		Code:      m.Code,
		CreatedAt: m.CreatedAt,
	}, nil
}

// FindCodeByCode looks up a referral code by its alphanumeric string value.
func (r *ReferralRepository) FindCodeByCode(ctx context.Context, code string) (*referral.ReferralCode, error) {
	var m ReferralCodeModel
	err := r.db.WithContext(ctx).First(&m, "code = ?", code).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindCodeByCode", "error", err)
		return nil, err
	}
	return &referral.ReferralCode{
		ID:        m.ID,
		UserID:    m.UserID,
		Code:      m.Code,
		CreatedAt: m.CreatedAt,
	}, nil
}

// CreateEvent records a new referral event (signup or test_completed).
func (r *ReferralRepository) CreateEvent(ctx context.Context, event *referral.ReferralEvent) error {
	m := ReferralEventModel{
		ID:             event.ID,
		ReferralCodeID: event.ReferralCodeID,
		ReferredUserID: event.ReferredUserID,
		EventType:      string(event.EventType),
		CreatedAt:      event.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		r.log.Error("query failed", "op", "CreateEvent", "error", err)
		return err
	}
	return nil
}

// CountEventsByCodeID counts total referral events for reporting.
func (r *ReferralRepository) CountEventsByCodeID(ctx context.Context, referralCodeID string, eventType referral.ReferralEventType) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ReferralEventModel{}).
		Where("referral_code_id = ? AND event_type = ?", referralCodeID, string(eventType)).
		Count(&count).Error
	if err != nil {
		r.log.Error("query failed", "op", "CountEventsByCodeID", "error", err)
		return 0, err
	}
	return count, nil
}
