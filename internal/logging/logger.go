package logging

import (
	"context"
	"log/slog"
	"os"

	"criticalsys.net/service-gateway/internal/config"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

func New(cfg config.LoggingConfig) *slog.Logger {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return base.With("request_id", requestID)
	}
	return base
}
