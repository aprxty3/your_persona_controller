package logger

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it — NewLogger hardcodes os.Stdout as its sink, so
// this is the only way to observe what it actually writes.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read pipe: %v", err)
	}
	return string(out)
}

func TestNewLogger_Development_ProducesTextFormat(t *testing.T) {
	out := captureStdout(t, func() {
		l := NewLogger("development")
		l.Info("hello world", "key", "value")
	})

	if !strings.Contains(out, "hello world") {
		t.Errorf("expected output to contain the message, got: %s", out)
	}
	if !strings.Contains(out, "key=value") {
		t.Errorf("expected text-handler key=value formatting, got: %s", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("expected non-JSON text output in development mode, got: %s", out)
	}
}

func TestNewLogger_Production_ProducesJSONFormat(t *testing.T) {
	out := captureStdout(t, func() {
		l := NewLogger("production")
		l.Info("hello world", "key", "value")
	})

	trimmed := strings.TrimSpace(out)
	if !strings.HasPrefix(trimmed, "{") {
		t.Errorf("expected JSON output in production mode, got: %s", out)
	}
	if !strings.Contains(out, `"key":"value"`) {
		t.Errorf("expected JSON key/value pair, got: %s", out)
	}
}

// Production mode uses LevelInfo — Debug-level lines must not be emitted.
func TestNewLogger_Production_SuppressesDebugLevel(t *testing.T) {
	out := captureStdout(t, func() {
		l := NewLogger("production")
		l.Debug("should not appear")
	})
	if strings.Contains(out, "should not appear") {
		t.Errorf("expected debug line to be suppressed in production mode, got: %s", out)
	}
}

// Development mode uses LevelDebug — Debug-level lines must be emitted.
func TestNewLogger_Development_EmitsDebugLevel(t *testing.T) {
	out := captureStdout(t, func() {
		l := NewLogger("development")
		l.Debug("debug line here")
	})
	if !strings.Contains(out, "debug line here") {
		t.Errorf("expected debug line to be emitted in development mode, got: %s", out)
	}
}

// With() must return a NEW logger carrying the bound args on every
// subsequent call, without mutating the original (siblings must not leak
// each other's fields).
func TestWith_BindsArgsToNewLoggerWithoutMutatingOriginal(t *testing.T) {
	var base Logger
	var childOut, baseOut string

	childOut = captureStdout(t, func() {
		base = NewLogger("development")
		child := base.With("component", "test-component")
		child.Info("child message")
	})
	baseOut = captureStdout(t, func() {
		base.Info("base message")
	})

	if !strings.Contains(childOut, "component=test-component") {
		t.Errorf("expected child logger output to include bound field, got: %s", childOut)
	}
	if strings.Contains(baseOut, "component=test-component") {
		t.Errorf("expected base logger to remain unaffected by With(), got: %s", baseOut)
	}
}

func TestLogLevels_AllEmitTheirMessage(t *testing.T) {
	out := captureStdout(t, func() {
		l := NewLogger("development")
		l.Debug("debug-msg")
		l.Info("info-msg")
		l.Warn("warn-msg")
		l.Error("error-msg")
	})

	scanner := bufio.NewScanner(strings.NewReader(out))
	lineCount := 0
	for scanner.Scan() {
		if scanner.Text() != "" {
			lineCount++
		}
	}
	for _, msg := range []string{"debug-msg", "info-msg", "warn-msg", "error-msg"} {
		if !strings.Contains(out, msg) {
			t.Errorf("expected output to contain %q, got: %s", msg, out)
		}
	}
}
