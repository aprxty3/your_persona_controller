package logger

import (
	"log/slog"
	"os"
)

// Logger defines the structured logging interface.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
}

// SlogLogger is the concrete implementation wrapper around log/slog.
type SlogLogger struct {
	logger *slog.Logger
}

// NewLogger constructs a Logger tailored for the given environment.
func NewLogger(env string) Logger {
	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}
	return &SlogLogger{
		logger: slog.New(handler),
	}
}

// Debug logs at debug level.
func (l *SlogLogger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Info logs at info level.
func (l *SlogLogger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn logs at warn level.
func (l *SlogLogger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error logs at error level.
func (l *SlogLogger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// With returns a child Logger that always includes the given key/value args.
func (l *SlogLogger) With(args ...any) Logger {
	return &SlogLogger{
		logger: l.logger.With(args...),
	}
}
