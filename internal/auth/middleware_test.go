package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/edsilegxrepo/service-gateway/internal/config"
)

func TestIPAllowed(t *testing.T) {
	cfg := config.AuthConfig{
		IPRules: config.IPRulesConfig{
			Enabled:   true,
			AllowList: []string{"192.168.1.0/24", "10.0.0.1"},
			DenyList:  []string{"192.168.1.100"},
		},
	}

	m := &Middleware{
		config:    cfg,
		allowNets: parseNetworks(cfg.IPRules.AllowList),
		denyNets:  parseNetworks(cfg.IPRules.DenyList),
	}

	tests := []struct {
		ip      string
		allowed bool
	}{
		{"192.168.1.1", true},
		{"192.168.1.50", true},
		{"192.168.1.100", false},
		{"10.0.0.1", true},
		{"10.0.0.2", false},
		{"172.16.0.1", false},
	}

	for _, tt := range tests {
		if got := m.isIPAllowed(tt.ip); got != tt.allowed {
			t.Errorf("isIPAllowed(%s) = %v, want %v", tt.ip, got, tt.allowed)
		}
	}
}

func TestIPAllowedNoRules(t *testing.T) {
	m := &Middleware{
		config: config.AuthConfig{
			IPRules: config.IPRulesConfig{Enabled: true},
		},
	}

	if !m.isIPAllowed("1.2.3.4") {
		t.Error("with no allow list, all IPs should be allowed")
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		expected   string
	}{
		{"direct", "192.168.1.1:12345", "", "192.168.1.1"},
		{"xff single", "10.0.0.1:80", "203.0.113.50", "203.0.113.50"},
		{"xff chain", "10.0.0.1:80", "203.0.113.50, 70.41.3.18, 150.172.238.178", "203.0.113.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}

			got := getClientIP(req)
			if got != tt.expected {
				t.Errorf("getClientIP() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestAuthMiddlewareMissingHeader(t *testing.T) {
	m := NewMiddleware(config.AuthConfig{}, nil)

	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m := NewMiddleware(config.AuthConfig{
		Keys: map[string]config.APIKey{
			"valid-key": {EncryptedSecret: "correct-secret"},
		},
	}, logger)

	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestParseNetworks(t *testing.T) {
	nets := parseNetworks([]string{"192.168.1.0/24", "10.0.0.1", "invalid"})

	if len(nets) != 2 {
		t.Errorf("expected 2 valid networks, got %d", len(nets))
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m := NewMiddleware(config.AuthConfig{
		Keys: map[string]config.APIKey{
			"valid-key": {
				EncryptedSecret: "correct-secret",
				Services:        []string{"*"},
			},
		},
	}, logger)

	called := false
	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer correct-secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("handler should have been called with valid token")
	}
}

func TestAuthMiddlewareIPDenied(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.AuthConfig{
		IPRules: config.IPRulesConfig{
			Enabled:  true,
			DenyList: []string{"192.168.1.1"},
		},
		Keys: map[string]config.APIKey{
			"key": {EncryptedSecret: "secret"},
		},
	}

	m := NewMiddleware(cfg, logger)

	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestAuthMiddlewareRateLimited(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.AuthConfig{
		RateLimit: config.RateLimitConfig{
			Enabled:        true,
			RequestsPerMin: 60,
			BurstSize:      1,
		},
		Keys: map[string]config.APIKey{
			"key": {EncryptedSecret: "secret"},
		},
	}

	m := NewMiddleware(cfg, logger)

	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest("GET", "/", nil)
	req1.Header.Set("Authorization", "Bearer secret")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer secret")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", rr2.Code)
	}
}

func TestAuthMiddlewareBearerRequired(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	m := NewMiddleware(config.AuthConfig{}, logger)

	handler := m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for non-Bearer auth", rr.Code)
	}
}

func TestGetAPIKey(t *testing.T) {
	ctx := context.WithValue(context.Background(), APIKeyContextKey, &APIKeyInfo{
		Services: []string{"svc1"},
	})

	key := GetAPIKey(ctx)
	if key == nil {
		t.Fatal("expected non-nil key")
	}
	if len(key.Services) != 1 {
		t.Errorf("Services = %v, want [svc1]", key.Services)
	}
}

func TestGetAPIKeyNil(t *testing.T) {
	key := GetAPIKey(context.Background())
	if key != nil {
		t.Error("expected nil key from empty context")
	}
}

func TestGetKeyName(t *testing.T) {
	ctx := context.WithValue(context.Background(), KeyNameContextKey, "my-key")

	name := GetKeyName(ctx)
	if name != "my-key" {
		t.Errorf("KeyName = %s, want my-key", name)
	}
}

func TestGetKeyNameEmpty(t *testing.T) {
	name := GetKeyName(context.Background())
	if name != "" {
		t.Errorf("KeyName = %s, want empty", name)
	}
}
