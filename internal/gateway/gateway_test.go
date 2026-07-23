package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/edsilegxrepo/service-gateway/internal/auth"
	"github.com/edsilegxrepo/service-gateway/internal/config"
	"github.com/edsilegxrepo/service-gateway/internal/services"
)

func TestHealthEndpoint(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}

	if _, ok := resp["version"]; !ok {
		t.Error("response should include version")
	}

	if _, ok := resp["uptime"]; !ok {
		t.Error("response should include uptime")
	}
}

func TestReadyEndpoint(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	requestID := rr.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("expected X-Request-ID header")
	}

	if len(requestID) < 4 {
		t.Errorf("request ID too short: %s", requestID)
	}
}

func TestRequestIDPreserved(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("X-Request-ID", "custom-request-id")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") != "custom-request-id" {
		t.Error("should preserve client-provided request ID")
	}
}

func TestAPIRequiresAuth(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/api/v1/test-service/query", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAPIRequiresNonce(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/api/v1/test-service/query", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (missing nonce)", rr.Code)
	}
}

func TestNonceEndpoint(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/api/v1/auth/nonce", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if resp["nonce"] == "" {
		t.Error("expected nonce in response")
	}
}

func TestServiceNotFound(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	nonce := getNonce(t, handler)

	req := httptest.NewRequest("GET", "/api/v1/nonexistent/query", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	req.Header.Set("X-Nonce", nonce)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestIsServiceAllowed(t *testing.T) {
	gw := newTestGateway()

	tests := []struct {
		services []string
		target   string
		allowed  bool
	}{
		{[]string{"svc1", "svc2"}, "svc1", true},
		{[]string{"svc1", "svc2"}, "svc3", false},
		{[]string{"*"}, "anything", true},
		{[]string{}, "anything", true},
		{nil, "anything", true},
	}

	for _, tt := range tests {
		key := &auth.APIKeyInfo{Services: tt.services}
		if got := gw.isServiceAllowed(key, tt.target); got != tt.allowed {
			t.Errorf("isServiceAllowed(%v, %s) = %v, want %v", tt.services, tt.target, got, tt.allowed)
		}
	}
}

func newTestGateway() *Gateway {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.AuthConfig{
		Keys: map[string]config.APIKey{
			"test-key": {
				EncryptedSecret: "test-secret",
				Services:        []string{"*"},
			},
		},
		NonceTTL: 5 * time.Minute,
	}

	return New(Config{
		Registry:   services.NewRegistry(logger),
		Auth:       auth.NewMiddleware(cfg, logger),
		NonceStore: auth.NewNonceStore(cfg.NonceTTL),
		Logger:     logger,
		StartTime:  time.Now(),
		Version:    "test",
	})
}

func getNonce(t *testing.T, handler http.Handler) string {
	req := httptest.NewRequest("GET", "/api/v1/auth/nonce", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	return resp["nonce"]
}

func TestHealthEndpointContentType(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

func TestReadyEndpointContentType(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

func TestAPIEmptyServiceName(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	nonce := getNonce(t, handler)

	req := httptest.NewRequest("GET", "/api/v1/", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	req.Header.Set("X-Nonce", nonce)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestIsServiceAllowedNilKey(t *testing.T) {
	gw := newTestGateway()

	if !gw.isServiceAllowed(nil, "any-service") {
		t.Error("nil key should allow all services")
	}
}

func TestLoggingMiddlewareSkipsHealth(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	if id1 == id2 {
		t.Error("request IDs should be unique")
	}

	if len(id1) < 4 {
		t.Errorf("request ID too short: %s", id1)
	}
}

func TestResponseWriter(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rr, status: 200}

	rw.WriteHeader(201)
	if rw.status != 201 {
		t.Errorf("status = %d, want 201", rw.status)
	}

	n, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 5 {
		t.Errorf("bytes written = %d, want 5", n)
	}
	if rw.bytes != 5 {
		t.Errorf("rw.bytes = %d, want 5", rw.bytes)
	}
}

func TestHealthEndpointTimestamp(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if _, ok := resp["timestamp"]; !ok {
		t.Error("response should include timestamp")
	}
}

func TestReadyEndpointChecks(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if _, ok := resp["checks"]; !ok {
		t.Error("response should include checks")
	}
}

func TestMultipleRequestIDs(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		id := generateRequestID()
		if ids[id] {
			t.Errorf("duplicate request ID: %s", id)
		}
		ids[id] = true
	}
}

func TestReadyEndpointDegraded(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	registry := services.NewRegistry(logger)

	services.RegisterType("failing-test", func(name string, cfg json.RawMessage) (services.Service, error) {
		return &failingService{name: name}, nil
	})

	tmpDir := t.TempDir()
	svcDir := filepath.Join(tmpDir, "fail-svc")
	if err := os.MkdirAll(svcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svcDir, "config.json"), []byte(`{"type": "failing-test"}`), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := registry.LoadServices(tmpDir); err != nil {
		t.Fatalf("LoadServices failed: %v", err)
	}

	cfg := config.AuthConfig{
		Keys: map[string]config.APIKey{
			"test-key": {EncryptedSecret: "test-secret", Services: []string{"*"}},
		},
		NonceTTL: 5 * time.Minute,
	}

	gw := New(Config{
		Registry:   registry,
		Auth:       auth.NewMiddleware(cfg, logger),
		NonceStore: auth.NewNonceStore(cfg.NonceTTL),
		Logger:     logger,
		StartTime:  time.Now(),
		Version:    "test",
	})

	handler := gw.Handler()

	req := httptest.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 for degraded", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if resp["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", resp["status"])
	}
}

type failingService struct {
	name string
}

func (s *failingService) Name() string                                     { return s.name }
func (s *failingService) Type() string                                     { return "failing-test" }
func (s *failingService) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
func (s *failingService) HealthCheck(ctx context.Context) error {
	return fmt.Errorf("service unhealthy")
}
func (s *failingService) Close() error { return nil }

func TestReloadEndpoint(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := services.NewRegistry(logger)

	reloadCalled := false
	reloadFn := func() error {
		reloadCalled = true
		return nil
	}

	gw := New(Config{
		Registry: registry,
		Logger:   logger,
		ReloadFn: reloadFn,
	})

	handler := gw.Handler()

	req := httptest.NewRequest("POST", "/_admin/reload", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	if !reloadCalled {
		t.Error("expected reload function to be called")
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if resp["status"] != "reloaded" {
		t.Errorf("status = %s, want reloaded", resp["status"])
	}
}

func TestReloadEndpointNotImplemented(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := services.NewRegistry(logger)

	gw := New(Config{
		Registry: registry,
		Logger:   logger,
		ReloadFn: nil,
	})

	handler := gw.Handler()

	req := httptest.NewRequest("POST", "/_admin/reload", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rr.Code)
	}
}

func TestReloadEndpointForbiddenForExternal(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := services.NewRegistry(logger)

	gw := New(Config{
		Registry: registry,
		Logger:   logger,
		ReloadFn: func() error { return nil },
	})

	handler := gw.Handler()

	req := httptest.NewRequest("POST", "/_admin/reload", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 Forbidden for external IP", rr.Code)
	}
}

func TestReloadEndpointForbiddenForForwardedIP(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := services.NewRegistry(logger)

	gw := New(Config{
		Registry: registry,
		Logger:   logger,
		ReloadFn: func() error { return nil },
	})

	handler := gw.Handler()

	// Test 1: External IP in XFF
	req1 := httptest.NewRequest("POST", "/_admin/reload", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	req1.Header.Set("X-Forwarded-For", "203.0.113.195")
	rr1 := httptest.NewRecorder()

	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 Forbidden for forwarded external IP", rr1.Code)
	}

	// Test 2: Spoofed loopback IP in XFF
	req2 := httptest.NewRequest("POST", "/_admin/reload", nil)
	req2.RemoteAddr = "127.0.0.1:12345"
	req2.Header.Set("X-Forwarded-For", "127.0.0.1")
	rr2 := httptest.NewRecorder()

	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 Forbidden for spoofed loopback XFF", rr2.Code)
	}
}

type mockService struct {
	name string
}

func (s *mockService) Name() string { return s.name }
func (s *mockService) Type() string { return "mock-test" }
func (s *mockService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("mock response"))
}
func (s *mockService) HealthCheck(ctx context.Context) error { return nil }
func (s *mockService) Close() error                          { return nil }

func TestAPIExecuteService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry := services.NewRegistry(logger)

	services.RegisterType("mock-test", func(name string, cfg json.RawMessage) (services.Service, error) {
		return &mockService{name: name}, nil
	})

	tmpDir := t.TempDir()
	svcDir := filepath.Join(tmpDir, "mock-svc")
	if err := os.MkdirAll(svcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svcDir, "config.json"), []byte(`{"type": "mock-test"}`), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := registry.LoadServices(tmpDir); err != nil {
		t.Fatalf("LoadServices failed: %v", err)
	}

	cfg := config.AuthConfig{
		Keys: map[string]config.APIKey{
			"test-key": {EncryptedSecret: "test-secret", Services: []string{"*"}},
		},
		NonceTTL: 5 * time.Minute,
	}

	gw := New(Config{
		Registry:   registry,
		Auth:       auth.NewMiddleware(cfg, logger),
		NonceStore: auth.NewNonceStore(cfg.NonceTTL),
		Logger:     logger,
		StartTime:  time.Now(),
		Version:    "test",
	})

	handler := gw.Handler()
	nonce := getNonce(t, handler)

	req := httptest.NewRequest("GET", "/api/v1/mock-svc/query", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	req.Header.Set("X-Nonce", nonce)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	if rr.Body.String() != "mock response" {
		t.Errorf("body = %s, want 'mock response'", rr.Body.String())
	}
}

func TestMetricsEndpoint(t *testing.T) {
	gw := newTestGateway()
	handler := gw.Handler()

	req1 := httptest.NewRequest("GET", "/health", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	req2 := httptest.NewRequest("GET", "/api/v1/nonexistent/path", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	reqM := httptest.NewRequest("GET", "/metrics", nil)
	rrM := httptest.NewRecorder()
	handler.ServeHTTP(rrM, reqM)

	if rrM.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rrM.Code)
	}

	var metrics map[string]any
	if err := json.Unmarshal(rrM.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("Unmarshal metrics failed: %v", err)
	}

	if metrics["status"] != "ok" {
		t.Errorf("status = %v, want ok", metrics["status"])
	}

	total, ok := metrics["requests_total"].(float64)
	if !ok || total < 3 {
		t.Errorf("requests_total = %v, want >= 3", metrics["requests_total"])
	}
}
