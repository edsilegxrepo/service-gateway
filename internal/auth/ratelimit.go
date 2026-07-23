package auth

import (
	"sync"
	"time"
)

const (
	maxBuckets     = 10000
	bucketIdleTime = 10 * time.Minute
)

type RateLimiter struct {
	mu     sync.Mutex
	tokens map[string]*bucket
	rate   float64
	burst  int
	done   chan struct{}
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

func NewRateLimiter(requestsPerMin, burstSize int) *RateLimiter {
	if requestsPerMin <= 0 {
		requestsPerMin = 60
	}
	if burstSize <= 0 {
		burstSize = 10
	}
	rl := &RateLimiter{
		tokens: make(map[string]*bucket),
		rate:   float64(requestsPerMin) / 60.0,
		burst:  burstSize,
		done:   make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Close() {
	close(rl.done)
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.done:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, b := range rl.tokens {
				if now.Sub(b.lastCheck) > bucketIdleTime {
					delete(rl.tokens, key)
				}
			}
			rl.mu.Unlock()
		}
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, exists := rl.tokens[key]
	if !exists {
		if len(rl.tokens) >= maxBuckets {
			rl.evictOldest()
		}
		b = &bucket{
			tokens:    float64(rl.burst),
			lastCheck: time.Now(),
		}
		rl.tokens[key] = b
	}

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.lastCheck = now

	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}

	if b.tokens < 1 {
		return false
	}

	b.tokens--
	return true
}

func (rl *RateLimiter) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	for k, b := range rl.tokens {
		if oldestKey == "" || b.lastCheck.Before(oldestTime) {
			oldestKey = k
			oldestTime = b.lastCheck
		}
	}
	if oldestKey != "" {
		delete(rl.tokens, oldestKey)
	}
}
