package handler

import (
	"net/http"

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

// bindJSON parses the request body into payload, logging the rejection and
// returning a VALIDATION_ERROR response on failure — the shared first step
// of every handler method that accepts a JSON body.
func bindJSON(c echo.Context, log logger.Logger, action string, payload interface{}) error {
	if err := c.Bind(payload); err != nil {
		log.Warn(action+" rejected", "reason", "bind_error", "error", err)
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body format")
	}
	return nil
}

// unwrapMessage strips the leading "context: " prefix chain from a %w-wrapped
// error, leaving just the innermost human-readable detail — used wherever an
// application.ErrInvalidInput's message is surfaced back to the client.
func unwrapMessage(err error) string {
	msg := err.Error()
	for i := 0; i < len(msg)-2; i++ {
		if msg[i] == ':' && msg[i+1] == ' ' {
			return msg[i+2:]
		}
	}
	return msg
}
