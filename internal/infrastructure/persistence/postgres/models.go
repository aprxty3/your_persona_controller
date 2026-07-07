package postgres

import (
	"time"

	"gorm.io/gorm"
)

// UserModel represents the database schema for users.
type UserModel struct {
	ID              string  `gorm:"primaryKey;type:uuid"` // Populated with UUIDv7 at the app level
	Email           string  `gorm:"uniqueIndex;not null"`
	PasswordHash    string  `gorm:"not null"`
	DisplayName     string  `gorm:"not null"`
	Age             int     `gorm:"not null"`
	Status          string  `gorm:"not null"`
	ReferredByCode  *string `gorm:"index"`
	PreferredLocale string  `gorm:"default:'en'"`
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time      `gorm:"autoCreateTime"`
	DeletedAt       gorm.DeletedAt `gorm:"index"`
	AnonymizedAt    *time.Time
	TokenVersion    int `gorm:"default:0"`
}

// TableName overrides the default table name.
func (UserModel) TableName() string {
	return "users"
}

// GuestSessionModel represents the database schema for guest sessions.
type GuestSessionModel struct {
	SessionID       string    `gorm:"primaryKey;type:uuid"` // Usually UUIDv4
	IPHash          string    `gorm:"not null"`
	DisplayName     string    `gorm:"not null"`
	Age             int       `gorm:"not null"`
	Status          string    `gorm:"not null"`
	ClaimedByUserID *string   `gorm:"type:uuid;index"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	ExpiresAt       time.Time `gorm:"index"`
}

func (GuestSessionModel) TableName() string {
	return "guest_sessions"
}

// TestResultModel represents the database schema for assessment results.
type TestResultModel struct {
	ID               string  `gorm:"primaryKey;type:uuid"` // Populated with UUIDv7
	UserID           *string `gorm:"type:uuid;index"`
	GuestSessionID   *string `gorm:"type:uuid;index"`
	ShareToken       string  `gorm:"type:varchar(50);uniqueIndex;not null"` // Secure public access
	Locale           string  `gorm:"not null"`
	MascotStyle      string  `gorm:"not null"`
	MBTIType         string  `gorm:"type:varchar(4)"`
	GritScore        int
	TraitScores      string  `gorm:"type:jsonb"` // Stored as JSON string
	AISummaryText    *string `gorm:"type:text"`
	Status           string  `gorm:"not null"`
	WellbeingFlag    bool    `gorm:"default:false"`
	PDFURL           *string
	PDFStatus        string `gorm:"default:'pending'"`
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	CreatedAt        time.Time  `gorm:"autoCreateTime;index"`
	ExpiresAt        *time.Time `gorm:"index"`
}

func (TestResultModel) TableName() string {
	return "test_results"
}
