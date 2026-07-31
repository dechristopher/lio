package handlers

import (
	"testing"

	"github.com/dechristopher/lio/message"
)

// TestRosterFor pins what the players card's site-wide section actually shows
// (arch/FOLLOWING.md Phase 4).
//
// Two exclusions, for two different reasons. A followed member appears in the
// "Following" section and is removed here rather than repeated — a name in both
// would read as two people, and would spend the general roster's eight slots on
// people the viewer already knows. The viewer themselves is removed because they
// know they are here, and their own chip is the one row in the list with nothing
// to do.
func TestRosterFor(t *testing.T) {
	all := []message.OnlineMember{
		{ID: 1, Username: "alpha"},
		{ID: 2, Username: "bravo"},
		{ID: 3, Username: "charlie"},
		{ID: 4, Username: "delta"},
	}
	followed := []message.OnlineMember{{ID: 2, Username: "bravo"}}

	names := func(rows []message.OnlineMember) string {
		out := ""
		for _, m := range rows {
			out += m.Username + " "
		}
		return out
	}

	// signed in as charlie, following bravo: neither is in the general list
	if got := names(rosterFor(all, followed, 3)); got != "alpha delta " {
		t.Fatalf("got %q, want \"alpha delta \"", got)
	}

	// signed out: nothing to exclude, and the slice is handed back untouched
	if got := names(rosterFor(all, nil, 0)); got != "alpha bravo charlie delta " {
		t.Fatalf("anonymous viewer: got %q, want all four", got)
	}

	// signed in, following nobody: only the viewer goes
	if got := names(rosterFor(all, nil, 1)); got != "bravo charlie delta " {
		t.Fatalf("got %q, want \"bravo charlie delta \"", got)
	}

	// a viewer who somehow appears in both sets is removed once, not twice
	if got := names(rosterFor(all, []message.OnlineMember{{ID: 1}}, 1)); got != "bravo charlie delta " {
		t.Fatalf("got %q, want \"bravo charlie delta \"", got)
	}

	// everybody excluded: the section legitimately empties, and the card renders
	// only the headings around it
	if got := rosterFor(all, all, 0); len(got) != 0 {
		t.Fatalf("got %d rows, want 0", len(got))
	}
}
