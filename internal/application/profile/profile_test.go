package profile

import (
	"context"
	"errors"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// --- UpdateProfile ---

func TestUpdateProfile_UserNotFound_Errors(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(nil, nil).Once()
	uc := NewProfileUseCase(userRepo, nil, testLogger())

	_, err := uc.UpdateProfile(context.Background(), UpdateProfileRequest{UserID: "user-1"})
	if err == nil {
		t.Fatal("expected an error when the authenticated user can't be found")
	}
}

func TestUpdateProfile_PartialUpdate_OnlyChangesSuppliedFields(t *testing.T) {
	user := &account.User{ID: "user-1", DisplayName: "Old", Age: 20, Status: "student", PreferredLocale: "en"}
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(user, nil).Once()
	userRepo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(u *account.User) bool {
		return u.DisplayName == "New Name" && u.Age == 20 && u.Status == "student" && u.PreferredLocale == "en"
	})).Return(nil).Once()
	uc := NewProfileUseCase(userRepo, nil, testLogger())

	resp, err := uc.UpdateProfile(context.Background(), UpdateProfileRequest{UserID: "user-1", DisplayName: strPtr("New Name")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.DisplayName != "New Name" || resp.Age != 20 {
		t.Fatalf("expected only display_name to change, got %+v", resp)
	}
}

func TestUpdateProfile_InvalidAge_Rejected(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1"}, nil).Once()
	uc := NewProfileUseCase(userRepo, nil, testLogger())

	_, err := uc.UpdateProfile(context.Background(), UpdateProfileRequest{UserID: "user-1", Age: intPtr(5)})
	if err == nil {
		t.Fatal("expected a validation error for an under-age update")
	}
}

func TestUpdateProfile_InvalidStatus_Rejected(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1"}, nil).Once()
	uc := NewProfileUseCase(userRepo, nil, testLogger())

	_, err := uc.UpdateProfile(context.Background(), UpdateProfileRequest{UserID: "user-1", Status: strPtr("not-a-status")})
	if err == nil {
		t.Fatal("expected a validation error for an invalid status")
	}
}

func TestUpdateProfile_InvalidLocale_Rejected(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1"}, nil).Once()
	uc := NewProfileUseCase(userRepo, nil, testLogger())

	_, err := uc.UpdateProfile(context.Background(), UpdateProfileRequest{UserID: "user-1", PreferredLocale: strPtr("fr")})
	if err == nil {
		t.Fatal("expected a validation error for an unsupported locale")
	}
}

// --- GetReferralCode ---

func TestGetReferralCode_Existing_ReturnedWithoutGenerating(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(&account.ReferralCode{Code: "ABCD1234"}, nil).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	resp, err := uc.GetReferralCode(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != "ABCD1234" {
		t.Fatalf("expected the existing code to be returned, got %q", resp.Code)
	}
}

func TestGetReferralCode_NoneExisting_GeneratesAndPersists(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(nil, nil).Once()
	referralRepo.EXPECT().FindCodeByCode(mock.Anything, mock.Anything).Return(nil, nil).Once()
	referralRepo.EXPECT().CreateCode(mock.Anything, mock.Anything).Return(nil).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	resp, err := uc.GetReferralCode(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Code) != referralCodeLength {
		t.Fatalf("expected an %d-char generated code, got %q", referralCodeLength, resp.Code)
	}
}

// A collision on the first generated code must retry with a new one rather
// than failing outright.
func TestGetReferralCode_CollisionRetries(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(nil, nil).Once()
	referralRepo.EXPECT().FindCodeByCode(mock.Anything, mock.Anything).Return(&account.ReferralCode{Code: "TAKEN"}, nil).Once()
	referralRepo.EXPECT().FindCodeByCode(mock.Anything, mock.Anything).Return(nil, nil).Once()
	referralRepo.EXPECT().CreateCode(mock.Anything, mock.Anything).Return(nil).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	if _, err := uc.GetReferralCode(context.Background(), "user-1"); err != nil {
		t.Fatalf("unexpected error after a single collision retry: %v", err)
	}
}

// A duplicate-key race on Create (two concurrent first-requests) must
// re-fetch and return the winner's code instead of erroring.
func TestGetReferralCode_CreateRace_ReturnsWinnersCode(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(nil, nil).Once()
	referralRepo.EXPECT().FindCodeByCode(mock.Anything, mock.Anything).Return(nil, nil).Once()
	referralRepo.EXPECT().CreateCode(mock.Anything, mock.Anything).Return(gorm.ErrDuplicatedKey).Once()
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(&account.ReferralCode{Code: "WINNER01"}, nil).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	resp, err := uc.GetReferralCode(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != "WINNER01" {
		t.Fatalf("expected the concurrent winner's code, got %q", resp.Code)
	}
}

func TestGetReferralCode_LookupError_Propagates(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(nil, errors.New("db down")).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	if _, err := uc.GetReferralCode(context.Background(), "user-1"); err == nil {
		t.Fatal("expected the repository error to propagate")
	}
}

// --- GetReferralStats (TICKET-25) ---

func TestGetReferralStats_NoCodeYet_ReturnsZeroCountsNotError(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t) // no CountEventsByCodeID EXPECT(): must never be called
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(nil, nil).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	resp, err := uc.GetReferralStats(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != "" || resp.SignupCount != 0 || resp.CompletedCount != 0 {
		t.Fatalf("expected zero-value stats for a user with no code, got %+v", resp)
	}
}

func TestGetReferralStats_ExistingCode_ReturnsBothCounts(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(&account.ReferralCode{ID: "code-1", Code: "ABCD1234"}, nil).Once()
	referralRepo.EXPECT().CountEventsByCodeID(mock.Anything, "code-1", account.EventTypeSignup).Return(int64(3), nil).Once()
	referralRepo.EXPECT().CountEventsByCodeID(mock.Anything, "code-1", account.EventTypeTestCompleted).Return(int64(1), nil).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	resp, err := uc.GetReferralStats(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Code != "ABCD1234" || resp.SignupCount != 3 || resp.CompletedCount != 1 {
		t.Fatalf("expected code=ABCD1234 signup=3 completed=1, got %+v", resp)
	}
}

func TestGetReferralStats_CodeLookupError_Propagates(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(nil, errors.New("db down")).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	if _, err := uc.GetReferralStats(context.Background(), "user-1"); err == nil {
		t.Fatal("expected the code lookup error to propagate")
	}
}

func TestGetReferralStats_SignupCountError_Propagates(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(&account.ReferralCode{ID: "code-1", Code: "ABCD1234"}, nil).Once()
	referralRepo.EXPECT().CountEventsByCodeID(mock.Anything, "code-1", account.EventTypeSignup).Return(int64(0), errors.New("db down")).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	if _, err := uc.GetReferralStats(context.Background(), "user-1"); err == nil {
		t.Fatal("expected the signup count error to propagate")
	}
}

func TestGetReferralStats_CompletedCountError_Propagates(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(&account.ReferralCode{ID: "code-1", Code: "ABCD1234"}, nil).Once()
	referralRepo.EXPECT().CountEventsByCodeID(mock.Anything, "code-1", account.EventTypeSignup).Return(int64(3), nil).Once()
	referralRepo.EXPECT().CountEventsByCodeID(mock.Anything, "code-1", account.EventTypeTestCompleted).Return(int64(0), errors.New("db down")).Once()
	uc := NewProfileUseCase(nil, referralRepo, testLogger())

	if _, err := uc.GetReferralStats(context.Background(), "user-1"); err == nil {
		t.Fatal("expected the completed count error to propagate")
	}
}
