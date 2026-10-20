// Package loginlimit slows down password guessing. Failed logins are counted
// per client address and per username, in memory (a restart clears them):
//
//   - an address with MaxFailures failures within Window is refused until the
//     oldest of them is older than Window;
//   - a username with MaxFailures failures within Window, from anywhere, then
//     takes one attempt per SlowInterval. It isn't locked: that would let
//     anyone keep its owner out, while one attempt a minute stops guessing and
//     still lets the right password in.
package loginlimit

import (
	"net"
	"strings"
	"sync"
	"time"
)

const (
	Window       = 15 * time.Minute
	MaxFailures  = 10
	SlowInterval = time.Minute
	// cleanupInterval: how often counters without recent failures are dropped.
	cleanupInterval = time.Minute
)

type record struct {
	failures    []time.Time // within Window, oldest first
	lastAllowed time.Time   // last attempt let through (slowed-down usernames)
}

type Limiter struct {
	mu          sync.Mutex
	now         func() time.Time
	byAddress   map[string]*record
	byUsername  map[string]*record
	lastCleanup time.Time
}

func New() *Limiter {
	return &Limiter{now: time.Now, byAddress: map[string]*record{}, byUsername: map[string]*record{}}
}

// Begin is called before checking a password. If the attempt may go ahead it
// is counted as a failure straight away — so parallel guesses count too — and
// Succeeded takes it back. Otherwise it returns how long until the next try.
func (l *Limiter) Begin(address, username string) (wait time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.cleanup(now)

	addressRecord := l.recordFor(l.byAddress, addressKey(address), now)
	usernameRecord := l.recordFor(l.byUsername, usernameKey(username), now)

	if len(addressRecord.failures) >= MaxFailures {
		return addressRecord.failures[0].Add(Window).Sub(now), false
	}
	if len(usernameRecord.failures) >= MaxFailures {
		if next := usernameRecord.lastAllowed.Add(SlowInterval); now.Before(next) {
			return next.Sub(now), false
		}
	}

	addressRecord.failures = append(addressRecord.failures, now)
	usernameRecord.failures = append(usernameRecord.failures, now)
	usernameRecord.lastAllowed = now
	return 0, true
}

// Succeeded clears the failures of the address and username: a correct
// password ends any slowdown.
func (l *Limiter) Succeeded(address, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byAddress, addressKey(address))
	delete(l.byUsername, usernameKey(username))
}

// recordFor returns key's record in records, created if needed, with
// failures older than Window dropped.
func (l *Limiter) recordFor(records map[string]*record, key string, now time.Time) *record {
	current, ok := records[key]
	if !ok {
		current = &record{}
		records[key] = current
	}
	current.failures = recent(current.failures, now)
	return current
}

// cleanup drops records without failures in the last Window, at most once per
// cleanupInterval, so addresses and usernames tried once don't pile up.
func (l *Limiter) cleanup(now time.Time) {
	if now.Sub(l.lastCleanup) < cleanupInterval {
		return
	}
	l.lastCleanup = now
	for _, records := range []map[string]*record{l.byAddress, l.byUsername} {
		for key, current := range records {
			if current.failures = recent(current.failures, now); len(current.failures) == 0 {
				delete(records, key)
			}
		}
	}
}

func recent(failures []time.Time, now time.Time) []time.Time {
	first := 0
	for first < len(failures) && now.Sub(failures[first]) >= Window {
		first++
	}
	return failures[first:]
}

// addressKey counts IPv6 clients by their /64 network: whoever has one IPv6
// address usually has the whole /64, and could otherwise rotate through it.
func addressKey(address string) string {
	ip := net.ParseIP(address)
	if ip == nil || ip.To4() != nil {
		return address
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func usernameKey(username string) string {
	return strings.ToLower(username)
}
