package auth

import (
	"testing"
	"time"
)

// TestValidateUsername exercises the pattern and reserved list.
func TestValidateUsername(t *testing.T) {
	valid := []string{"drew", "Drew_42", "a1-b2", "abc", "x2345678901234567890"}
	for _, u := range valid {
		if err := ValidateUsername(u); err != nil {
			t.Errorf("valid username rejected: %q: %v", u, err)
		}
	}
	invalid := []string{
		"", "ab", "x23456789012345678901", // length
		"-lead", "_lead", // bad first char
		"has space", "uh.oh", "émile", // charset
	}
	for _, u := range invalid {
		if err := ValidateUsername(u); err == nil {
			t.Errorf("invalid username accepted: %q", u)
		}
	}
	reserved := []string{"anonymous", "ANONYMOUS", "Bot", "admin", "lioctad", "You"}
	for _, u := range reserved {
		if err := ValidateUsername(u); err == nil {
			t.Errorf("reserved username accepted: %q", u)
		}
	}
}

// TestTokenRoundTrip: a minted token's cookie form hashes back to the same
// storage hash, and malformed cookie values are rejected.
func TestTokenRoundTrip(t *testing.T) {
	token, hash := NewToken()
	got, ok := hashToken(token)
	if !ok || got != hash {
		t.Fatalf("token round-trip failed ok=%v", ok)
	}
	if _, ok := hashToken(""); ok {
		t.Error("empty token hashed")
	}
	if _, ok := hashToken("not!!b64"); ok {
		t.Error("malformed token hashed")
	}
	if _, ok := hashToken("dG9vLXNob3J0"); ok {
		t.Error("short token hashed")
	}
	// two mints never collide
	token2, hash2 := NewToken()
	if token == token2 || hash == hash2 {
		t.Error("token mint collision")
	}
}

// TestLimiter drives the fixed-window limiter with an injected clock.
func TestLimiter(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(3, time.Minute)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("attempt %d denied within limit", i+1)
		}
	}
	if l.Allow("k") {
		t.Fatal("4th attempt allowed")
	}
	// other keys unaffected
	if !l.Allow("other") {
		t.Fatal("independent key denied")
	}
	// window rollover resets
	now = now.Add(61 * time.Second)
	if !l.Allow("k") {
		t.Fatal("attempt denied after window rollover")
	}
}
