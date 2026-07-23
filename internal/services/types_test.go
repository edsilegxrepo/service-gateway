package services

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestRegisterType(t *testing.T) {
	RegisterType("test-type", func(name string, cfg json.RawMessage) (Service, error) {
		return &mockService{name: name}, nil
	})

	types := RegisteredTypes()
	found := false
	for _, typ := range types {
		if typ == "test-type" {
			found = true
			break
		}
	}

	if !found {
		t.Error("registered type not found in RegisteredTypes()")
	}
}

func TestCreateService(t *testing.T) {
	RegisterType("mock", func(name string, cfg json.RawMessage) (Service, error) {
		return &mockService{name: name}, nil
	})

	cfg := json.RawMessage(`{"type": "mock", "description": "test"}`)
	svc, err := CreateService("test-service", cfg)
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}

	if svc.Name() != "test-service" {
		t.Errorf("Name() = %s, want test-service", svc.Name())
	}
}

func TestCreateServiceUnknownType(t *testing.T) {
	cfg := json.RawMessage(`{"type": "nonexistent"}`)
	_, err := CreateService("test", cfg)
	if err == nil {
		t.Error("expected error for unknown service type")
	}
}

func TestCreateServiceInvalidJSON(t *testing.T) {
	cfg := json.RawMessage(`{invalid json}`)
	_, err := CreateService("test", cfg)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

type mockService struct {
	BaseService
	name string
}

func (m *mockService) Name() string                                     { return m.name }
func (m *mockService) Type() string                                     { return "mock" }
func (m *mockService) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
func (m *mockService) HealthCheck(ctx context.Context) error            { return nil }
func (m *mockService) Close() error                                     { return nil }
