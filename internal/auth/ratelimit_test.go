package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter(60, 5)
	defer rl.Close()

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
	defer rl.Close()

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
	defer rl.Close()

	rl.Allow("key1")
	rl.Allow("key1")

	if !rl.Allow("key2") {
		t.Error("key2 should have its own bucket")
	}
}

func TestRateLimiterDefaults(t *testing.T) {
	rl := NewRateLimiter(0, 0)
	defer rl.Close()

	if rl.rate != 1.0 {
		t.Errorf("default rate should be 1.0, got %f", rl.rate)
	}
	if rl.burst != 10 {
		t.Errorf("default burst should be 10, got %d", rl.burst)
	}
}

func TestRateLimiterClose(t *testing.T) {
	rl := NewRateLimiter(60, 5)
	rl.Close()
}

func TestRateLimiterEvictOldest(t *testing.T) {
	rl := NewRateLimiter(6000, 5)
	defer rl.Close()

	// Fill the map to maxBuckets
	for i := 0; i < maxBuckets; i++ {
		key := fmt.Sprintf("key-%d", i)
		rl.tokens[key] = &bucket{
			tokens:    5.0,
			lastCheck: time.Now().Add(-time.Duration(i) * time.Second),
		}
	}

	// Trigger eviction by adding one more key
	rl.Allow("key-overflow")

	if len(rl.tokens) != maxBuckets {
		t.Errorf("expected map size to stay at %d after overflow, got %d", maxBuckets, len(rl.tokens))
	}
}
