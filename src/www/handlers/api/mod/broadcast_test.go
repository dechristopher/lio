package mod

import (
	"strings"
	"testing"
)

// cleanChoices decides whether a message asks a question at all, so its edge
// cases are the difference between an announcement and a row that sits in every
// account's bell until it is answered. Both composers run through it — the
// broadcast and the one-player message — so a rule stated here holds for both.

func TestCleanChoicesNormalizes(t *testing.T) {
	out, err := cleanChoices([]string{" Yes ", "No", ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 || out[0] != "Yes" || out[1] != "No" {
		t.Errorf("cleanChoices = %v, want [Yes No] — trimmed, in order, blanks dropped", out)
	}
}

// No options is the ordinary message, and it must stay distinguishable from a
// question: nil is the value the column holds for one, and the CHECK constraint
// refuses an empty array outright.
func TestCleanChoicesEmptyMeansNoQuestion(t *testing.T) {
	// The client splits the operator's comma-separated field before sending, so
	// what arrives here is the entries — and a trailing comma arrives as one
	// blank among them.
	for name, in := range map[string][]string{
		"nil":      nil,
		"empty":    {},
		"blanks":   {"", "   "},
		"trailing": {""},
	} {
		out, err := cleanChoices(in)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
		if out != nil {
			t.Errorf("%s: cleanChoices = %v, want nil — a question with no options "+
				"could never be answered, so it could never be cleared", name, out)
		}
	}
}

// Two options reading the same would tally as one, and the operator could never
// tell which button was pressed: the answer is stored as its own label, not as
// an index. The comparison is case-insensitive because "Yes" and "yes" are the
// same button to everybody except the database.
func TestCleanChoicesRefusesDuplicates(t *testing.T) {
	if _, err := cleanChoices([]string{"Yes", "yes"}); err == nil {
		t.Fatal("cleanChoices accepted two options that read the same")
	}
}

func TestCleanChoicesBounds(t *testing.T) {
	if _, err := cleanChoices([]string{"a", "b", "c", "d", "e"}); err == nil {
		t.Error("cleanChoices accepted more options than a notification row can show")
	}
	long := strings.Repeat("x", maxChoiceLength+1)
	if _, err := cleanChoices([]string{long}); err == nil {
		t.Error("cleanChoices accepted an option that is a sentence")
	}
	if _, err := cleanChoices([]string{strings.Repeat("x", maxChoiceLength)}); err != nil {
		t.Errorf("cleanChoices refused an option at the limit: %v", err)
	}
}
