package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/auth/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

const validNewPassword = "Str0ngPassw0rd!"

func newHash(t *testing.T) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt hash fixture: %v", err)
	}
	return string(hash)
}

// --- ResetPassword: single-use jti + subject-matching, all pre-transaction ---

func TestResetPassword_JTIAlreadyConsumed_RejectsAsInvalidToken(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	_, resetToken, err := jwtService.GenerateResetToken("user-1", ResetTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint reset token fixture: %v", err)
	}

	tokenStore := mocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().ConsumeResetJTI(mock.Anything, mock.Anything).Return("", nil).Once() // already-consumed jti (GETDEL found nothing)
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
}

func TestResetPassword_ConsumedUserIDMismatch_RejectsAsInvalidToken(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	_, resetToken, err := jwtService.GenerateResetToken("user-1", ResetTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint reset token fixture: %v", err)
	}

	tokenStore := mocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().ConsumeResetJTI(mock.Anything, mock.Anything).Return("someone-else", nil).Once()
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
	tokenStore := mocks.NewMockSessionTokenStore(t) // no EXPECT() set: any call panics the test
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
}

func TestResetPassword_WeakPassword_RejectsBeforeTouchingToken(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	tokenStore := mocks.NewMockSessionTokenStore(t) // no EXPECT() set: ConsumeResetJTI must never be called
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
}

// --- RefreshToken: token_version invalidation + denylist, fully mockable ---

func TestRefreshToken_StaleTokenVersion_Rejected(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	refreshToken, err := jwtService.GenerateRefreshToken("user-1", 1, RefreshTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint refresh token fixture: %v", err)
	}

	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", TokenVersion: 2}, nil).Once()
	tokenStore := mocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().IsRefreshJTIDenylisted(mock.Anything, mock.Anything).Return(false, nil).Once()

	uc := &SessionUseCase{jwtService: jwtService, userRepo: userRepo, tokenStore: tokenStore, log: testLogger()}

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

	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", TokenVersion: 2}, nil).Once()
	tokenStore := mocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().IsRefreshJTIDenylisted(mock.Anything, mock.Anything).Return(false, nil).Once()
	tokenStore.EXPECT().DenylistRefreshJTI(mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	uc := &SessionUseCase{jwtService: jwtService, userRepo: userRepo, tokenStore: tokenStore, log: testLogger()}

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

	tokenStore := mocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().IsRefreshJTIDenylisted(mock.Anything, mock.Anything).Return(true, nil).Once()
	uc := &SessionUseCase{jwtService: jwtService, tokenStore: tokenStore, log: testLogger()}

	_, err = uc.RefreshToken(context.Background(), RefreshTokenRequest{RefreshToken: refreshToken})
	if !errors.Is(err, application.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for a denylisted refresh jti, got %v", err)
	}
}

// --- Login ---

func TestLogin_UnknownEmail_RejectsWithInvalidCredentials(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "nobody@example.com").Return(nil, nil).Once()
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, log: testLogger()}

	_, err := uc.Login(context.Background(), LoginRequest{Email: "nobody@example.com", Password: "whatever"})
	if !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_RateLimited_RejectsBeforeLookup(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t) // no EXPECT(): FindByEmail must never be called
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(false, 30, nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, log: testLogger()}

	_, err := uc.Login(context.Background(), LoginRequest{Email: "a@example.com", Password: "whatever"})
	if !errors.Is(err, application.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestLogin_AccountLocked_Rejects(t *testing.T) {
	future := time.Now().Add(10 * time.Minute)
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{ID: "user-1", Email: "a@example.com", LockedUntil: &future}, nil).Once()
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, log: testLogger()}

	_, err := uc.Login(context.Background(), LoginRequest{Email: "a@example.com", Password: "whatever"})
	if !errors.Is(err, application.ErrAccountLocked) {
		t.Fatalf("expected ErrAccountLocked, got %v", err)
	}
}

func TestLogin_EmailNotVerified_Rejects(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{ID: "user-1", Email: "a@example.com"}, nil).Once()
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, log: testLogger()}

	_, err := uc.Login(context.Background(), LoginRequest{Email: "a@example.com", Password: "whatever"})
	if !errors.Is(err, application.ErrEmailNotVerified) {
		t.Fatalf("expected ErrEmailNotVerified, got %v", err)
	}
}

func TestLogin_WrongPassword_IncrementsFailedCountAndRejects(t *testing.T) {
	now := time.Now()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{
		ID: "user-1", Email: "a@example.com", PasswordHash: newHash(t), EmailVerifiedAt: &now,
	}, nil).Once()
	userRepo.EXPECT().UpdateLoginAttempt(mock.Anything, "user-1", 1, (*time.Time)(nil)).Return(nil).Once()
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, loginMaxAttempts: 10, lockDuration: 15 * time.Minute, log: testLogger()}

	_, err := uc.Login(context.Background(), LoginRequest{Email: "a@example.com", Password: "wrong-password"})
	if !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_WrongPassword_LocksAccountAtMaxAttempts(t *testing.T) {
	now := time.Now()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{
		ID: "user-1", Email: "a@example.com", PasswordHash: newHash(t), EmailVerifiedAt: &now, FailedLoginCount: 2,
	}, nil).Once()
	userRepo.EXPECT().UpdateLoginAttempt(mock.Anything, "user-1", 3, mock.AnythingOfType("*time.Time")).Return(nil).Once()
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, loginMaxAttempts: 3, lockDuration: 15 * time.Minute, log: testLogger()}

	_, err := uc.Login(context.Background(), LoginRequest{Email: "a@example.com", Password: "wrong-password"})
	if !errors.Is(err, application.ErrAccountLocked) {
		t.Fatalf("expected ErrAccountLocked once failed count hits the max, got %v", err)
	}
}

