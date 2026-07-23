package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type NonceStore struct {
	mu     sync.RWMutex
	nonces map[string]nonceEntry
	ttl    time.Duration
	done   chan struct{}
}

type nonceEntry struct {
	createdAt time.Time
	used      bool
}

func NewNonceStore(ttl time.Duration) *NonceStore {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	ns := &NonceStore{
		nonces: make(map[string]nonceEntry),
		ttl:    ttl,
		done:   make(chan struct{}),
	}
	go ns.cleanup()
	return ns
}

func (ns *NonceStore) Close() {
	close(ns.done)
}

func (ns *NonceStore) Generate() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(b)

	ns.mu.Lock()
	ns.nonces[nonce] = nonceEntry{createdAt: time.Now()}
	ns.mu.Unlock()

	return nonce, nil
}

func (ns *NonceStore) Validate(nonce string) bool {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	entry, exists := ns.nonces[nonce]
	if !exists {
		return false
	}

	if entry.used {
		return false
	}

	if time.Since(entry.createdAt) > ns.ttl {
		delete(ns.nonces, nonce)
		return false
	}

	entry.used = true
	ns.nonces[nonce] = entry
	return true
}

func (ns *NonceStore) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ns.done:
			return
		case <-ticker.C:
			ns.mu.Lock()
			now := time.Now()
			for nonce, entry := range ns.nonces {
				if now.Sub(entry.createdAt) > ns.ttl {
					delete(ns.nonces, nonce)
				}
			}
			ns.mu.Unlock()
		}
	}
}

func (ns *NonceStore) HandleGetNonce(w http.ResponseWriter, r *http.Request) {
	nonce, err := ns.Generate()
	if err != nil {
		http.Error(w, "Failed to generate nonce", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"nonce": nonce}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (ns *NonceStore) ValidateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := r.Header.Get("X-Nonce")
		if nonce == "" {
			http.Error(w, "X-Nonce header required", http.StatusBadRequest)
			return
		}

		if !ns.Validate(nonce) {
			http.Error(w, "Invalid or expired nonce", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
