// Package logger defines the structured-logging interface and its slog-backed implementation.
package logger

import "context"

// requestIDKey is the private context key for the per-request correlation ID.
type requestIDKey struct{}

// ContextWithRequestID returns a child context carrying the request ID —
// set once by the HTTP RequestID middleware; everything downstream reads it
// via RequestIDFromContext/WithRequestID.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext returns the request ID carried by ctx, or "" when
// there is none (worker jobs, tests, direct calls).
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithRequestID returns log bound with the ctx's request_id field, or log
// unchanged when ctx carries none
func WithRequestID(ctx context.Context, log Logger) Logger {
	if id := RequestIDFromContext(ctx); id != "" {
		return log.With("request_id", id)
	}
	return log
}
