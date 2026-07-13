package handler

import (
	"errors"
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/profile"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/dto"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

type ProfileHandler struct {
	profileUseCase *profile.ProfileUseCase
	log            logger.Logger
}

func NewProfileHandler(
	profileUseCase *profile.ProfileUseCase,
	log logger.Logger,
) *ProfileHandler {
	return &ProfileHandler{
		profileUseCase: profileUseCase,
		log:            log.With("handler", "profile"),
	}
}

// UpdateProfile handles PATCH /v1/account/profile
// @Summary      Update account profile (partial update)
// @Description  Updates `display_name`/`age`/`status`/`preferred_locale` — send only the fields you want to change.
// @Description  Used both to change locale from settings and to complete a profile for members who
// @Description  registered without ever having a `GUEST_SESSION`.
// @Tags         Account
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body dto.UpdateProfileRequestDTO true "Fields to update (all optional)"
// @Success      200 {object} httpresponse.Response{data=profile.ProfileResponse} "Profile updated"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR — an included field has an invalid value"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/account/profile [patch]
func (h *ProfileHandler) UpdateProfile(c echo.Context) error {
	var payload dto.UpdateProfileRequestDTO
	if err := c.Bind(&payload); err != nil {
		h.log.Warn("update profile rejected", "reason", "bind_error", "error", err)
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body format")
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
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
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
func (h *ProfileHandler) GetReferralCode(c echo.Context) error {
	resp, err := h.profileUseCase.GetReferralCode(c.Request().Context(), middleware.UserIDFromContext(c))
	if err != nil {
		h.log.Error("get referral code failed", "error", err)
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}