func TestLogin_CorrectPassword_Succeeds(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	now := time.Now()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{
		ID: "user-1", Email: "a@example.com", PasswordHash: newHash(t), EmailVerifiedAt: &now, TokenVersion: 1,
	}, nil).Once()
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, jwtService: jwtService, log: testLogger()}

	resp, err := uc.Login(context.Background(), LoginRequest{Email: "a@example.com", Password: "correct-password"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("expected a token pair on successful login")
	}
}

func TestLogin_CorrectPassword_ResetsFailedCountWhenNonZero(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	now := time.Now()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{
		ID: "user-1", Email: "a@example.com", PasswordHash: newHash(t), EmailVerifiedAt: &now, FailedLoginCount: 2,
	}, nil).Once()
	userRepo.EXPECT().UpdateLoginAttempt(mock.Anything, "user-1", 0, (*time.Time)(nil)).Return(nil).Once()
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, jwtService: jwtService, log: testLogger()}

	if _, err := uc.Login(context.Background(), LoginRequest{Email: "a@example.com", Password: "correct-password"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- VerifyEmailOTP ---

func TestVerifyEmailOTP_CorrectCode_ActivatesAndAutoLogsIn(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	user := &account.User{ID: "user-1", Email: "a@example.com", TokenVersion: 0}
	token := &account.VerificationToken{ID: "token-1", UserID: "user-1", Token: "123456", Type: account.TokenTypeEmailVerification, ExpiresAt: time.Now().Add(15 * time.Minute)}

	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(user, nil).Once()
	userRepo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().FindActiveByUserAndType(mock.Anything, "user-1", account.TokenTypeEmailVerification).Return(token, nil).Once()
	tokenRepo.EXPECT().MarkUsed(mock.Anything, "token-1").Return(nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, tokenRepo: tokenRepo, jwtService: jwtService, log: testLogger()}

	resp, err := uc.VerifyEmailOTP(context.Background(), VerifyEmailOTPRequest{Email: "a@example.com", OTP: "123456"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("expected auto-login token pair")
	}
	if user.EmailVerifiedAt == nil {
		t.Fatal("expected EmailVerifiedAt to be set")
	}
}

func TestVerifyEmailOTP_UnknownEmail_RejectsAsInvalidOTP(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "nobody@example.com").Return(nil, nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, log: testLogger()}

	_, err := uc.VerifyEmailOTP(context.Background(), VerifyEmailOTPRequest{Email: "nobody@example.com", OTP: "123456"})
	if !errors.Is(err, application.ErrInvalidOTP) {
		t.Fatalf("expected ErrInvalidOTP, got %v", err)
	}
}

// --- LogoutAll / Logout ---

func TestLogoutAll_IncrementsTokenVersion(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().IncrementTokenVersion(mock.Anything, "user-1").Return(nil).Once()

	uc := &SessionUseCase{userRepo: userRepo, log: testLogger()}

	if err := uc.LogoutAll(context.Background(), "user-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogoutAll_RepoError_Propagates(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().IncrementTokenVersion(mock.Anything, "user-1").Return(errors.New("db down")).Once()

	uc := &SessionUseCase{userRepo: userRepo, log: testLogger()}

	if err := uc.LogoutAll(context.Background(), "user-1"); err == nil {
		t.Fatal("expected the repository error to propagate")
	}
}

func TestLogout_SubjectMismatch_Rejected(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	refreshToken, err := jwtService.GenerateRefreshToken("user-1", 1, RefreshTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint refresh token fixture: %v", err)
	}

	uc := &SessionUseCase{jwtService: jwtService, log: testLogger()}

	err = uc.Logout(context.Background(), LogoutRequest{UserID: "someone-else", RefreshToken: refreshToken})
	if !errors.Is(err, application.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken on subject mismatch, got %v", err)
	}
}

func TestLogout_ValidToken_DenylistsAndSucceeds(t *testing.T) {
	jwtService := jwtservice.NewJWTService("test-secret")
	refreshToken, err := jwtService.GenerateRefreshToken("user-1", 1, RefreshTokenTTL)
	if err != nil {
		t.Fatalf("failed to mint refresh token fixture: %v", err)
	}

	tokenStore := mocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().DenylistRefreshJTI(mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	uc := &SessionUseCase{jwtService: jwtService, tokenStore: tokenStore, log: testLogger()}

	if err := uc.Logout(context.Background(), LogoutRequest{UserID: "user-1", RefreshToken: refreshToken}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogout_AlreadyInvalidToken_NoOp(t *testing.T) {
	uc := &SessionUseCase{jwtService: jwtservice.NewJWTService("test-secret"), log: testLogger()} // tokenStore nil: DenylistRefreshJTI must never be called

	if err := uc.Logout(context.Background(), LogoutRequest{UserID: "user-1", RefreshToken: "not-a-jwt"}); err != nil {
		t.Fatalf("expected a no-op success for an already-invalid token, got %v", err)
	}
}
