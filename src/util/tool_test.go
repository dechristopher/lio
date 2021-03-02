package util

import (
	"testing"
)

// TestGenerateCode tests code generation
func TestGenerateCode(t *testing.T) {
	if len(GenerateCode(5, false)) != 5 {
		t.Fatalf("Epic fail, code generation sucks!")
	}
}

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
