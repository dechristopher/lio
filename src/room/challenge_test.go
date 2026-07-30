package room

import (
	"testing"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/player"
)

// newChallengeInstance is a waiting room targeted at one account: the creator
// holds White, Black is open, and only invited may take it.
func newChallengeInstance(t *testing.T, invited int64) *Instance {
	t.Helper()
	r := newTestInstance(t, "creator", "")
	r.params.InvitedUserID = &invited
	r.params.InvitedName = "bob"
	return r
}

// TestJoinRequiresTheInvitedAccount is the security property behind a direct
// challenge. The room link is shareable and the notification is not a secret,
// so nothing except this check stops somebody who has the room id from taking a
// seat that was addressed to another player.
func TestJoinRequiresTheInvitedAccount(t *testing.T) {
	const invited int64 = 77

	t.Run("a stranger with an account is refused", func(t *testing.T) {
		r := newChallengeInstance(t, invited)
		other := int64(78)
		if r.Join(player.Identity{UID: "stranger", UserID: &other}, "tok") {
			t.Fatal("an uninvited account took the seat")
		}
		if r.players[octad.Black].ID != "" {
			t.Fatalf("seat was filled by %q", r.players[octad.Black].ID)
		}
	})

	t.Run("an anonymous visitor is refused", func(t *testing.T) {
		r := newChallengeInstance(t, invited)
		// An anonymous session holds no account, so it can never be the invitee —
		// and must not slip through a nil-vs-nil comparison.
		if r.Join(player.Identity{UID: "anon"}, "tok") {
			t.Fatal("an anonymous visitor took an invited seat")
		}
	})

	t.Run("the invited account is admitted", func(t *testing.T) {
		r := newChallengeInstance(t, invited)
		id := invited
		if !r.Join(player.Identity{UID: "bob-uid", UserID: &id}, "tok") {
			t.Fatal("the invited account was refused its own challenge")
		}
		if got := r.players[octad.Black].ID; got != "bob-uid" {
			t.Fatalf("black seat = %q, want bob-uid", got)
		}
	})

	t.Run("a room with no invitation still admits anyone", func(t *testing.T) {
		r := newTestInstance(t, "creator", "")
		if !r.Join(player.Identity{UID: "stranger"}, "tok") {
			t.Fatal("an open challenge refused an ordinary joiner")
		}
	})
}

// TestIsInvited covers the advisory form the page render and the decline
// endpoint use. It must answer true for every account when the room carries no
// invitation, which is what lets the callers ask unconditionally.
func TestIsInvited(t *testing.T) {
	const invited int64 = 90
	r := newChallengeInstance(t, invited)

	id := invited
	other := int64(91)
	if !r.IsInvited(&id) {
		t.Error("the invitee is not recognised")
	}
	if r.IsInvited(&other) {
		t.Error("a stranger holds the invitation")
	}
	if r.IsInvited(nil) {
		t.Error("an anonymous visitor holds the invitation")
	}

	open := newTestInstance(t, "creator", "")
	if !open.IsInvited(nil) || !open.IsInvited(&other) {
		t.Error("a room with no invitation must admit everybody")
	}
}

// TestChallengeJoinRegistersBusy: taking an invited seat makes both players
// unavailable for further challenges. Without this the invitee would keep being
// offered as a target while sitting in the game they just accepted.
func TestChallengeJoinRegistersBusy(t *testing.T) {
	const creator int64 = 101
	const invited int64 = 102

	r := newChallengeInstance(t, invited)
	r.players[octad.White].UserID = func() *int64 { id := creator; return &id }()
	r.stateMu.Lock()
	r.setBusySeats()
	r.stateMu.Unlock()
	t.Cleanup(r.clearBusySeats)

	if !AccountBusy(creator) {
		t.Fatal("the challenger is not busy while waiting")
	}
	if AccountBusy(invited) {
		t.Fatal("the invitee is busy before accepting")
	}

	id := invited
	if !r.Join(player.Identity{UID: "bob-uid", UserID: &id}, "tok") {
		t.Fatal("the invited account was refused")
	}
	if !AccountBusy(invited) {
		t.Fatal("the invitee is still challengeable after accepting")
	}
}
