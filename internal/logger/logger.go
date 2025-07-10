package logger

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Logger defines a standard interface for logging.
type Logger interface {
	Debugf(format string, v ...interface{})
	Infof(format string, v ...interface{})
	Warnf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
}

// SlogLogger is a wrapper around Go's structured logger.
type SlogLogger struct {
	*slog.Logger
}

// NewLogger creates a new logger instance based on the specified level.
func NewLogger(level string) Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})

	return &SlogLogger{slog.New(handler)}
}

// Debugf logs a message at the debug level.
func (l *SlogLogger) Debugf(format string, v ...interface{}) {
	l.Debug(fmt.Sprintf(format, v...))
}

// Infof logs a message at the info level.
func (l *SlogLogger) Infof(format string, v ...interface{}) {
	l.Info(fmt.Sprintf(format, v...))
}

// Warnf logs a message at the warn level.
func (l *SlogLogger) Warnf(format string, v ...interface{}) {
	l.Warn(fmt.Sprintf(format, v...))
}

// Errorf logs a message at the error level.
func (l *SlogLogger) Errorf(format string, v ...interface{}) {
	l.Error(fmt.Sprintf(format, v...))
}
