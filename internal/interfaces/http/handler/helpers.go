package handler

import (
	"errors"
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

// errResponseWritten signals that a helper already wrote a rejection
// response to c and the caller must stop processing immediately. It exists
// because httpresponse.Error/Success return c.JSON()'s own result — nil on
// any successfully-written response, success or error alike — which is an
// I/O-failure signal, NOT a "did this succeed?" signal. A helper that writes
// an error response and then hands that nil back to its caller would let
// the caller wrongly conclude nothing went wrong and fall through to
// business logic after the response has already been sent (and, since Echo
// does not guard body writes past the first commit, corrupt it with a
// second one). Every helper below that can reject a request mid-handler
// (bindJSON, verifyTurnstile, otpVerifyError) returns this instead.
var errResponseWritten = errors.New("handler: response already written")

// bindJSON parses the request body into payload, logging the rejection and
// writing a VALIDATION_ERROR response on failure — the shared first step
// of every handler method that accepts a JSON body. Returns errResponseWritten
// (not nil) on failure so callers reliably stop processing instead of
// falling through to business logic after a response has already been sent.
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
