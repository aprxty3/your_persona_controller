package result

// ErrorType represents general classes of business errors.
type ErrorType string

const (
	ErrorTypeNone         ErrorType = ""
	ErrorTypeNotFound     ErrorType = "NOT_FOUND"
	ErrorTypeValidation   ErrorType = "VALIDATION"
	ErrorTypeUnauthorized ErrorType = "UNAUTHORIZED"
	ErrorTypeConflict     ErrorType = "CONFLICT"
	ErrorTypeInternal     ErrorType = "INTERNAL"
	ErrorTypeRateLimited  ErrorType = "RATE_LIMITED"
	ErrorTypeLocked       ErrorType = "LOCKED"
)

// AppError is the standardized business error format.
type AppError struct {
	Type    ErrorType
	Message string
	Code    string // specific code (e.g., "INVALID_OTP")
	Meta    any    // extra info (attempts remaining, cooldown, etc.)
}

func (e AppError) Error() string {
	return e.Message
}

// Result encapsulates a standard return signature containing values or errors.
type Result[T any] struct {
	Value   T
	Error   *AppError
	Success bool
}

// Ok creates a success Result.
func Ok[T any](value T) Result[T] {
	return Result[T]{
		Value:   value,
		Success: true,
	}
}

// Fail creates a failure Result.
func Fail[T any](err AppError) Result[T] {
	return Result[T]{
		Error:   &err,
		Success: false,
	}
}
