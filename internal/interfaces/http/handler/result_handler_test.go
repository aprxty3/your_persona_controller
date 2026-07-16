package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
)

func newResultCtx(method, body, resultID, userID, guestSessionID string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, "/", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, "/", nil)
	}
	if guestSessionID != "" {
		req.AddCookie(&http.Cookie{Name: "session_id", Value: guestSessionID})
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(resultID)
	if userID != "" {
		c.Set(middleware.ContextUserID, userID)
	}
	c.Set(middleware.ContextLocale, "en")
	return c, rec
}

func TestGetQuestions_Success_200(t *testing.T) {
	qRepo := mocks.NewMockQuestionCatalogRepository(t)
	qRepo.EXPECT().FindAllWithTranslation(mock.Anything, "en").Return(nil, nil, nil).Once()

	h := NewResultHandler(assessment.NewQuestionCatalogUseCase(qRepo, testLog()), nil, testLog())
	c, rec := newResultCtx(http.MethodGet, "", "", "", "")

	if err := h.GetQuestions(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetResult_NotFound_404(t *testing.T) {
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "missing-id").Return(nil, nil).Once()

	h := NewResultHandler(nil, assessment.NewResultUseCase(repo, mocks.NewMockPDFSignerService(t), testLog()), testLog())
	c, rec := newResultCtx(http.MethodGet, "", "missing-id", "", "")

	if err := h.GetResult(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "RESULT_NOT_FOUND" {
		t.Errorf("expected RESULT_NOT_FOUND, got %+v", body.Error)
	}
}

func TestGetResult_Success_200_IsOwnerTrue(t *testing.T) {
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "result-1").Return(&testresult.TestResult{
		ID: "result-1", UserID: strPtr("user-1"), Status: testresult.StatusCompleted,
	}, nil).Once()

	h := NewResultHandler(nil, assessment.NewResultUseCase(repo, mocks.NewMockPDFSignerService(t), testLog()), testLog())
	c, rec := newResultCtx(http.MethodGet, "", "result-1", "user-1", "")

	if err := h.GetResult(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"is_owner":true`) {
		t.Errorf("expected is_owner=true, got: %s", rec.Body.String())
	}
}

func strPtr(s string) *string { return &s }

func TestUpdateMascotStyle_InvalidStyle_400(t *testing.T) {
	h := NewResultHandler(nil, assessment.NewResultUseCase(mocks.NewMockResultRepository(t), mocks.NewMockPDFSignerService(t), testLog()), testLog())
	c, rec := newResultCtx(http.MethodPatch, `{"mascot_style":"not_a_style"}`, "result-1", "user-1", "")

	if err := h.UpdateMascotStyle(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateMascotStyle_NotOwner_403(t *testing.T) {
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "result-1").Return(&testresult.TestResult{
		ID: "result-1", UserID: strPtr("someone-else"), Status: testresult.StatusCompleted,
	}, nil).Once()

	h := NewResultHandler(nil, assessment.NewResultUseCase(repo, mocks.NewMockPDFSignerService(t), testLog()), testLog())
	c, rec := newResultCtx(http.MethodPatch, `{"mascot_style":"style_b"}`, "result-1", "user-1", "")

	if err := h.UpdateMascotStyle(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "FORBIDDEN" {
		t.Errorf("expected FORBIDDEN, got %+v", body.Error)
	}
}

func TestUpdateMascotStyle_Success_200(t *testing.T) {
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "result-1").Return(&testresult.TestResult{
		ID: "result-1", UserID: strPtr("user-1"), Status: testresult.StatusCompleted,
	}, nil).Once()
	repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()

	h := NewResultHandler(nil, assessment.NewResultUseCase(repo, mocks.NewMockPDFSignerService(t), testLog()), testLog())
	c, rec := newResultCtx(http.MethodPatch, `{"mascot_style":"style_b"}`, "result-1", "user-1", "")

	if err := h.UpdateMascotStyle(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPDFStatus_GuestOwner_Success_200(t *testing.T) {
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "result-1").Return(&testresult.TestResult{
		ID: "result-1", GuestSessionID: strPtr("guest-sess-1"), Status: testresult.StatusCompleted, PDFStatus: testresult.PDFStatusProcessing,
	}, nil).Once()

	h := NewResultHandler(nil, assessment.NewResultUseCase(repo, mocks.NewMockPDFSignerService(t), testLog()), testLog())
	c, rec := newResultCtx(http.MethodGet, "", "result-1", "", "guest-sess-1")

	if err := h.GetPDFStatus(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "processing") {
		t.Errorf("expected pdf_status=processing, got: %s", rec.Body.String())
	}
}

func TestGetPDF_NotReady_409(t *testing.T) {
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "result-1").Return(&testresult.TestResult{
		ID: "result-1", UserID: strPtr("user-1"), Status: testresult.StatusCompleted, PDFUrl: nil,
	}, nil).Once()

	h := NewResultHandler(nil, assessment.NewResultUseCase(repo, mocks.NewMockPDFSignerService(t), testLog()), testLog())
	c, rec := newResultCtx(http.MethodGet, "", "result-1", "user-1", "")

	if err := h.GetPDF(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "PDF_NOT_READY" {
		t.Errorf("expected PDF_NOT_READY, got %+v", body.Error)
	}
}

func TestGetPDF_Success_302Redirect(t *testing.T) {
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "result-1").Return(&testresult.TestResult{
		ID: "result-1", UserID: strPtr("user-1"), Status: testresult.StatusCompleted, PDFUrl: strPtr("https://storage.example.com/obj.pdf"),
	}, nil).Once()
	signer := mocks.NewMockPDFSignerService(t)
	signer.EXPECT().PresignedGetURL(mock.Anything, "https://storage.example.com/obj.pdf", mock.Anything).
		Return("https://storage.example.com/signed-url", nil).Once()

	h := NewResultHandler(nil, assessment.NewResultUseCase(repo, signer, testLog()), testLog())
	c, rec := newResultCtx(http.MethodGet, "", "result-1", "user-1", "")

	if err := h.GetPDF(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "https://storage.example.com/signed-url" {
		t.Errorf("expected redirect to signed URL, got %q", loc)
	}
}
