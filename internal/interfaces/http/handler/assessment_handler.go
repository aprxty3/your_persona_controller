package handler

import (
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/labstack/echo/v4"
)

// AssessmentHandler handles HTTP requests for assessment-related endpoints.
type AssessmentHandler struct {
	submitUseCase *assessment.SubmitAssessmentUseCase
}

// NewAssessmentHandler is the constructor for Dependency Injection.
func NewAssessmentHandler(uc *assessment.SubmitAssessmentUseCase) *AssessmentHandler {
	return &AssessmentHandler{
		submitUseCase: uc,
	}
}

// SubmitRequestDTO represents the incoming JSON payload for assessment submission.
type SubmitRequestDTO struct {
	Locale  string `json:"locale"`
	Answers []struct {
		QuestionID string `json:"question_id"`
		Value      string `json:"value"`
	} `json:"answers"`
}

// Submit handles the POST /v1/assessment/submit endpoint.
func (h *AssessmentHandler) Submit(c echo.Context) error {
	// 1. Extract Idempotency-Key from Header
	idempotencyKey := c.Request().Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key header is required")
	}

	// 2. Parse Request Body
	var payload SubmitRequestDTO
	if err := c.Bind(&payload); err != nil {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
	}

	// 3. Extract Session ID or User ID
	// Based on API Contract, Guest uses 'session_id' cookie, Member uses 'access_token'.
	sessionID := ""
	isMember := false

	cookie, err := c.Cookie("session_id")
	if err == nil && cookie != nil {
		sessionID = cookie.Value
	} else {
		// TODO: Implement JWT Extraction for logged-in Members here.
		// For example:
		// userID := c.Get("user_id").(string)
		// sessionID = userID
		// isMember = true

		// If BOTH are missing, reject the request (VALIDATION_ERROR)
		if sessionID == "" && !isMember {
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Missing session_id cookie or access token")
		}
	}

	// 4. Map DTO to Usecase Request
	ucReq := assessment.SubmitRequest{
		IdempotencyKey: idempotencyKey,
		SessionID:      sessionID,
		IsMember:       isMember,
		Locale:         payload.Locale,
	}

	for _, ans := range payload.Answers {
		ucReq.Answers = append(ucReq.Answers, assessment.AnswerInput{
			QuestionID: ans.QuestionID,
			Value:      ans.Value,
		})
	}

	// 5. Execute the Core Business Logic (Usecase)
	resp, err := h.submitUseCase.Execute(c.Request().Context(), ucReq)
	if err != nil {
		// In a production app, map specific domain errors to 400, 429, or 500.
		// For now, we fallback to a generic 500 error.
		return httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}

	// 6. Return Standardized Success Response
	return httpresponse.Success(c, http.StatusOK, resp, nil)
}
