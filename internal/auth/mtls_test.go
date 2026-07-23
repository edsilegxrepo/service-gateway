package auth

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edsilegxrepo/service-gateway/internal/config"
)

func TestMTLSDisabled(t *testing.T) {
	m := NewMTLSMiddleware(config.MTLSConfig{Enabled: false})

	handler := m.Validate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (mTLS disabled)", rr.Code)
	}
}

func TestMTLSRequiresTLS(t *testing.T) {
	m := NewMTLSMiddleware(config.MTLSConfig{Enabled: true})

	handler := m.Validate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.TLS = nil
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (TLS required)", rr.Code)
	}
}

func TestMTLSRequiresClientCert(t *testing.T) {
	m := NewMTLSMiddleware(config.MTLSConfig{
		Enabled:       true,
		RequireClient: true,
	})

	handler := m.Validate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: nil,
	}
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (client cert required)", rr.Code)
	}
}

func TestMTLSLoadTLSConfigDisabled(t *testing.T) {
	m := NewMTLSMiddleware(config.MTLSConfig{Enabled: false})

	cfg, err := m.LoadTLSConfig()
	if err != nil {
		t.Fatalf("LoadTLSConfig failed: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config when disabled")
	}
}

func TestMTLSLoadTLSConfigMissingCA(t *testing.T) {
	m := NewMTLSMiddleware(config.MTLSConfig{
		Enabled:      true,
		ClientCAPath: "/nonexistent/ca.pem",
	})

	_, err := m.LoadTLSConfig()
	if err == nil {
		t.Error("expected error for missing CA file")
	}
}

func TestGetClientCN(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	info := &ClientCertInfo{CommonName: "test-cn", Organization: "test-org"}
	ctx := context.WithValue(req.Context(), clientCertKey{}, info)
	req = req.WithContext(ctx)

	cn := GetClientCN(req)
	if cn != "test-cn" {
		t.Errorf("GetClientCN = %s, want test-cn", cn)
	}
}

func TestGetClientOrg(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	info := &ClientCertInfo{CommonName: "test-cn", Organization: "test-org"}
	ctx := context.WithValue(req.Context(), clientCertKey{}, info)
	req = req.WithContext(ctx)

	org := GetClientOrg(req)
	if org != "test-org" {
		t.Errorf("GetClientOrg = %s, want test-org", org)
	}
}

func TestGetClientCert(t *testing.T) {
	ctx := context.Background()
	if info := GetClientCert(ctx); info != nil {
		t.Error("expected nil for context without cert info")
	}

	info := &ClientCertInfo{CommonName: "cn", Organization: "org"}
	ctx = context.WithValue(ctx, clientCertKey{}, info)
	got := GetClientCert(ctx)
	if got == nil || got.CommonName != "cn" {
		t.Errorf("GetClientCert failed, got %v", got)
	}
}
