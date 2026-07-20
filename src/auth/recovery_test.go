package auth

import (
	"strings"
	"testing"
)

// TestGenerateRecoveryCodes: a fresh set has the right count, unique codes and
// hashes, grouped display, and each code's hash matches HashRecoveryCode.
func TestGenerateRecoveryCodes(t *testing.T) {
	plain, hashes := GenerateRecoveryCodes()
	if len(plain) != RecoveryCodeCount || len(hashes) != RecoveryCodeCount {
		t.Fatalf("count: plain=%d hashes=%d want %d", len(plain), len(hashes), RecoveryCodeCount)
	}
	seen := make(map[string]bool)
	for i, code := range plain {
		if seen[code] {
			t.Fatalf("duplicate code %q", code)
		}
		seen[code] = true
		if !strings.Contains(code, "-") {
			t.Errorf("code not grouped: %q", code)
		}
		if len(hashes[i]) != 32 {
			t.Errorf("hash %d not sha-256 length: %d", i, len(hashes[i]))
		}
		want := HashRecoveryCode(code)
		if string(want) != string(hashes[i]) {
			t.Errorf("hash mismatch for %q", code)
		}
	}
}

// TestHashRecoveryCodeNormalization: a code hashes identically regardless of
// case, grouping dashes, or surrounding whitespace.
func TestHashRecoveryCodeNormalization(t *testing.T) {
	base := HashRecoveryCode("abcd-efgh-ijkl-mnop")
	variants := []string{
		"ABCD-EFGH-IJKL-MNOP",
		"abcdefghijklmnop",
		"  abcd efgh ijkl mnop  ",
		"AbCd-eFgH-iJkL-mNoP",
	}
	for _, v := range variants {
		if string(HashRecoveryCode(v)) != string(base) {
			t.Errorf("normalization mismatch for %q", v)
		}
	}
	// a genuinely different code must not collide
	if string(HashRecoveryCode("zzzz-zzzz-zzzz-zzzz")) == string(base) {
		t.Error("distinct codes collided")
	}
}
