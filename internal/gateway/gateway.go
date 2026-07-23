package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/edsilegxrepo/service-gateway/internal/auth"
	"github.com/edsilegxrepo/service-gateway/internal/logging"
	"github.com/edsilegxrepo/service-gateway/internal/services"
)

type Gateway struct {
	registry    *services.Registry
	auth        *auth.Middleware
	nonceStore  *auth.NonceStore
	logger      *slog.Logger
	startTime   time.Time
	version     string
	servicesDir string
	reloadFn    func() error
}

type Config struct {
	Registry    *services.Registry
	Auth        *auth.Middleware
	NonceStore  *auth.NonceStore
	Logger      *slog.Logger
	StartTime   time.Time
	Version     string
	ServicesDir string
	ReloadFn    func() error
}

func New(cfg Config) *Gateway {
	return &Gateway{
		registry:    cfg.Registry,
		auth:        cfg.Auth,
		nonceStore:  cfg.NonceStore,
		logger:      cfg.Logger,
		startTime:   cfg.StartTime,
		version:     cfg.Version,
		servicesDir: cfg.ServicesDir,
		reloadFn:    cfg.ReloadFn,
	}
}

func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", g.handleHealth)
	mux.HandleFunc("GET /ready", g.handleReady)
	mux.HandleFunc("POST /_admin/reload", g.handleReload)

	mux.Handle("GET /api/v1/auth/nonce", g.auth.Authenticate(http.HandlerFunc(g.nonceStore.HandleGetNonce)))

	apiHandler := g.auth.Authenticate(
		g.nonceStore.ValidateMiddleware(
			http.HandlerFunc(g.handleAPI),
		),
	)
	mux.Handle("/api/v1/", apiHandler)

	return g.requestIDMiddleware(g.loggingMiddleware(mux))
}

func (g *Gateway) handleReload(w http.ResponseWriter, r *http.Request) {
	if g.reloadFn == nil {
		http.Error(w, "Reload not configured", http.StatusNotImplemented)
		return
	}

	g.logger.Info("reload triggered via HTTP")
	if err := g.reloadFn(); err != nil {
		g.logger.Error("reload failed", "error", err)
		http.Error(w, "Reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "reloaded"}); err != nil {
		g.logger.Error("failed to encode reload response", "error", err)
	}
}

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
		"version":   g.version,
		"uptime":    time.Since(g.startTime).Round(time.Second).String(),
	}); err != nil {
		g.logger.Error("failed to encode health response", "error", err)
	}
}

func (g *Gateway) handleReady(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]any)
	allOK := true

	for name, svc := range g.registry.Services() {
		start := time.Now()
		err := svc.HealthCheck(r.Context())
		latency := time.Since(start)

		if err != nil {
			checks[name] = map[string]any{
				"status": "error",
				"error":  err.Error(),
			}
			allOK = false
		} else {
			checks[name] = map[string]any{
				"status":  "ok",
				"latency": latency.String(),
			}
		}
	}

	status := "ok"
	httpCode := http.StatusOK
	if !allOK {
		status = "degraded"
		httpCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":    status,
		"timestamp": time.Now().UTC(),
		"checks":    checks,
	}); err != nil {
		g.logger.Error("failed to encode ready response", "error", err)
	}
}

func (g *Gateway) handleAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}

	serviceName := parts[0]
	svc, exists := g.registry.Get(serviceName)
	if !exists {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	apiKey := auth.GetAPIKey(r.Context())
	if apiKey != nil && !g.isServiceAllowed(apiKey, serviceName) {
		http.Error(w, "Access denied to service", http.StatusForbidden)
		return
	}

	if len(parts) > 1 {
		r.URL.Path = "/" + parts[1]
	} else {
		r.URL.Path = "/"
	}

	svc.ServeHTTP(w, r)
}

func (g *Gateway) isServiceAllowed(key *auth.APIKeyInfo, serviceName string) bool {
	if key == nil || len(key.Services) == 0 {
		return true
	}
	for _, s := range key.Services {
		if s == serviceName || s == "*" {
			return true
		}
	}
	return false
}

func (g *Gateway) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		ctx := logging.WithRequestID(r.Context(), requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (g *Gateway) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: 200}

		next.ServeHTTP(wrapped, r)

		log := logging.FromContext(r.Context(), g.logger)
		log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"latency_ms", time.Since(start).Milliseconds(),
			"bytes", wrapped.bytes,
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func generateRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "req_fallback"
	}
	return "req_" + hex.EncodeToString(b)
}
