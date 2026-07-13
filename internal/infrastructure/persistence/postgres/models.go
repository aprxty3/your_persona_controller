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
	UserID            string    `gorm:"type:uuid;not null;index;uniqueIndex:uniq_active_deletion_per_user,where:status = 'pending_grace' OR status = 'processing'"`
	NotificationEmail string    `gorm:"not null"` // snapshot before anonymization
	Status            string    `gorm:"not null"` // pending_grace | processing | completed | cancelled
	RequestedAt       time.Time `gorm:"not null"`
	CompletedAt       *time.Time
}

func (DataDeletionRequestModel) TableName() string { return "data_deletion_requests" }

// QuestionModel represents GORM model for questions table.
type QuestionModel struct {
	ID               string `gorm:"primaryKey;type:uuid"`
	Section          string `gorm:"type:varchar(10);not null"`
	Type             string `gorm:"type:varchar(20);not null"`
	IsReverseScored  bool   `gorm:"not null;default:false"`
	IsAttentionCheck bool   `gorm:"not null;default:false"`
	DisplayOrder     int    `gorm:"not null"`
}

func (QuestionModel) TableName() string { return "questions" }

// QuestionTranslationModel represents GORM model for question_translations table.
type QuestionTranslationModel struct {
	ID           string  `gorm:"primaryKey;type:uuid"`
	QuestionID   string  `gorm:"type:uuid;not null;index:idx_qt_quest_locale,unique"`
	Locale       string  `gorm:"type:varchar(10);not null;index:idx_qt_quest_locale,unique"`
	QuestionText string  `gorm:"type:text;not null"`
	Options      *string `gorm:"type:jsonb"`
}

func (QuestionTranslationModel) TableName() string { return "question_translations" }

// AnswerModel represents GORM model for answers table.
type AnswerModel struct {
	ID           string    `gorm:"primaryKey;type:uuid"`
	TestResultID string    `gorm:"type:uuid;not null;index:idx_ans_test_quest,unique"`
	QuestionID   string    `gorm:"type:uuid;not null;index:idx_ans_test_quest,unique"`
	Value        string    `gorm:"type:text;not null"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

func (AnswerModel) TableName() string { return "answers" }

// InsightTemplateModel represents GORM model for insight_templates table.
type InsightTemplateModel struct {
	ID             string   `gorm:"primaryKey;type:uuid"`
	InsightKey     string   `gorm:"type:varchar(100);not null;index:idx_it_key_locale,unique"`
	Locale         string   `gorm:"type:varchar(10);not null;index:idx_it_key_locale,unique"`
	Trait          string   `gorm:"type:varchar(10);not null"`
	ConditionType  string   `gorm:"type:varchar(20);not null"`
	MinDelta       *float64 `gorm:"type:numeric"`
	ThresholdValue *float64 `gorm:"type:numeric"`
	TemplateText   string   `gorm:"type:text;not null"`
	IsActive       bool     `gorm:"not null;default:true"`
}

func (InsightTemplateModel) TableName() string { return "insight_templates" }

// PromptAuditLogModel represents GORM model for prompt_audit_logs table.
type PromptAuditLogModel struct {
	ID             string    `gorm:"primaryKey;type:uuid"`
	TestResultID   string    `gorm:"type:uuid;not null;index"`
	RawPrompt      string    `gorm:"type:text;not null"`
	RawResponse    string    `gorm:"type:text;not null"`
	FlaggedAnomaly bool      `gorm:"not null;default:false"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	ExpiresAt      time.Time `gorm:"not null;index"`
}

func (PromptAuditLogModel) TableName() string { return "prompt_audit_logs" }

