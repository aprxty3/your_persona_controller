package referral

import (
	"context"
)

// Repository defines the contract for ReferralCode data persistence.
type Repository interface {
	CreateCode(ctx context.Context, code *ReferralCode) error
	FindCodeByUserID(ctx context.Context, userID string) (*ReferralCode, error)
	FindCodeByCode(ctx context.Context, code string) (*ReferralCode, error)
	CreateEvent(ctx context.Context, event *ReferralEvent) error
	CountEventsByCodeID(ctx context.Context, referralCodeID string, eventType ReferralEventType) (int64, error)
}
