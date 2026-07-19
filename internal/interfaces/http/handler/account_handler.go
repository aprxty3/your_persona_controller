// Package handler implements the Echo HTTP handlers — one file per resource
// (auth, account, assessment, dashboard, result).
package handler

import (
	"errors"
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/application/profile"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/dto"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

// AccountHandler handles HTTP requests for everything under /v1/account/* —
// profile self-service, referral code retrieval, and the deletion-request lifecycle.
type AccountHandler struct {
	profileUseCase  *profile.UseCase
	deletionUseCase *deletionrequest.DeletionUseCase
	log             logger.Logger
}

// NewAccountHandler is the constructor for Dependency Injection.
func NewAccountHandler(
	profileUseCase *profile.UseCase,
	deletionUseCase *deletionrequest.DeletionUseCase,
	log logger.Logger,
) *AccountHandler {
	return &AccountHandler{
		profileUseCase:  profileUseCase,
		deletionUseCase: deletionUseCase,
		log:             log.With("handler", "account"),
	}
}

// UpdateProfile handles PATCH /v1/account/profile
// @Summary      Update account profile (partial update)
// @Description  Updates `display_name`/`age`/`status`/`preferred_locale` — send only the fields you want to change.
// @Description  Used both to change locale from settings and to complete a profile for members who
// @Description  registered without ever having a `GUEST_SESSION`.
// @Description
// @Description  **CSRF-protected**: requires header `X-CSRF-Token` set to the value of cookie `csrf_token`
// @Description  (double-submit pattern — the cookie is primed by the response of ANY prior request).
// @Description  Missing/invalid token → `403 Forbidden`.
// @Tags         Account
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        X-CSRF-Token header string true "From cookie csrf_token — see CSRF note above"
// @Param        request body dto.UpdateProfileRequestDTO true "Fields to update (all optional)"
// @Success      200 {object} httpresponse.Response{data=profile.Response} "Profile updated"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR — an included field has an invalid value"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      403 {object} httpresponse.Response "Missing/invalid X-CSRF-Token"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/account/profile [patch]
func (h *AccountHandler) UpdateProfile(c echo.Context) error {
	var payload dto.UpdateProfileRequestDTO
	if err := bindJSON(c, h.log, "update profile", &payload); err != nil {
		return err
	}

	resp, err := h.profileUseCase.UpdateProfile(c.Request().Context(), profile.UpdateProfileRequest{
		UserID:          middleware.UserIDFromContext(c),
		DisplayName:     payload.DisplayName,
		Age:             payload.Age,
		Status:          payload.Status,
		PreferredLocale: payload.PreferredLocale,
	})
	if err != nil {
		if errors.Is(err, application.ErrInvalidInput) {
			return httpresponse.Error(c, http.StatusBadRequest, errCodeValidation, unwrapMessage(err))
		}
		h.log.Error("update profile failed", "error", err)
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// GetReferralCode handles GET /v1/account/referral-code
// @Summary      Get (or generate) my referral code
// @Description  Returns the caller's referral code, generating and persisting one on the very first request.
// @Description  Subsequent calls always return the same code — one code per user, never rotated.
// @Tags         Account
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} httpresponse.Response{data=profile.ReferralCodeResponse} "Referral code"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/account/referral-code [get]
func (h *AccountHandler) GetReferralCode(c echo.Context) error {
	resp, err := h.profileUseCase.GetReferralCode(c.Request().Context(), middleware.UserIDFromContext(c))
	if err != nil {
		h.log.Error("get referral code failed", "error", err)
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// GetReferralStats handles GET /v1/account/referral-stats
// @Summary      Get my referral conversion stats
// @Description  Returns aggregate counts only — how many people signed up and completed a test using
// @Description  the caller's referral code. Never exposes invitee identity (no email/name/user_id),
// @Description  per data-privacy requirements (UU PDP) — this is deliberate, not a gap.
// @Description  If the caller has never generated a referral code yet, returns zero counts and an
// @Description  empty `code` (200, not 404) — use `/account/referral-code` to generate one first.
// @Tags         Account
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} httpresponse.Response{data=profile.ReferralStatsResponse} "Referral stats"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/account/referral-stats [get]
func (h *AccountHandler) GetReferralStats(c echo.Context) error {
	resp, err := h.profileUseCase.GetReferralStats(c.Request().Context(), middleware.UserIDFromContext(c))
	if err != nil {
		h.log.Error("get referral stats failed", "error", err)
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// RequestDeletion handles POST /v1/account/delete-request
// @Summary      Request account deletion
// @Description  Starts a 14-day grace period. The account remains fully usable during this window —
// @Description  use `/account/delete-request/cancel` any time before the grace period ends to abort.
// @Description
// @Description  Once the grace period elapses, a background worker anonymizes personal data
// @Description  (email, display name, essay answers, AI summary) and deletes stored PDFs.
// @Description  Aggregate data (mbti_type, grit_score) is retained.
// @Description
// @Description  **CSRF-protected**: requires header `X-CSRF-Token` set to the value of cookie `csrf_token`
// @Description  (double-submit pattern — the cookie is primed by the response of ANY prior request).
// @Description  Missing/invalid token → `403 Forbidden`.
// @Tags         Account
// @Produce      json
// @Security     BearerAuth
// @Param        X-CSRF-Token header string true "From cookie csrf_token — see CSRF note above"
// @Success      200 {object} httpresponse.Response{data=deletionrequest.RequestDeletionResponse} "Grace period started"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      403 {object} httpresponse.Response "Missing/invalid X-CSRF-Token"
// @Failure      409 {object} httpresponse.Response "DELETION_ALREADY_REQUESTED — an active request already exists"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/account/delete-request [post]
func (h *AccountHandler) RequestDeletion(c echo.Context) error {
	resp, err := h.deletionUseCase.RequestDeletion(c.Request().Context(), middleware.UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, application.ErrDeletionAlreadyRequested) {
			return httpcallErrorCustom(c, http.StatusConflict, "DELETION_ALREADY_REQUESTED", "A deletion request is already in progress for this account")
		}
		h.log.Error("request deletion failed", "error", err)
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// CancelDeletion handles POST /v1/account/delete-request/cancel
// @Summary      Cancel a pending account deletion request
// @Description  Aborts an in-progress grace period — only works while the request is still
// @Description  `pending_grace`. Once the anonymization worker has started, it's too late to cancel.
// @Description
// @Description  **CSRF-protected**: requires header `X-CSRF-Token` set to the value of cookie `csrf_token`
// @Description  (double-submit pattern — the cookie is primed by the response of ANY prior request).
// @Description  Missing/invalid token → `403 Forbidden`.
// @Tags         Account
// @Produce      json
// @Security     BearerAuth
// @Param        X-CSRF-Token header string true "From cookie csrf_token — see CSRF note above"
// @Success      200 {object} httpresponse.Response "Deletion request cancelled"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      403 {object} httpresponse.Response "Missing/invalid X-CSRF-Token"
// @Failure      404 {object} httpresponse.Response "NO_ACTIVE_DELETION_REQUEST — nothing to cancel"
// @Failure      409 {object} httpresponse.Response "DELETION_ALREADY_PROCESSING — grace period already elapsed"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/account/delete-request/cancel [post]
func (h *AccountHandler) CancelDeletion(c echo.Context) error {
	err := h.deletionUseCase.CancelDeletion(c.Request().Context(), middleware.UserIDFromContext(c))
	if err != nil {
		switch {
		case errors.Is(err, application.ErrNoActiveDeletionRequest):
			return httpcallErrorCustom(c, http.StatusNotFound, "NO_ACTIVE_DELETION_REQUEST", "There is no active deletion request to cancel")
		case errors.Is(err, application.ErrDeletionAlreadyProcessing):
			return httpcallErrorCustom(c, http.StatusConflict, "DELETION_ALREADY_PROCESSING", "The grace period has already elapsed and processing has started — cancellation is no longer possible")
		default:
			h.log.Error("cancel deletion failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusOK, map[string]string{"message": "Deletion request cancelled"}, nil)
}
