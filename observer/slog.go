package observer

import (
	"context"
	"log/slog"
)

type SlogOption func(*slogOptions)

type slogOptions struct {
	level   slog.Level
	message string
}

// WithSlogLevel sets the log level used by the slog observer.
func WithSlogLevel(level slog.Level) SlogOption {
	return func(opts *slogOptions) {
		opts.level = level
	}
}

// WithSlogMessage sets the log message used by the slog observer.
func WithSlogMessage(message string) SlogOption {
	return func(opts *slogOptions) {
		opts.message = message
	}
}

// NewSlog returns an observer that emits one structured log entry per
// completed storage operation.
func NewSlog(logger *slog.Logger, opts ...SlogOption) Observer {
	options := slogOptions{
		level:   slog.LevelInfo,
		message: "storx operation",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	logger = normalizeSlogLogger(logger)

	return ObserverFunc(func(ctx context.Context, event Event) {
		attrs := []any{
			slog.String("engine", event.Engine),
			slog.String("target_type", event.TargetType),
			slog.String("target", event.Target),
			slog.String("op", event.Operation),
			slog.Time("started_at", event.StartedAt),
			slog.Duration("duration", event.Duration),
			slog.String("status", slogStatus(event.Err)),
		}
		if event.Err != nil {
			attrs = append(attrs, slog.Any("err", event.Err))
		}

		logger.Log(ctx, options.level, options.message, attrs...)
	})
}

func normalizeSlogLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}

func slogStatus(err error) string {
	if err == nil {
		return "ok"
	}
	return "error"
}
