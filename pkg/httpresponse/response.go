package httpresponse

import "github.com/labstack/echo/v4"

// ErrorDetail represents the standardized error structure.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Response represents the unified API response envelope.
type Response struct {
	Success bool         `json:"success"`
	Data    any          `json:"data,omitempty"`
	Meta    any          `json:"meta,omitempty"`
	Error   *ErrorDetail `json:"error,omitempty"`
}

// Success sends a standardized JSON success response.
func Success(c echo.Context, statusCode int, data any, meta any) error {
	return c.JSON(statusCode, Response{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// Error sends a standardized JSON error response.
func Error(c echo.Context, statusCode int, errorCode, message string) error {
	return c.JSON(statusCode, Response{
		Success: false,
		Error: &ErrorDetail{
			Code:    errorCode,
			Message: message,
		},
	})
}
