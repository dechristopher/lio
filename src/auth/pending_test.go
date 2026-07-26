package auth

import (
	"testing"
	"time"

	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/title"
)

// TestPendingLifecycle: issue → resolve (non-consuming) → consume, plus the
// rejections for unknown/empty/expired tokens.
func TestPendingLifecycle(t *testing.T) {
	acct := AccountInfo{
		UserID:   42,
		Username: "drew",
		Title:    title.Title{Code: "GM", Name: "Grandmaster"},
		Role:     role.Mod,
	}
	tok := NewPending(acct)
	if tok == "" {
		t.Fatal("empty pending token")
	}

	// resolvable, and resolving does not consume (a failed factor is retryable);
	// the whole account identity — name, title and role — round-trips into the
	// pending record, so completing a second factor needs no second user read
	if p, ok := ResolvePending(tok); !ok || p.AccountInfo != acct {
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
	expired := NewPending(AccountInfo{UserID: 7, Username: "old"})
	pendingStore.Lock()
	e := pendingStore.m[expired]
	e.expires = time.Now().Add(-time.Minute)
	pendingStore.m[expired] = e
	pendingStore.Unlock()
	if _, ok := ResolvePending(expired); ok {
		t.Error("expired token resolved")
	}
}
