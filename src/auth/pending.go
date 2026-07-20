package auth

import (
	"sync"
	"time"
)

// The MFA-pending token (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4). When a login
// clears the password (first factor) but the account has a second factor
// enabled, the server does NOT log the visitor in — it issues a short-lived
// pending token that merely proves "this password was correct". The token
// grants no access: it is not a session, carries no cookie, and cannot resolve
// to a user.Context. Presenting a valid second factor (TOTP, recovery code, or
// passkey) against the pending token is what finally performs the real session
// Upgrade (auth.Login).
//
// In-process only, like the resolution cache / rate limiter / WebAuthn
// challenge store — single-instance for now; the Redis lio:* namespace is the
// multi-instance escape hatch.

// pendingTTL bounds how long a half-finished login may be completed.
const pendingTTL = 5 * time.Minute

// Pending is a resolved pending-login record: which account passed the first
// factor and the display name to carry into the session on completion.
type Pending struct {
	UserID   int64
	Username string
	expires  time.Time
}

var pendingStore = struct {
	sync.Mutex
	m map[string]Pending
}{m: make(map[string]Pending)}

// NewPending issues a pending token for a user who just passed the password
// factor. The token is a fresh opaque 256-bit value (the same minter sessions
// use), but it is stored only here, never as a session.
func NewPending(userID int64, username string) string {
	token, _ := NewToken()
	now := time.Now()
	pendingStore.Lock()
	// opportunistic sweep so abandoned half-logins can't accrete
	for k, p := range pendingStore.m {
		if now.After(p.expires) {
			delete(pendingStore.m, k)
		}
	}
	pendingStore.m[token] = Pending{
		UserID:   userID,
		Username: username,
		expires:  now.Add(pendingTTL),
	}
	pendingStore.Unlock()
	return token
}

// ResolvePending returns the record for a pending token without consuming it —
// a failed second-factor attempt stays retryable within the TTL. Expired or
// unknown tokens report ok=false.
func ResolvePending(token string) (Pending, bool) {
	if token == "" {
		return Pending{}, false
	}
	pendingStore.Lock()
	defer pendingStore.Unlock()
	p, ok := pendingStore.m[token]
	if !ok || time.Now().After(p.expires) {
		if ok {
			delete(pendingStore.m, token)
		}
		return Pending{}, false
	}
	return p, true
}

// ConsumePending removes a pending token once a second factor has completed the
// login (or on explicit abandonment).
func ConsumePending(token string) {
	pendingStore.Lock()
	delete(pendingStore.m, token)
	pendingStore.Unlock()
}
