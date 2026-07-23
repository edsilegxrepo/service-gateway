package auth

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(60, 5)

	for i := 0; i < 5; i++ {
		if !rl.Allow("key1") {
			t.Errorf("request %d should be allowed (within burst)", i+1)
		}
	}

	if rl.Allow("key1") {
		t.Error("request 6 should be denied (burst exhausted)")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(6000, 1)

	if !rl.Allow("key1") {
		t.Error("first request should be allowed")
	}

	if rl.Allow("key1") {
		t.Error("second request should be denied")
	}

	time.Sleep(15 * time.Millisecond)

	if !rl.Allow("key1") {
		t.Error("request after refill should be allowed")
	}
}

func TestRateLimiterIsolation(t *testing.T) {
	rl := NewRateLimiter(60, 2)

	rl.Allow("key1")
	rl.Allow("key1")

	if !rl.Allow("key2") {
		t.Error("key2 should have its own bucket")
	}
}

func TestRateLimiterDefaults(t *testing.T) {
	rl := NewRateLimiter(0, 0)

	if rl.rate != 1.0 {
		t.Errorf("default rate should be 1.0, got %f", rl.rate)
	}
	if rl.burst != 10 {
		t.Errorf("default burst should be 10, got %d", rl.burst)
	}
}
