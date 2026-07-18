package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/dto"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/mocks"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
)

// allowingIPLimiter returns an IPRateLimiter mock that always allows — the
// default for every test not specifically exercising the TICKET-22
// rate-limit gate itself, since Submit's use case checks it unconditionally
// up front.
func allowingIPLimiter(t *testing.T) *mocks.MockIPRateLimiter {
	limiter := mocks.NewMockIPRateLimiter(t)
	limiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Maybe()
	return limiter
}

func newSubmitCtx(t *testing.T, body string, idemKey string, sessionCookie string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/assessment/submit", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	if sessionCookie != "" {
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionCookie})
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

const validSubmitBody = `{"locale":"en","answers":[{"question_id":"q1","value":"4"}]}`

func TestSubmit_MissingIdempotencyKey_400(t *testing.T) {
	h := NewAssessmentHandler(nil, testLog())
	c, rec := newSubmitCtx(t, validSubmitBody, "", "sess-1")

	if err := h.Submit(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %+v", body.Error)
	}
}

func TestSubmit_MissingSessionAndAuth_400(t *testing.T) {
	h := NewAssessmentHandler(nil, testLog())
	c, rec := newSubmitCtx(t, validSubmitBody, "key-1", "")

	if err := h.Submit(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %+v", body.Error)
	}
}

func TestSubmit_IdempotencyCacheHit_200(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).
		Return(&dto.SubmitResponse{ResultID: "cached-1", Status: "completed"}, nil).Once()

	uc := assessment.NewSubmitAssessmentUseCase(nil, nil, nil, nil, nil, nil, idemSvc, nil, allowingIPLimiter(t), nil, testLog())
	h := NewAssessmentHandler(uc, testLog())
	c, rec := newSubmitCtx(t, validSubmitBody, "key-1", "sess-1")

	if err := h.Submit(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cached-1") {
		t.Errorf("expected the cached response to be returned, got: %s", rec.Body.String())
	}
}

func TestSubmit_IdempotencyKeyReused_409(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, application.ErrIdempotencyKeyReused).Once()

	uc := assessment.NewSubmitAssessmentUseCase(nil, nil, nil, nil, nil, nil, idemSvc, nil, allowingIPLimiter(t), nil, testLog())
	h := NewAssessmentHandler(uc, testLog())
	c, rec := newSubmitCtx(t, validSubmitBody, "key-1", "sess-1")

	if err := h.Submit(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "IDEMPOTENCY_KEY_REUSED" {
		t.Errorf("expected IDEMPOTENCY_KEY_REUSED, got %+v", body.Error)
	}
}

func TestSubmit_LockNotAcquired_423(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	lockSvc := mocks.NewMockDistributedLockService(t)
	lockSvc.EXPECT().AcquireLock(mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Once()

	uc := assessment.NewSubmitAssessmentUseCase(nil, nil, nil, nil, nil, lockSvc, idemSvc, nil, allowingIPLimiter(t), nil, testLog())
	h := NewAssessmentHandler(uc, testLog())
	c, rec := newSubmitCtx(t, validSubmitBody, "key-1", "sess-1")

	if err := h.Submit(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusLocked {
		t.Fatalf("expected 423, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "LOCK_NOT_ACQUIRED" {
		t.Errorf("expected LOCK_NOT_ACQUIRED, got %+v", body.Error)
	}
}

func TestSubmit_QuotaExceeded_429(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	lockSvc := mocks.NewMockDistributedLockService(t)
	lockSvc.EXPECT().AcquireLock(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()
	lockSvc.EXPECT().ReleaseLock(mock.Anything, mock.Anything).Return(nil).Once()
	trRepo := mocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().CountMonthlyUsageByGuestSession(mock.Anything, "sess-1").Return(int64(999999), nil).Once()

	uc := assessment.NewSubmitAssessmentUseCase(nil, trRepo, nil, nil, nil, lockSvc, idemSvc, nil, allowingIPLimiter(t), nil, testLog())
	h := NewAssessmentHandler(uc, testLog())
	c, rec := newSubmitCtx(t, validSubmitBody, "key-1", "sess-1")

	if err := h.Submit(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "QUOTA_EXCEEDED" {
		t.Errorf("expected QUOTA_EXCEEDED, got %+v", body.Error)
	}
}

func TestSubmit_RateLimited_429(t *testing.T) {
	limiter := mocks.NewMockIPRateLimiter(t)
	limiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(false, 900, nil).Once()

	uc := assessment.NewSubmitAssessmentUseCase(nil, nil, nil, nil, nil, nil, nil, nil, limiter, nil, testLog())
	h := NewAssessmentHandler(uc, testLog())
	c, rec := newSubmitCtx(t, validSubmitBody, "key-1", "sess-1")

	if err := h.Submit(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "RATE_LIMITED" {
		t.Errorf("expected RATE_LIMITED, got %+v", body.Error)
	}
	if body.Meta == nil {
		t.Error("expected meta.retry_after_seconds to be present")
	}
}

func TestSubmit_EmptyAnswers_400(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	uc := assessment.NewSubmitAssessmentUseCase(nil, nil, nil, nil, nil, nil, idemSvc, nil, allowingIPLimiter(t), nil, testLog())
	h := NewAssessmentHandler(uc, testLog())
	c, rec := newSubmitCtx(t, `{"locale":"en","answers":[]}`, "key-1", "sess-1")

	if err := h.Submit(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
