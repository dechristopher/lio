package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// Single-use recovery codes (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4): the escape
// hatch when a second factor is unavailable. A fresh set is shown to the user
// exactly once — only SHA-256 hashes persist. The codes are 80 bits of
// crypto/rand each, high-entropy enough that a fast hash suffices (unlike
// passwords, there is nothing low-entropy to brute-force). Regenerating
// replaces the whole set; disabling the last factor clears it.

const (
	// RecoveryCodeCount is how many codes a fresh set contains.
	RecoveryCodeCount = 10

	// recoveryCodeBytes is the entropy per code (80 bits).
	recoveryCodeBytes = 10
)

// recoveryEncoding is unpadded base32 (Crockford-ish A–Z2–7): 10 bytes → 16
// chars, no ambiguous padding, case-folded on input.
var recoveryEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateRecoveryCodes returns a fresh batch of display codes and their
// storage hashes (parallel slices). The plaintext is returned to the caller to
// show once and then discard; only the hashes are ever persisted.
func GenerateRecoveryCodes() (plain []string, hashes [][]byte) {
	plain = make([]string, RecoveryCodeCount)
	hashes = make([][]byte, RecoveryCodeCount)
	for i := range plain {
		raw := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(raw); err != nil {
			panic("auth: crypto/rand failed: " + err.Error())
		}
		code := formatRecoveryCode(recoveryEncoding.EncodeToString(raw))
		plain[i] = code
		hashes[i] = HashRecoveryCode(code)
	}
	return plain, hashes
}

// HashRecoveryCode normalizes a user-entered code (case-fold, strip grouping)
// and returns its SHA-256 — the storage/lookup key. Normalization makes entry
// forgiving of the display grouping and case.
func HashRecoveryCode(code string) []byte {
	sum := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return sum[:]
}

// formatRecoveryCode groups a raw 16-char code into 4-char blocks for legible
// display ("ABCD-EFGH-IJKL-MNOP"), lower-cased for friendlier reading.
func formatRecoveryCode(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for i := 0; i < len(s); i += 4 {
		if i > 0 {
			b.WriteByte('-')
		}
		end := i + 4
		if end > len(s) {
			end = len(s)
		}
		b.WriteString(s[i:end])
	}
	return b.String()
}

// normalizeRecoveryCode strips grouping/whitespace and upper-cases, so a code
// hashes identically however the user typed it (dashes, spaces, or bare).
func normalizeRecoveryCode(code string) string {
	var b strings.Builder
	for _, r := range code {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 32) // to upper
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		}
		// drop everything else (dashes, spaces)
	}
	return b.String()
}
