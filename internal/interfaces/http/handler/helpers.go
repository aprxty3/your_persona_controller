package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

func httpcallError(c echo.Context, err error) error {
	return httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}

func httpcallErrorCustom(c echo.Context, code int, errCode string, msg string) error {
	return httpresponse.Error(c, code, errCode, msg)
}

func httpcallSuccess(c echo.Context, code int, data interface{}, meta interface{}) error {
	return httpresponse.Success(c, code, data, meta)
}

func rateLimitedResponse(c echo.Context, retryAfterSeconds int, message string) error {
	return c.JSON(http.StatusTooManyRequests, httpresponse.Response{
		Success: false,
		Error:   &httpresponse.ErrorDetail{Code: "RATE_LIMITED", Message: message},
		Meta:    map[string]interface{}{"retry_after_seconds": retryAfterSeconds},
	})
}

var errResponseWritten = errors.New("handler: response already written")

func bindJSON(c echo.Context, log logger.Logger, action string, payload interface{}) error {
	if err := c.Bind(payload); err != nil {
		log.Warn(action+" rejected", "reason", "bind_error", "error", err)
		if writeErr := httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body format"); writeErr != nil {
			return writeErr
		}
		return errResponseWritten
	}
	return nil
}

func unwrapMessage(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 {
		return msg[i+2:]
	}
	return msg
}
