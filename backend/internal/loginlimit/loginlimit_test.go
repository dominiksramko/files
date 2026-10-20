package loginlimit

import (
	"fmt"
	"testing"
	"time"
)

// newTestLimiter returns a limiter whose clock is *now.
func newTestLimiter(now *time.Time) *Limiter {
	limiter := New()
	limiter.now = func() time.Time { return *now }
	return limiter
}

// fail makes count attempts that all fail (Begin without Succeeded).
func fail(t *testing.T, limiter *Limiter, address, username string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if _, ok := limiter.Begin(address, username); !ok {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
}

func TestAddressLimit(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	limiter := newTestLimiter(&now)
	for i := 0; i < MaxFailures; i++ {
		fail(t, limiter, "203.0.113.9", "user"+string(rune('a'+i)), 1) // different usernames
		now = now.Add(time.Minute)
	}
	wait, ok := limiter.Begin("203.0.113.9", "someone")
	if ok {
		t.Fatal("11th attempt from the address went through")
	}
	if wait != 5*time.Minute { // the first failure (12:00) leaves the window at 12:15; it's 12:10
		t.Errorf("wait = %v, want 5m", wait)
	}
	if _, ok := limiter.Begin("198.51.100.7", "someone"); !ok {
		t.Error("another address was refused")
	}
	now = now.Add(5 * time.Minute)
	if _, ok := limiter.Begin("203.0.113.9", "someone"); !ok {
		t.Error("still refused once the oldest failure left the window")
	}
}

func TestSuccessClearsFailures(t *testing.T) {
	now := time.Now()
	limiter := newTestLimiter(&now)
	fail(t, limiter, "203.0.113.9", "ann", MaxFailures-1)
	limiter.Begin("203.0.113.9", "ann")
	limiter.Succeeded("203.0.113.9", "ann")
	fail(t, limiter, "203.0.113.9", "ann", MaxFailures) // a full allowance again
}

func TestUsernameSlowsDown(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	limiter := newTestLimiter(&now)
	for i := 0; i < MaxFailures; i++ {
		fail(t, limiter, "203.0.113."+string(rune('0'+i)), "admin", 1) // spread over addresses
	}
	wait, ok := limiter.Begin("198.51.100.7", "ADMIN") // any address, any case
	if ok || wait != SlowInterval {
		t.Fatalf("Begin = %v, %v; want refused for %v", wait, ok, SlowInterval)
	}
	now = now.Add(SlowInterval)
	if _, ok := limiter.Begin("198.51.100.7", "admin"); !ok {
		t.Fatal("no attempt let through after a minute")
	}
	if _, ok := limiter.Begin("198.51.100.8", "admin"); ok {
		t.Error("a second attempt in the same minute went through")
	}
	if _, ok := limiter.Begin("198.51.100.7", "bob"); !ok {
		t.Error("another username was slowed down")
	}
}

func TestParallelAttemptsCount(t *testing.T) {
	// Each attempt counts as soon as it begins, before its password is checked.
	now := time.Now()
	limiter := newTestLimiter(&now)
	results := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func() {
			_, ok := limiter.Begin("203.0.113.9", "ann")
			results <- ok
		}()
	}
	allowed := 0
	for i := 0; i < 50; i++ {
		if <-results {
			allowed++
		}
	}
	if allowed != MaxFailures {
		t.Errorf("%d parallel attempts allowed, want %d", allowed, MaxFailures)
	}
}

func TestIPv6CountedPerNetwork(t *testing.T) {
	now := time.Now()
	limiter := newTestLimiter(&now)
	for i := 0; i < MaxFailures; i++ {
		fail(t, limiter, fmt.Sprintf("2001:db8:1:2::%x", i+1), fmt.Sprintf("user%d", i), 1)
	}
	if _, ok := limiter.Begin("2001:db8:1:2:ffff::1", "other"); ok {
		t.Error("another address in the same /64 went through")
	}
	if _, ok := limiter.Begin("2001:db8:1:3::1", "other"); !ok {
		t.Error("a different /64 was refused")
	}
}

func TestOldRecordsAreDropped(t *testing.T) {
	now := time.Now()
	limiter := newTestLimiter(&now)
	fail(t, limiter, "203.0.113.9", "ann", 3)
	now = now.Add(Window + cleanupInterval)
	limiter.Begin("198.51.100.7", "bob") // triggers the cleanup
	if _, ok := limiter.byAddress["203.0.113.9"]; ok {
		t.Error("stale address record kept")
	}
	if _, ok := limiter.byUsername["ann"]; ok {
		t.Error("stale username record kept")
	}
}
