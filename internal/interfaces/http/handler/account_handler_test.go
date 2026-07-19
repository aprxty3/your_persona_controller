package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/application/profile"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	deletiondomain "github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest"
	deletiondomainmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
)

func newAuthedCtx(method, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, "/", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, "/", nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(middleware.ContextUserID, "user-1")
	return c, rec
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder) httpresponse.Response {
	t.Helper()
	var body httpresponse.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v (raw: %s)", err, rec.Body.String())
	}
	return body
}

func TestUpdateProfile_Success(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", DisplayName: "Old Name"}, nil).Once()
	userRepo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	h := NewAccountHandler(profile.NewProfileUseCase(userRepo, accountmocks.NewMockReferralRepository(t), testLog()), nil, testLog())
	c, rec := newAuthedCtx(http.MethodPatch, `{"display_name":"New Name"}`)

	if err := h.UpdateProfile(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if !body.Success {
		t.Error("expected success=true")
	}
}

func TestUpdateProfile_InvalidInput_400(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1"}, nil).Once()

	h := NewAccountHandler(profile.NewProfileUseCase(userRepo, accountmocks.NewMockReferralRepository(t), testLog()), nil, testLog())
	c, rec := newAuthedCtx(http.MethodPatch, `{"age":-5}`)

	if err := h.UpdateProfile(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != errCodeValidation {
		t.Errorf("expected VALIDATION_ERROR, got %+v", body.Error)
	}
}

func TestUpdateProfile_MalformedBody_400(t *testing.T) {
	h := NewAccountHandler(profile.NewProfileUseCase(accountmocks.NewMockUserRepository(t), accountmocks.NewMockReferralRepository(t), testLog()), nil, testLog())
	c, rec := newAuthedCtx(http.MethodPatch, `{"age":`)

	// A bind failure legitimately returns errResponseWritten (so real Echo routing
	// stops there) — the response body/status is what matters here, not the
	// return value, since Echo's error middleware is what consumes it in production.
	_ = h.UpdateProfile(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetReferralCode_ExistingCode_ReturnedAsIs(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(&account.ReferralCode{Code: "ABC12345"}, nil).Once()

	h := NewAccountHandler(profile.NewProfileUseCase(accountmocks.NewMockUserRepository(t), referralRepo, testLog()), nil, testLog())
	c, rec := newAuthedCtx(http.MethodGet, "")

	if err := h.GetReferralCode(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ABC12345") {
		t.Errorf("expected response to contain the referral code, got: %s", rec.Body.String())
	}
}

func TestGetReferralCode_RepoError_500(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(nil, assertErrHandler).Once()

	h := NewAccountHandler(profile.NewProfileUseCase(accountmocks.NewMockUserRepository(t), referralRepo, testLog()), nil, testLog())
	c, rec := newAuthedCtx(http.MethodGet, "")

	if err := h.GetReferralCode(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestGetReferralStats_NoCodeYet_ReturnsZeroCounts200(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t) // no CountEventsByCodeID EXPECT(): must never be called
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(nil, nil).Once()

	h := NewAccountHandler(profile.NewProfileUseCase(accountmocks.NewMockUserRepository(t), referralRepo, testLog()), nil, testLog())
	c, rec := newAuthedCtx(http.MethodGet, "")

	if err := h.GetReferralStats(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (not 404/auto-generate), got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"signup_count":0`) || !strings.Contains(rec.Body.String(), `"completed_count":0`) {
		t.Errorf("expected zero counts, got: %s", rec.Body.String())
	}
}

func TestGetReferralStats_ExistingCode_ReturnsCountsNoInviteePII(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(&account.ReferralCode{ID: "code-1", Code: "ABC12345"}, nil).Once()
	referralRepo.EXPECT().CountEventsByCodeID(mock.Anything, "code-1", account.EventTypeSignup).Return(int64(3), nil).Once()
	referralRepo.EXPECT().CountEventsByCodeID(mock.Anything, "code-1", account.EventTypeTestCompleted).Return(int64(1), nil).Once()

	h := NewAccountHandler(profile.NewProfileUseCase(accountmocks.NewMockUserRepository(t), referralRepo, testLog()), nil, testLog())
	c, rec := newAuthedCtx(http.MethodGet, "")

	if err := h.GetReferralStats(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	data, ok := body.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a JSON object, got %T", body.Data)
	}
	// Privacy contract (UU PDP): only code + aggregate counts, nothing else —
	// explicitly enumerate the allowed key set rather than spot-checking,
	// so an accidental future field (email/user_id/name) fails this test.
	allowedKeys := map[string]bool{"code": true, "signup_count": true, "completed_count": true}
	for k := range data {
		if !allowedKeys[k] {
			t.Errorf("response contains unexpected field %q — possible PII leak, response must be aggregate-only", k)
		}
	}
	if data["code"] != "ABC12345" || data["signup_count"] != float64(3) || data["completed_count"] != float64(1) {
		t.Errorf("expected code=ABC12345 signup_count=3 completed_count=1, got %+v", data)
	}
}

func TestGetReferralStats_RepoError_500(t *testing.T) {
	referralRepo := accountmocks.NewMockReferralRepository(t)
	referralRepo.EXPECT().FindCodeByUserID(mock.Anything, "user-1").Return(nil, assertErrHandler).Once()

	h := NewAccountHandler(profile.NewProfileUseCase(accountmocks.NewMockUserRepository(t), referralRepo, testLog()), nil, testLog())
	c, rec := newAuthedCtx(http.MethodGet, "")

	if err := h.GetReferralStats(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

var assertErrHandler = &handlerTestErr{}

type handlerTestErr struct{}

func (e *handlerTestErr) Error() string { return "boom" }

func TestRequestDeletion_Success_200(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(nil, nil).Once()
	deleteRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", Email: "a@example.com"}, nil).Once()

	h := NewAccountHandler(nil, deletionrequest.NewDeletionUseCase(userRepo, deleteRepo, testLog()), testLog())
	c, rec := newAuthedCtx(http.MethodPost, "")

	if err := h.RequestDeletion(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequestDeletion_AlreadyRequested_409(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").
		Return(&deletiondomain.DataDeletionRequest{ID: "req-1", Status: deletiondomain.StatusPendingGrace}, nil).Once()

	h := NewAccountHandler(nil, deletionrequest.NewDeletionUseCase(accountmocks.NewMockUserRepository(t), deleteRepo, testLog()), testLog())
	c, rec := newAuthedCtx(http.MethodPost, "")

	if err := h.RequestDeletion(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "DELETION_ALREADY_REQUESTED" {
		t.Errorf("expected DELETION_ALREADY_REQUESTED, got %+v", body.Error)
	}
}

func TestCancelDeletion_Success_200(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").
		Return(&deletiondomain.DataDeletionRequest{ID: "req-1", Status: deletiondomain.StatusPendingGrace}, nil).Once()
	deleteRepo.EXPECT().TransitionStatus(mock.Anything, "req-1", deletiondomain.StatusPendingGrace, deletiondomain.StatusCancelled, mock.Anything).
		Return(true, nil).Once()

	h := NewAccountHandler(nil, deletionrequest.NewDeletionUseCase(accountmocks.NewMockUserRepository(t), deleteRepo, testLog()), testLog())
	c, rec := newAuthedCtx(http.MethodPost, "")

	if err := h.CancelDeletion(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCancelDeletion_NoActiveRequest_404(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(nil, nil).Once()

	h := NewAccountHandler(nil, deletionrequest.NewDeletionUseCase(accountmocks.NewMockUserRepository(t), deleteRepo, testLog()), testLog())
	c, rec := newAuthedCtx(http.MethodPost, "")

	if err := h.CancelDeletion(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "NO_ACTIVE_DELETION_REQUEST" {
		t.Errorf("expected NO_ACTIVE_DELETION_REQUEST, got %+v", body.Error)
	}
}

func TestCancelDeletion_AlreadyProcessing_409(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").
		Return(&deletiondomain.DataDeletionRequest{ID: "req-1", Status: deletiondomain.StatusProcessing}, nil).Once()

	h := NewAccountHandler(nil, deletionrequest.NewDeletionUseCase(accountmocks.NewMockUserRepository(t), deleteRepo, testLog()), testLog())
	c, rec := newAuthedCtx(http.MethodPost, "")

	if err := h.CancelDeletion(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "DELETION_ALREADY_PROCESSING" {
		t.Errorf("expected DELETION_ALREADY_PROCESSING, got %+v", body.Error)
	}
}
