package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNonceStore(t *testing.T) {
	store := NewNonceStore(1 * time.Minute)

	nonce, err := store.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if len(nonce) != 32 {
		t.Errorf("expected 32-char nonce, got %d", len(nonce))
	}

	if !store.Validate(nonce) {
		t.Error("valid nonce should pass validation")
	}

	if store.Validate(nonce) {
		t.Error("nonce should not validate twice (replay protection)")
	}
}

func TestNonceUnique(t *testing.T) {
	store := NewNonceStore(1 * time.Minute)

	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		nonce, _ := store.Generate()
		if seen[nonce] {
			t.Errorf("duplicate nonce generated: %s", nonce)
		}
		seen[nonce] = true
	}
}

func TestNonceExpiry(t *testing.T) {
	store := NewNonceStore(10 * time.Millisecond)

	nonce, _ := store.Generate()

	time.Sleep(20 * time.Millisecond)

	if store.Validate(nonce) {
		t.Error("expired nonce should not validate")
	}
}

func TestNonceInvalid(t *testing.T) {
	store := NewNonceStore(1 * time.Minute)

	if store.Validate("nonexistent-nonce") {
		t.Error("invalid nonce should not validate")
	}
}

func TestNonceStoreDefaultTTL(t *testing.T) {
	store := NewNonceStore(0)

	if store.ttl != 5*time.Minute {
		t.Errorf("default TTL = %v, want 5m", store.ttl)
	}
}

func TestNonceHandleGetNonce(t *testing.T) {
	store := NewNonceStore(1 * time.Minute)

	req := httptest.NewRequest("GET", "/auth/nonce", nil)
	rr := httptest.NewRecorder()

	store.HandleGetNonce(rr, req)

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

func TestNonceValidateMiddleware(t *testing.T) {
	store := NewNonceStore(1 * time.Minute)

	called := false
	handler := store.ValidateMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	nonce, _ := store.Generate()

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Nonce", nonce)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("handler should have been called")
	}
}

func TestNonceValidateMiddlewareMissingNonce(t *testing.T) {
	store := NewNonceStore(1 * time.Minute)

	handler := store.ValidateMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestNonceValidateMiddlewareInvalidNonce(t *testing.T) {
	store := NewNonceStore(1 * time.Minute)

	handler := store.ValidateMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Nonce", "invalid-nonce")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}
