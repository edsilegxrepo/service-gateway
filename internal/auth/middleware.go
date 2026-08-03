package auth

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"

	"criticalsys.net/service-gateway/internal/config"
	"criticalsys.net/service-gateway/internal/crypto"
	"criticalsys.net/service-gateway/internal/logging"
)

type contextKey string

const (
	APIKeyContextKey  contextKey = "api_key"
	KeyNameContextKey contextKey = "key_name"
)

type concurrencyLimiter struct {
	max     int
	current int
	mu      sync.Mutex
}

func newConcurrencyLimiter(max int) *concurrencyLimiter {
	return &concurrencyLimiter{max: max}
}

func (c *concurrencyLimiter) Acquire() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current >= c.max {
		return false
	}
	c.current++
	return true
}

func (c *concurrencyLimiter) Release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current > 0 {
		c.current--
	}
}

type Middleware struct {
	config          config.AuthConfig
	logger          *slog.Logger
	secrets         map[string]string
	allowNets       []*net.IPNet
	denyNets        []*net.IPNet
	rateLimiter     *RateLimiter
	keyRateLimiters map[string]*RateLimiter
	keyConcurrency  map[string]*concurrencyLimiter
}

func NewMiddleware(cfg config.AuthConfig, logger *slog.Logger) *Middleware {
	m := &Middleware{
		config:          cfg,
		logger:          logger,
		secrets:         make(map[string]string),
		keyRateLimiters: make(map[string]*RateLimiter),
		keyConcurrency:  make(map[string]*concurrencyLimiter),
	}

	for name, key := range cfg.Keys {
		secret, err := decryptSecret(key.EncryptedSecret)
		if err != nil {
			logger.Error("failed to decrypt API key", "key", name, "error", err)
			continue
		}
		m.secrets[name] = secret

		if key.RateLimitRPM > 0 {
			m.keyRateLimiters[name] = NewRateLimiter(key.RateLimitRPM, key.RateLimitRPM/6+1)
		}
		if key.MaxConcurrent > 0 {
			m.keyConcurrency[name] = newConcurrencyLimiter(key.MaxConcurrent)
		}
	}

	if cfg.IPRules.Enabled {
		m.allowNets = parseNetworks(cfg.IPRules.AllowList)
		m.denyNets = parseNetworks(cfg.IPRules.DenyList)
	}

	if cfg.RateLimit.Enabled {
		m.rateLimiter = NewRateLimiter(cfg.RateLimit.RequestsPerMin, cfg.RateLimit.BurstSize)
	}

	return m
}

func (m *Middleware) Close() {
	if m.rateLimiter != nil {
		m.rateLimiter.Close()
	}
	for _, limiter := range m.keyRateLimiters {
		if limiter != nil {
			limiter.Close()
		}
	}
}

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logging.FromContext(r.Context(), m.logger)

		if m.config.IPRules.Enabled {
			clientIP := getClientIP(r)
			if !m.isIPAllowed(clientIP) {
				log.Warn("IP denied", "client_ip", clientIP)
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Bearer token required", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		keyName, apiKey := m.validateToken(token)
		if keyName == "" {
			log.Warn("invalid token")
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if m.rateLimiter != nil && !m.rateLimiter.Allow(keyName) {
			log.Warn("global rate limit exceeded", "key", keyName)
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		if rl, ok := m.keyRateLimiters[keyName]; ok && !rl.Allow(keyName) {
			log.Warn("per-key rate limit exceeded", "key", keyName)
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		var concLimiter *concurrencyLimiter
		if cl, ok := m.keyConcurrency[keyName]; ok {
			if !cl.Acquire() {
				log.Warn("max concurrent requests exceeded", "key", keyName)
				http.Error(w, "Too many concurrent requests", http.StatusTooManyRequests)
				return
			}
			concLimiter = cl
		}

		ctx := context.WithValue(r.Context(), APIKeyContextKey, apiKey)
		ctx = context.WithValue(ctx, KeyNameContextKey, keyName)

		if concLimiter != nil {
			defer concLimiter.Release()
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) validateToken(token string) (string, *APIKeyInfo) {
	for name, secret := range m.secrets {
		if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1 {
			key := m.config.Keys[name]
			return name, &APIKeyInfo{
				Services:    key.Services,
				Roles:       key.Roles,
				Description: key.Description,
			}
		}
	}
	return "", nil
}

func (m *Middleware) isIPAllowed(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}

	for _, n := range m.denyNets {
		if n.Contains(parsed) {
			return false
		}
	}

	if len(m.allowNets) == 0 {
		return true
	}

	for _, n := range m.allowNets {
		if n.Contains(parsed) {
			return true
		}
	}

	return false
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func parseNetworks(cidrs []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		if !strings.Contains(cidr, "/") {
			cidr += "/32"
		}
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

func decryptSecret(encrypted string) (string, error) {
	return crypto.Decrypt(encrypted)
}

func GetAPIKey(ctx context.Context) *APIKeyInfo {
	if key, ok := ctx.Value(APIKeyContextKey).(*APIKeyInfo); ok {
		return key
	}
	return nil
}

type APIKeyInfo struct {
	Services    []string
	Roles       []string
	Description string
}

func GetKeyName(ctx context.Context) string {
	if name, ok := ctx.Value(KeyNameContextKey).(string); ok {
		return name
	}
	return ""
}
