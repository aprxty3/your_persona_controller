package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/guestsession"
	"github.com/google/uuid"
)

// CreateGuestSessionRequest holds validated input from the Onboarding form.
type CreateGuestSessionRequest struct {
	DisplayName string
	Age         int
	Status      string // student|college|work|etc
	Locale      string // en|id
	IPAddress   string // raw IP — will be hashed, never stored
}

// CreateGuestSessionResponse is returned to the handler to set the cookie.
type CreateGuestSessionResponse struct {
	SessionID string
	ExpiresAt time.Time
}

// CreateGuestSessionUseCase creates a new GUEST_SESSION record.
type CreateGuestSessionUseCase struct {
	repo guestsession.Repository
}

func NewCreateGuestSessionUseCase(repo guestsession.Repository) *CreateGuestSessionUseCase {
	return &CreateGuestSessionUseCase{repo: repo}
}

func (uc *CreateGuestSessionUseCase) Execute(ctx context.Context, req CreateGuestSessionRequest) (*CreateGuestSessionResponse, error) {
	sessionID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(14 * 24 * time.Hour)

	session := &guestsession.GuestSession{
		SessionID:   sessionID,
		IPHash:      hashIP(req.IPAddress),
		DisplayName: req.DisplayName,
		Age:         req.Age,
		Status:      req.Status,
		Locale:      req.Locale,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	}

	if err := uc.repo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("create_guest_session: %w", err)
	}

	return &CreateGuestSessionResponse{
		SessionID: sessionID,
		ExpiresAt: expiresAt,
	}, nil
}

// hashIP produces a one-way hash of the raw IP address.
// Raw IPs are never stored — only the hash, for deduplication only.
func hashIP(ip string) string {
	h := sha256.Sum256([]byte(ip))
	return fmt.Sprintf("%x", h)
}
