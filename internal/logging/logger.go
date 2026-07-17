package logging

import (
	"context"
	"os"
	"runtime"

	"github.com/rs/zerolog"
)

func New(levelStr string) zerolog.Logger {
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05.000Z07:00"
	zerolog.TimestampFieldName = "timestamp"
	zerolog.LevelFieldName = "level"
	zerolog.MessageFieldName = "message"
	zerolog.ErrorFieldName = "error"

	level, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		level = zerolog.InfoLevel
	}

	return zerolog.New(os.Stderr).
		Level(level).
		With().
		Timestamp().
		Logger().
		Hook(errorStackHook{})
}

// errorStackHook appends the current goroutine stack trace on error-level logs.
type errorStackHook struct{}

func (h errorStackHook) Run(e *zerolog.Event, level zerolog.Level, _ string) {
	if level >= zerolog.ErrorLevel {
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		e.Str("stacktrace", string(buf[:n]))
	}
}

func WithContext(ctx context.Context, logger zerolog.Logger) context.Context {
	return logger.WithContext(ctx)
}

func FromContext(ctx context.Context) zerolog.Logger {
	if l := zerolog.Ctx(ctx); l != nil && l.GetLevel() != zerolog.Disabled {
		return *l
	}
	return zerolog.New(os.Stderr).Level(zerolog.InfoLevel)
}
