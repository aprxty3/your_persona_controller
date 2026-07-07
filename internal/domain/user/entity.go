package user

import (
	"context"
	"time"
)

// User represents the core business entity for a registered member.
type User struct {
	ID              string
	Email           string
	PasswordHash    string
	DisplayName     string
	Age             int
	Status          string
	ReferredByCode  *string
	PreferredLocale string
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	DeletedAt       *time.Time
	AnonymizedAt    *time.Time
	TokenVersion    int
}

// Repository defines the contract for User data persistence.
type Repository interface {
	Create(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
}
