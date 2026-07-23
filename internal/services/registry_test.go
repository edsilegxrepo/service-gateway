package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryLoadServices(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := t.TempDir()
	svcDir := filepath.Join(tmpDir, "test-svc")
	if err := os.MkdirAll(svcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	RegisterType("test-registry", func(name string, cfg json.RawMessage) (Service, error) {
		return &testService{name: name}, nil
	})

	configJSON := `{"type": "test-registry", "description": "test"}`
	if err := os.WriteFile(filepath.Join(svcDir, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	registry := NewRegistry(logger)
	err := registry.LoadServices(tmpDir)
	if err != nil {
		t.Fatalf("LoadServices failed: %v", err)
	}

	svc, exists := registry.Get("test-svc")
	if !exists {
		t.Fatal("service not found")
	}
	if svc.Name() != "test-svc" {
		t.Errorf("Name() = %s, want test-svc", svc.Name())
	}
}

func TestRegistryLoadServicesEmptyDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	tmpDir := t.TempDir()

	registry := NewRegistry(logger)
	err := registry.LoadServices(tmpDir)
	if err != nil {
		t.Fatalf("LoadServices on empty dir failed: %v", err)
	}

	services := registry.Services()
	if len(services) != 0 {
		t.Errorf("expected 0 services, got %d", len(services))
	}
}

func TestRegistryLoadServicesNonexistent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	registry := NewRegistry(logger)
	err := registry.LoadServices("/nonexistent/path")
	if err != nil {
		t.Error("nonexistent dir should not error (just warn)")
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := NewRegistry(logger)

	_, exists := registry.Get("nonexistent")
	if exists {
		t.Error("expected service to not exist")
	}
}

func TestRegistryServices(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := NewRegistry(logger)

	services := registry.Services()
	if services == nil {
		t.Error("Services() should return non-nil map")
	}
}

func TestRegistryCloseAll(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := NewRegistry(logger)

	registry.CloseAll()
}

type testService struct {
	name string
}

func (s *testService) Name() string                                     { return s.name }
func (s *testService) Type() string                                     { return "test" }
func (s *testService) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
func (s *testService) HealthCheck(ctx context.Context) error            { return nil }
func (s *testService) Close() error                                     { return nil }
