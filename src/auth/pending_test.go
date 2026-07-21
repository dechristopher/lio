package auth

import (
	"testing"
	"time"
)

// TestPendingLifecycle: issue → resolve (non-consuming) → consume, plus the
// rejections for unknown/empty/expired tokens.
func TestPendingLifecycle(t *testing.T) {
	tok := NewPending(42, "drew", "GM")
	if tok == "" {
		t.Fatal("empty pending token")
	}

	// resolvable, and resolving does not consume (a failed factor is retryable);
	// username + title both round-trip into the pending record
	if p, ok := ResolvePending(tok); !ok || p.UserID != 42 || p.Username != "drew" || p.Title != "GM" {
		t.Fatalf("resolve: ok=%v p=%+v", ok, p)
	}
	if _, ok := ResolvePending(tok); !ok {
		t.Fatal("token consumed by resolve")
	}

	// consume ends it
	ConsumePending(tok)
	if _, ok := ResolvePending(tok); ok {
		t.Fatal("token survives consume")
	}

	// unknown + empty
	if _, ok := ResolvePending("nope"); ok {
		t.Error("unknown token resolved")
	}
	if _, ok := ResolvePending(""); ok {
		t.Error("empty token resolved")
	}

	// expired entries are rejected (and dropped) — insert one directly
	expired := NewPending(7, "old", "")
	pendingStore.Lock()
	e := pendingStore.m[expired]
	e.expires = time.Now().Add(-time.Minute)
	pendingStore.m[expired] = e
	pendingStore.Unlock()
	if _, ok := ResolvePending(expired); ok {
		t.Error("expired token resolved")
	}
}
