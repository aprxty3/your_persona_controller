package logger

import (
	"context"
	"testing"
)

func TestRequestIDContextRoundTrip(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req-abc-123")
	if got := RequestIDFromContext(ctx); got != "req-abc-123" {
		t.Errorf("RequestIDFromContext = %q, want %q", got, "req-abc-123")
	}
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Errorf("expected empty request ID on a bare context, got %q", got)
	}
}

// context.WithoutCancel must preserve the request ID — the submit use case
// detaches cancellation before the Gemini phase but its logs still need the
// correlation ID.
func TestRequestIDSurvivesWithoutCancel(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req-abc-123")
	detached := context.WithoutCancel(ctx)
	if got := RequestIDFromContext(detached); got != "req-abc-123" {
		t.Errorf("request ID lost through WithoutCancel: got %q", got)
	}
}

func TestWithRequestID(t *testing.T) {
	base := NewLogger("test")

	ctx := ContextWithRequestID(context.Background(), "req-abc-123")
	if got := WithRequestID(ctx, base); got == base {
		t.Error("expected a bound (different) logger when ctx carries a request ID")
	}
	if got := WithRequestID(context.Background(), base); got != base {
		t.Error("expected the base logger unchanged when ctx has no request ID")
	}
}
