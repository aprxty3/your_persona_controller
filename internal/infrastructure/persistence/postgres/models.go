package postgres

import (
	"time"

	"gorm.io/gorm"
)

// UserModel represents the database schema for users.
type UserModel struct {
	ID               string  `gorm:"primaryKey;type:uuid"`
	Email            string  `gorm:"uniqueIndex;not null"`
	PasswordHash     string  `gorm:"not null"`
	DisplayName      string  `gorm:"not null;default:''"`
	Age              int     `gorm:"not null;default:0"`
	Status           string  `gorm:"not null;default:''"`
	ReferredByCode   *string `gorm:"index"`
	PreferredLocale  string  `gorm:"not null;default:'en'"`
	EmailVerifiedAt  *time.Time
	CreatedAt        time.Time      `gorm:"autoCreateTime"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	AnonymizedAt     *time.Time
	TokenVersion     int `gorm:"not null;default:0"`
	FailedLoginCount int `gorm:"not null;default:0"`
	LockedUntil      *time.Time
}

func (UserModel) TableName() string { return "users" }

// GuestSessionModel represents the database schema for guest sessions.
type GuestSessionModel struct {
	SessionID       string    `gorm:"primaryKey;type:uuid"`
	IPHash          string    `gorm:"not null"`
	DisplayName     string    `gorm:"not null"`
	Age             int       `gorm:"not null"`
	Status          string    `gorm:"not null"`
	Locale          string    `gorm:"not null;default:'en'"`
	ClaimedByUserID *string   `gorm:"type:uuid;index"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	ExpiresAt       time.Time `gorm:"index"`
}

func (GuestSessionModel) TableName() string { return "guest_sessions" }

// TestResultModel represents the database schema for assessment results.
type TestResultModel struct {
	ID               string  `gorm:"primaryKey;type:uuid"`
	UserID           *string `gorm:"type:uuid;index"`
	GuestSessionID   *string `gorm:"type:uuid;index"`
	ShareToken       string  `gorm:"type:varchar(50);uniqueIndex;not null"`
	Locale           string  `gorm:"not null"`
	MascotStyle      string  `gorm:"not null;default:'style_a'"`
	MBTIType         string  `gorm:"type:varchar(4)"`
	GritScore        int
	TraitScores      string  `gorm:"type:jsonb"`
	AISummaryText    *string `gorm:"type:text"`
	Status           string  `gorm:"not null"`
	WellbeingFlag    bool    `gorm:"default:false"`
	PDFUrl           *string
	PDFStatus        string `gorm:"not null;default:'pending'"`
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	CreatedAt        time.Time  `gorm:"autoCreateTime;index"`
	ExpiresAt        *time.Time `gorm:"index"`
}

func (TestResultModel) TableName() string { return "test_results" }

// VerificationTokenModel represents the database schema for OTP tokens.
// Composite index on (user_id, type) because lookup is ALWAYS scoped to both —
// a 6-digit OTP is not globally unique across users.
type VerificationTokenModel struct {
	ID           string    `gorm:"primaryKey;type:uuid"`
	UserID       string    `gorm:"type:uuid;not null;index:idx_vt_user_type"`
	Token        string    `gorm:"not null"`
	Type         string    `gorm:"not null;index:idx_vt_user_type"` // email_verification | password_reset
	AttemptCount int       `gorm:"not null;default:0"`
	ExpiresAt    time.Time `gorm:"not null"`
	UsedAt       *time.Time
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

func (VerificationTokenModel) TableName() string { return "verification_tokens" }

// ReferralCodeModel — one code per user.
type ReferralCodeModel struct {
	ID        string    `gorm:"primaryKey;type:uuid"`
	UserID    string    `gorm:"type:uuid;not null;uniqueIndex"`
	Code      string    `gorm:"not null;uniqueIndex"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (ReferralCodeModel) TableName() string { return "referral_codes" }

// ReferralEventModel records conversion events triggered by a referral code.
type ReferralEventModel struct {
	ID             string    `gorm:"primaryKey;type:uuid"`
	ReferralCodeID string    `gorm:"type:uuid;not null;index"`
	ReferredUserID string    `gorm:"type:uuid;not null;index"`
	EventType      string    `gorm:"not null"` // signup | test_completed
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

func (ReferralEventModel) TableName() string { return "referral_events" }

// DataDeletionRequestModel records formal deletion requests with grace period.
type DataDeletionRequestModel struct {
	ID                string    `gorm:"primaryKey;type:uuid"`
	UserID            string    `gorm:"type:uuid;not null;index"`
	NotificationEmail string    `gorm:"not null"` // snapshot before anonymization
	Status            string    `gorm:"not null"` // pending_grace | processing | completed | cancelled
	RequestedAt       time.Time `gorm:"not null"`
	CompletedAt       *time.Time
}

func (DataDeletionRequestModel) TableName() string { return "data_deletion_requests" }
