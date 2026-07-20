package auth

import (
	"sync"
	"time"
)

// Limiter is a fixed-window request limiter for the login path, keyed by
// caller-chosen strings (IP|username). In-process only — fine single-instance;
// a Redis-backed window is the multi-instance escape hatch, like the other
// in-process auth state (arch/ACCOUNTS_AUTH_RATINGS.md).
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	seen   map[string]*limitWindow
	now    func() time.Time // injectable for tests
}

type limitWindow struct {
	start time.Time
	count int
}

// NewLimiter allows max events per key per window.
func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{
		max:    max,
		window: window,
		seen:   make(map[string]*limitWindow),
		now:    time.Now,
	}
}

// Allow records an attempt for key and reports whether it is within the
// limit. Stale windows are dropped lazily as they are touched, plus a
// full sweep whenever the map grows past a bound (drive-by keys can't
// accrete forever).
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	w := l.seen[key]
	if w == nil || now.Sub(w.start) >= l.window {
		if len(l.seen) > 4096 {
			for k, v := range l.seen {
				if now.Sub(v.start) >= l.window {
					delete(l.seen, k)
				}
			}
		}
		l.seen[key] = &limitWindow{start: now, count: 1}
		return true
	}
	w.count++
	return w.count <= l.max
}

// loginLimiter guards password verification: 10 attempts per IP+username per
// 5 minutes. Generous for humans, hostile to credential stuffing.
var loginLimiter = NewLimiter(10, 5*time.Minute)

// AllowLogin records a login attempt for the given key (IP|username) and
// reports whether it is within the limit.
func AllowLogin(key string) bool {
	return loginLimiter.Allow(key)
}
