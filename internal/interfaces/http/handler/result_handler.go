package handler

import (
	"errors"
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/dto"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

// ResultHandler handles HTTP requests for the question bank and test-result
// detail/PDF endpoints.
type ResultHandler struct {
	questionUseCase *assessment.QuestionCatalogUseCase
	resultUseCase   *assessment.ResultUseCase
	log             logger.Logger
}

// NewResultHandler is the constructor for Dependency Injection.
func NewResultHandler(
	questionUseCase *assessment.QuestionCatalogUseCase,
	resultUseCase *assessment.ResultUseCase,
	log logger.Logger,
) *ResultHandler {
	return &ResultHandler{
		questionUseCase: questionUseCase,
		resultUseCase:   resultUseCase,
		log:             log.With("handler", "result"),
	}
}

// callerIdentity reads the two possible caller identities set upstream —
// a Member's user ID (AuthMiddleware.OptionalAuth) or a Guest's session_id
// cookie — for the ownership checks shared by every /v1/results/:id/* route.
func callerIdentity(c echo.Context) (userID, guestSessionID string) {
	userID = middleware.UserIDFromContext(c)
	if cookie, err := c.Cookie("session_id"); err == nil && cookie != nil {
		guestSessionID = cookie.Value
	}
	return userID, guestSessionID
}

// GetQuestions handles GET /v1/questions
// @Summary      Get the question bank
// @Description  Returns the full SJT/Likert/Essay question bank translated to the negotiated locale.
// @Tags         Assessment
// @Produce      json
// @Param        locale query string false "Overrides the negotiated locale for this request (en|id)"
// @Success      200 {object} httpresponse.Response{data=[]assessment.QuestionResponse} "Question bank"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/questions [get]
func (h *ResultHandler) GetQuestions(c echo.Context) error {
	questions, err := h.questionUseCase.ListQuestions(c.Request().Context(), middleware.LocaleFromContext(c))
	if err != nil {
		h.log.Error("get questions failed", "error", err)
		return httpcallError(c, err)
	}
	return httpcallSuccess(c, http.StatusOK, questions, nil)
}

// GetResult handles GET /v1/results/:id
// @Summary      Get a test result
// @Description  Optional auth. Anyone holding the result's (unguessable) ID can view it.
// @Description  `is_owner` tells the frontend whether to render owner-only affordances.
// @Tags         Result
// @Produce      json
// @Param        id path string true "Test result ID"
// @Success      200 {object} httpresponse.Response{data=assessment.ResultResponse} "Test result detail"
// @Failure      404 {object} httpresponse.Response "RESULT_NOT_FOUND"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/results/{id} [get]
func (h *ResultHandler) GetResult(c echo.Context) error {
	userID, guestSessionID := callerIdentity(c)

	resp, err := h.resultUseCase.GetByID(c.Request().Context(), c.Param("id"), userID, guestSessionID)
	if err != nil {
		return h.handleResultError(c, err, "get_result")
	}
	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// UpdateMascotStyle handles PATCH /v1/results/:id/mascot-style
// @Summary      Choose a mascot visual variant
// @Description  Owner only. Purely visual — never affects trait_scores/grit_score/ai_summary_text.
// @Tags         Result
// @Accept       json
// @Produce      json
// @Param        id path string true "Test result ID"
// @Param        request body dto.UpdateMascotStyleRequestDTO true "style_a or style_b"
// @Success      200 {object} httpresponse.Response{data=assessment.ResultResponse} "Updated result"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR — mascot_style must be style_a or style_b"
// @Failure      403 {object} httpresponse.Response "FORBIDDEN — caller does not own this result"
// @Failure      404 {object} httpresponse.Response "RESULT_NOT_FOUND"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/results/{id}/mascot-style [patch]
func (h *ResultHandler) UpdateMascotStyle(c echo.Context) error {
	var payload dto.UpdateMascotStyleRequestDTO
	if err := bindJSON(c, h.log, "update mascot style", &payload); err != nil {
		return err
	}

	userID, guestSessionID := callerIdentity(c)

	resp, err := h.resultUseCase.UpdateMascotStyle(c.Request().Context(), assessment.UpdateMascotStyleRequest{
		ResultID:             c.Param("id"),
		CallerUserID:         userID,
		CallerGuestSessionID: guestSessionID,
		MascotStyle:          payload.MascotStyle,
	})
	if err != nil {
		return h.handleResultError(c, err, "update_mascot_style")
	}
	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// GetPDFStatus handles GET /v1/results/:id/pdf-status
// @Summary      Poll PDF generation status
// @Description  Owner only. Frontend polls with exponential backoff (2s→4s→8s→cap 10s, ~90s total);
// @Description  `failed` means stop polling immediately.
// @Tags         Result
// @Produce      json
// @Param        id path string true "Test result ID"
// @Success      200 {object} httpresponse.Response{data=assessment.PDFStatusResponse} "pending|processing|completed|failed"
// @Failure      403 {object} httpresponse.Response "FORBIDDEN — caller does not own this result"
// @Failure      404 {object} httpresponse.Response "RESULT_NOT_FOUND"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/results/{id}/pdf-status [get]
func (h *ResultHandler) GetPDFStatus(c echo.Context) error {
	userID, guestSessionID := callerIdentity(c)

	resp, err := h.resultUseCase.GetPDFStatus(c.Request().Context(), c.Param("id"), userID, guestSessionID)
	if err != nil {
		return h.handleResultError(c, err, "get_pdf_status")
	}
	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// GetPDF handles GET /v1/results/:id/pdf
// @Summary      Download the generated PDF
// @Description  Owner only. Redirects (302) to a short-lived signed URL once the PDF is ready.
// @Tags         Result
// @Param        id path string true "Test result ID"
// @Success      302 {string} string "Redirect to signed PDF URL"
// @Failure      403 {object} httpresponse.Response "FORBIDDEN — caller does not own this result"
// @Failure      404 {object} httpresponse.Response "RESULT_NOT_FOUND"
// @Failure      409 {object} httpresponse.Response "PDF_NOT_READY — generation hasn't completed yet"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/results/{id}/pdf [get]
func (h *ResultHandler) GetPDF(c echo.Context) error {
	userID, guestSessionID := callerIdentity(c)

	resp, err := h.resultUseCase.GetPDFDownloadURL(c.Request().Context(), c.Param("id"), userID, guestSessionID)
	if err != nil {
		return h.handleResultError(c, err, "get_pdf")
	}
	return c.Redirect(http.StatusFound, resp.URL)
}

func (h *ResultHandler) handleResultError(c echo.Context, err error, op string) error {
	switch {
	case errors.Is(err, application.ErrInvalidInput):
		return httpresponse.Error(c, http.StatusBadRequest, errCodeValidation, unwrapMessage(err))
	case errors.Is(err, application.ErrResultNotFound):
		return httpresponse.Error(c, http.StatusNotFound, "RESULT_NOT_FOUND", "Test result not found")
	case errors.Is(err, application.ErrForbidden):
		return httpresponse.Error(c, http.StatusForbidden, "FORBIDDEN", "You do not have access to this result")
	case errors.Is(err, application.ErrPDFNotReady):
		return httpresponse.Error(c, http.StatusConflict, "PDF_NOT_READY", "PDF generation has not completed yet")
	default:
		h.log.Error(op+" failed", "error", err)
		return httpcallError(c, err)
	}
}
