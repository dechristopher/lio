package config

import (
	"testing"
)

// TestNaughty tests that naughty filtering and detection work fine
func TestNaughty(t *testing.T) {
	naughty = loadNaughty()

	if len(naughty) == 0 {
		t.Fatalf("Has the naughty list been moved?")
	}

	if !Naughty("fuck") {
		t.Fatalf("Hmm, naughty list isn't loading properly.")
	}

	if Naughty("kitten") {
		t.Fatalf("Hmm, naughty checking isn't working properly.")
	}
}

// TestNaughtyUsername exercises the length-tiered username matcher: it must
// catch profanity and common evasions while leaving legitimate names that
// merely nest a short banned fragment (the Scunthorpe problem) alone.
func TestNaughtyUsername(t *testing.T) {
	// blocked: whole words, tokens, and light obfuscation.
	blocked := []string{
		"fuck", "Fuck", "shit", "cunt", // whole word, case-insensitive
		"xX_fuck_Xx", // token split on separators
		"fuck123",    // digits as a token boundary
		"a55",        // leetspeak digits -> "ass"
		"5h1t",       // leetspeak -> "shit"
		"f_u_c_k",    // separator padding -> "fuck"
		"bollocks99", // long word, substring pass
		"biiitch",    // repeated-letter padding -> "bitch"
		"wowjackass", // long word nested is still caught
	}
	for _, u := range blocked {
		if !NaughtyUsername(u) {
			t.Errorf("naughty username not caught: %q", u)
		}
	}

	// allowed: the false-positive corpus. These nest a *short* banned fragment
	// (ass/cock/anal/cunt) or an allowlisted word, and must pass.
	allowed := []string{
		"class", "assassin", "compass", "bass",
		"cockburn", "hancock",
		"analyst", "canal", "banal",
		"scunthorpe",
		"therapist",
		"passion",
		"drew", "Drew_42", "kitten", "chessmaster",
	}
	for _, u := range allowed {
		if NaughtyUsername(u) {
			t.Errorf("legitimate username wrongly blocked: %q", u)
		}
	}
}
