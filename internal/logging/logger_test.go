package logging

import (
	"context"
	"log/slog"
	"testing"

	"github.com/edsilegxrepo/service-gateway/internal/config"
)

func TestNewLoggerJSON(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
	}

	logger := New(cfg)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestNewLoggerText(t *testing.T) {
	cfg := config.LoggingConfig{
		Level:  "debug",
		Format: "text",
	}

	logger := New(cfg)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		got := parseLevel(tt.input)
		if got != tt.expected {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestWithRequestID(t *testing.T) {
	ctx := context.Background()
	ctx = WithRequestID(ctx, "req-123")

	val := ctx.Value(RequestIDKey)
	if val != "req-123" {
		t.Errorf("RequestID = %v, want req-123", val)
	}
}

func TestFromContext(t *testing.T) {
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	base := New(cfg)

	ctx := WithRequestID(context.Background(), "req-456")
	logger := FromContext(ctx, base)

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestFromContextNoRequestID(t *testing.T) {
	cfg := config.LoggingConfig{Level: "info", Format: "json"}
	base := New(cfg)

	logger := FromContext(context.Background(), base)

	if logger != base {
		t.Error("expected same logger when no request ID")
	}
}
