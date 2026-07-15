package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
)

type mockUserRepo struct {
	byID    map[string]*account.User
	byEmail map[string]*account.User
}

func newMockUserRepo(users ...*account.User) *mockUserRepo {
	m := &mockUserRepo{byID: map[string]*account.User{}, byEmail: map[string]*account.User{}}
	for _, u := range users {
		m.byID[u.ID] = u
		m.byEmail[u.Email] = u
	}
	return m
}

func (m *mockUserRepo) Create(ctx context.Context, u *account.User) error { return nil }
func (m *mockUserRepo) FindByID(ctx context.Context, id string) (*account.User, error) {
	return m.byID[id], nil
}
func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*account.User, error) {
	return m.byEmail[email], nil
}
func (m *mockUserRepo) Update(ctx context.Context, u *account.User) error          { return nil }
func (m *mockUserRepo) IncrementTokenVersion(ctx context.Context, id string) error { return nil }
func (m *mockUserRepo) UpdateLoginAttempt(ctx context.Context, id string, failedCount int, lockedUntil *time.Time) error {
	return nil
}
func (m *mockUserRepo) Anonymize(ctx context.Context, id string, scrubbedEmail string) error {
	return nil
}

type mockSessionTokenStore struct {
	consumeResetJTIFn    func(ctx context.Context, jti string) (string, error)
	consumeResetJTICalls int

	denylisted    bool
	denylistedErr error
}

func (m *mockSessionTokenStore) StoreResetJTI(ctx context.Context, jti, userID string, ttl time.Duration) error {
	return nil
}

func (m *mockSessionTokenStore) ConsumeResetJTI(ctx context.Context, jti string) (string, error) {
	m.consumeResetJTICalls++
	if m.consumeResetJTIFn != nil {
		return m.consumeResetJTIFn(ctx, jti)
	}
	return "", nil
}

func (m *mockSessionTokenStore) DenylistRefreshJTI(ctx context.Context, jti string, ttl time.Duration) error {
	return nil
}

func (m *mockSessionTokenStore) IsRefreshJTIDenylisted(ctx context.Context, jti string) (bool, error) {
	return m.denylisted, m.denylistedErr
}

const validNewPassword = "Str0ngPassw0rd!"

func TestResetPassword_JTIAlreadyConsumed_RejectsAsInvalidToken(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	_, resetToken, err := jwtService.GenerateResetToken("user-1", ResetTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint reset token fixture: %v", err)
	}

	tokenStore := &mockSessionTokenStore{consumeResetJTIFn: func(ctx context.Context, jti string) (string, error) {
		return "", nil // simulates an already-consumed jti (GETDEL found nothing)
	}}
	uc := &SessionUseCase{
		jwtService:    jwtService,
		tokenStore:    tokenStore,
		breachChecker: NewNoopBreachChecker(),
		log:           testLogger(),
	}

	_, err = uc.ResetPassword(context.Background(), ResetPasswordRequest{ResetToken: resetToken, NewPassword: validNewPassword})
	if !errors.Is(err, application.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken on replay, got %v", err)
	}
	if tokenStore.consumeResetJTICalls != 1 {
		t.Fatalf("expected exactly 1 consume call, got %d", tokenStore.consumeResetJTICalls)
	}
}

func TestResetPassword_ConsumedUserIDMismatch_RejectsAsInvalidToken(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	_, resetToken, err := jwtService.GenerateResetToken("user-1", ResetTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint reset token fixture: %v", err)
	}

	tokenStore := &mockSessionTokenStore{consumeResetJTIFn: func(ctx context.Context, jti string) (string, error) {
		return "someone-else", nil
	}}
	uc := &SessionUseCase{
		jwtService:    jwtService,
		tokenStore:    tokenStore,
		breachChecker: NewNoopBreachChecker(),
		log:           testLogger(),
	}

	_, err = uc.ResetPassword(context.Background(), ResetPasswordRequest{ResetToken: resetToken, NewPassword: validNewPassword})
	if !errors.Is(err, application.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken on subject mismatch, got %v", err)
	}
}

func TestResetPassword_MalformedJWT_RejectsWithoutConsuming(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	tokenStore := &mockSessionTokenStore{}
	uc := &SessionUseCase{
		jwtService:    jwtService,
		tokenStore:    tokenStore,
		breachChecker: NewNoopBreachChecker(),
		log:           testLogger(),
	}

	_, err := uc.ResetPassword(context.Background(), ResetPasswordRequest{ResetToken: "not-a-jwt", NewPassword: validNewPassword})
	if !errors.Is(err, application.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for a malformed JWT, got %v", err)
	}
	if tokenStore.consumeResetJTICalls != 0 {
		t.Fatalf("expected ConsumeResetJTI to never be called for an unparseable token, got %d calls", tokenStore.consumeResetJTICalls)
	}
}

func TestResetPassword_WeakPassword_RejectsBeforeTouchingToken(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	tokenStore := &mockSessionTokenStore{}
	uc := &SessionUseCase{
		jwtService:    jwtService,
		tokenStore:    tokenStore,
		breachChecker: NewNoopBreachChecker(),
		log:           testLogger(),
	}

	_, err := uc.ResetPassword(context.Background(), ResetPasswordRequest{ResetToken: "irrelevant", NewPassword: "short"})
	if err == nil {
		t.Fatal("expected a password policy error")
	}
	if errors.Is(err, application.ErrInvalidToken) {
		t.Fatal("expected a password-policy error, not a token error, for a too-short password")
	}
	if tokenStore.consumeResetJTICalls != 0 {
		t.Fatalf("expected ConsumeResetJTI to never be called when password validation fails first, got %d calls", tokenStore.consumeResetJTICalls)
	}
}

func TestRefreshToken_StaleTokenVersion_Rejected(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	refreshToken, err := jwtService.GenerateRefreshToken("user-1", 1, RefreshTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint refresh token fixture: %v", err)
	}

	uc := &SessionUseCase{
		jwtService: jwtService,
		userRepo:   newMockUserRepo(&account.User{ID: "user-1", TokenVersion: 2}),
		tokenStore: &mockSessionTokenStore{},
		log:        testLogger(),
	}

	_, err = uc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: refreshToken})
	if !errors.Is(err, application.ErrTokenVersionMismatch) {
		t.Fatalf("expected ErrTokenVersionMismatch for a stale token_version, got %v", err)
	}
}

func TestRefreshToken_CurrentTokenVersion_Succeeds(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	refreshToken, err := jwtService.GenerateRefreshToken("user-1", 2, RefreshTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint refresh token fixture: %v", err)
	}

	uc := &SessionUseCase{
		jwtService: jwtService,
		userRepo:   newMockUserRepo(&account.User{ID: "user-1", TokenVersion: 2}),
		tokenStore: &mockSessionTokenStore{},
		log:        testLogger(),
	}

	resp, err := uc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: refreshToken})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("expected a fresh token pair")
	}
}

func TestRefreshToken_DenylistedJTI_Rejected(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	refreshToken, err := jwtService.GenerateRefreshToken("user-1", 1, RefreshTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint refresh token fixture: %v", err)
	}

	uc := &SessionUseCase{
		jwtService: jwtService,
		userRepo:   newMockUserRepo(&account.User{ID: "user-1", TokenVersion: 1}),
		tokenStore: &mockSessionTokenStore{denylisted: true},
		log:        testLogger(),
	}

	_, err = uc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: refreshToken})
	if !errors.Is(err, application.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for a denylisted refresh jti, got %v", err)
	}
}
