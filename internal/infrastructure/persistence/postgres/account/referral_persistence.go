package account

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// ReferralRepository implements account.ReferralRepository backed by PostgreSQL via GORM.
type ReferralRepository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewReferralRepository constructs a ReferralRepository.
func NewReferralRepository(db *gorm.DB, log logger.Logger) account.ReferralRepository {
	return &ReferralRepository{db: db, log: log.With("repository", "referral")}
}

// CreateCode creates a new referral code for a user.
func (r *ReferralRepository) CreateCode(ctx context.Context, code *account.ReferralCode) error {
	m := postgres.ReferralCodeModel{
		ID:        code.ID,
		UserID:    code.UserID,
		Code:      code.Code,
		CreatedAt: code.CreatedAt,
	}
	return postgres.LogQueryError(r.log, "CreateCode", r.db.WithContext(ctx).Create(&m).Error)
}

// FindCodeByUserID returns the referral code owned by the given user, or nil if none.
func (r *ReferralRepository) FindCodeByUserID(ctx context.Context, userID string) (*account.ReferralCode, error) {
	var m postgres.ReferralCodeModel
	err := r.db.WithContext(ctx).First(&m, "user_id = ?", userID).Error
	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindCodeByUserID", err); err != nil {
		return nil, err
	}
	return &account.ReferralCode{
		ID:        m.ID,
		UserID:    m.UserID,
		Code:      m.Code,
		CreatedAt: m.CreatedAt,
	}, nil
}

// FindCodeByCode looks up a referral code by its alphanumeric string value.
func (r *ReferralRepository) FindCodeByCode(ctx context.Context, code string) (*account.ReferralCode, error) {
	var m postgres.ReferralCodeModel
	err := r.db.WithContext(ctx).First(&m, "code = ?", code).Error
	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindCodeByCode", err); err != nil {
		return nil, err
	}
	return &account.ReferralCode{
		ID:        m.ID,
		UserID:    m.UserID,
		Code:      m.Code,
		CreatedAt: m.CreatedAt,
	}, nil
}

// CreateEvent records a new referral event (signup or test_completed).
func (r *ReferralRepository) CreateEvent(ctx context.Context, event *account.ReferralEvent) error {
	m := postgres.ReferralEventModel{
		ID:             event.ID,
		ReferralCodeID: event.ReferralCodeID,
		ReferredUserID: event.ReferredUserID,
		EventType:      string(event.EventType),
		CreatedAt:      event.CreatedAt,
	}
	return postgres.LogQueryError(r.log, "CreateEvent", r.db.WithContext(ctx).Create(&m).Error)
}

// CountEventsByCodeID counts total referral events for reporting.
func (r *ReferralRepository) CountEventsByCodeID(ctx context.Context, referralCodeID string, eventType account.ReferralEventType) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&postgres.ReferralEventModel{}).
		Where("referral_code_id = ? AND event_type = ?", referralCodeID, string(eventType)).
		Count(&count).Error
	if err := postgres.LogQueryError(r.log, "CountEventsByCodeID", err); err != nil {
		return 0, err
	}
	return count, nil
}
