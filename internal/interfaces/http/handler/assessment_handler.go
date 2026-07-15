package handler

import (
	"errors"
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/dto"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/aprxty3/your_persona_controller.git/pkg/locale"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

// AssessmentHandler handles HTTP requests for assessment-related endpoints.
type AssessmentHandler struct {
	submitUseCase *assessment.SubmitAssessmentUseCase
	log           logger.Logger
}

// NewAssessmentHandler is the constructor for Dependency Injection.
func NewAssessmentHandler(uc *assessment.SubmitAssessmentUseCase, log logger.Logger) *AssessmentHandler {
	return &AssessmentHandler{
		submitUseCase: uc,
		log:           log.With("handler", "assessment"),
	}
}

// Submit handles the POST /v1/assessment/submit endpoint.
// @Summary      Submit assessment answers
// @Description  Guest-or-Member endpoint (Auth: Optional). Member identity is taken from the Bearer
// @Description  access token if present (set by AuthMiddleware.OptionalAuth); otherwise falls back to
// @Description  the `session_id` Guest cookie. Runs the AI analysis synchronously — expect 3-8s latency.
// @Tags         Assessment
// @Accept       json
// @Produce      json
// @Param        Idempotency-Key header string true "Client-generated UUIDv4, replayed to dedupe retries"
// @Param        request body dto.SubmitRequestDTO true "Locale + answer set"
// @Success      200 {object} httpresponse.Response{data=dto.SubmitResponse} "Assessment result"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR — missing Idempotency-Key, malformed body, or neither session_id cookie nor access token present"
// @Failure      409 {object} httpresponse.Response "IDEMPOTENCY_KEY_REUSED — same key replayed with a different payload"
// @Failure      423 {object} httpresponse.Response "LOCK_NOT_ACQUIRED — a submission for this identity is already in flight"
// @Failure      429 {object} httpresponse.Response "QUOTA_EXCEEDED — monthly submission quota reached"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/assessment/submit [post]
func (h *AssessmentHandler) Submit(c echo.Context) error {
	idempotencyKey := c.Request().Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key header is required")
	}

	var payload dto.SubmitRequestDTO
	if err := bindJSON(c, h.log, "submit assessment", &payload); err != nil {
		return err
	}

	sessionID := middleware.UserIDFromContext(c)
	isMember := sessionID != ""

	if !isMember {
		cookie, err := c.Cookie("session_id")
		if err == nil && cookie != nil {
			sessionID = cookie.Value
		}
	}

	if sessionID == "" {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Missing session_id cookie or access token")
	}

	reqLocale := payload.Locale
	if !locale.IsSupported(reqLocale) {
		reqLocale = middleware.LocaleFromContext(c)
	}

	ucReq := assessment.SubmitRequest{
		IdempotencyKey: idempotencyKey,
		SessionID:      sessionID,
		IsMember:       isMember,
		Locale:         reqLocale,
	}

	for _, ans := range payload.Answers {
		ucReq.Answers = append(ucReq.Answers, assessment.AnswerInput{
			QuestionID: ans.QuestionID,
			Value:      ans.Value,
		})
	}

	resp, err := h.submitUseCase.Execute(c.Request().Context(), ucReq)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
		case errors.Is(err, application.ErrIdempotencyKeyReused):
			return httpresponse.Error(c, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "This Idempotency-Key was already used with a different payload")
		case errors.Is(err, application.ErrLockNotAcquired):
			return httpresponse.Error(c, http.StatusLocked, "LOCK_NOT_ACQUIRED", "A submission for this session is already being processed")
		case errors.Is(err, application.ErrQuotaExceeded):
			return httpresponse.Error(c, http.StatusTooManyRequests, "QUOTA_EXCEEDED", "Monthly assessment quota reached")
		default:
			h.log.Error("submit failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}
