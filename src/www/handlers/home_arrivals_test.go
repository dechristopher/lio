package handlers

import (
	"testing"

	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/presence"
)

// arrivalsWithPresence must never write into the slice it was handed.
//
// This is the sharp edge of the whole feature. db.NewestMembers serves a
// process-wide TTL cache, and the slice it returns is shared by every caller
// until that cache expires — the home page's first paint, every digest tick, and
// every other reader. Stamping presence into those elements would pin one
// moment's flags to every later read, so somebody who signed off minutes ago
// would keep a green dot and a challenge sword until the cache turned over.
//
// The bug would be invisible in a single render and would only show up as a
// stale dot nobody could reproduce, which is exactly why it is tested rather
// than commented.
func TestArrivalsWithPresenceDoesNotMutateInput(t *testing.T) {
	cached := []message.NewMember{
		{ID: 1, Username: "here"},
		{ID: 2, Username: "gone"},
	}
	snap := presence.Snapshot{
		Accounts: map[int64]message.OnlineMember{
			1: {ID: 1, Username: "here", Playing: true, Busy: true},
		},
	}

	out := arrivalsWithPresence(cached, snap)

	for i, m := range cached {
		if m.Online || m.Playing || m.Busy {
			t.Fatalf("input element %d was mutated: %+v", i, m)
		}
	}
	if len(out) != 2 {
		t.Fatalf("output length = %d, want 2", len(out))
	}
	if !out[0].Online || !out[0].Playing || !out[0].Busy {
		t.Errorf("online arrival not marked: %+v", out[0])
	}
	if out[1].Online || out[1].Playing || out[1].Busy {
		t.Errorf("absent arrival must carry no presence: %+v", out[1])
	}
}

// The registration facts survive the stamping — an arrival is still the row the
// database returned, with presence added rather than substituted.
func TestArrivalsWithPresenceKeepsRegistrationFields(t *testing.T) {
	cached := []message.NewMember{{ID: 7, Username: "nova"}}
	out := arrivalsWithPresence(cached, presence.Snapshot{
		Accounts: map[int64]message.OnlineMember{7: {ID: 7, Username: "nova"}},
	})
	if len(out) != 1 || out[0].Username != "nova" || out[0].ID != 7 {
		t.Fatalf("registration fields lost: %+v", out)
	}
	if !out[0].Online || out[0].Playing || out[0].Busy {
		t.Errorf("an online but idle arrival must read available: %+v", out[0])
	}
}

// Nobody online is the common case on a quiet site, and an empty list is the
// common case on a fresh database. Neither may panic or invent rows.
func TestArrivalsWithPresenceEdgeCases(t *testing.T) {
	if got := arrivalsWithPresence(nil, presence.Snapshot{}); got != nil {
		t.Errorf("no arrivals must produce no rows, got %+v", got)
	}
	cached := []message.NewMember{{ID: 1, Username: "solo"}}
	got := arrivalsWithPresence(cached, presence.Snapshot{})
	if len(got) != 1 || got[0].Online {
		t.Errorf("nobody online must leave every arrival unmarked, got %+v", got)
	}
}

// Presence is matched on the account id, not the username. The site folds
// presence on the id deliberately (see presence.Online), and matching by name
// here would reintroduce the case-folding question that fold exists to avoid.
func TestArrivalsWithPresenceMatchesOnIDNotName(t *testing.T) {
	cached := []message.NewMember{{ID: 1, Username: "nova"}}
	// same display name, different account
	snap := presence.Snapshot{
		Accounts: map[int64]message.OnlineMember{
			99: {ID: 99, Username: "nova", Playing: true},
		},
	}
	got := arrivalsWithPresence(cached, snap)
	if got[0].Online {
		t.Errorf("a different account sharing a name must not mark the arrival: %+v", got[0])
	}
}
