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
