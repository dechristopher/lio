package handlers

import (
	"strings"
	"testing"
)

// TestChallengeLink locks the shape the accepted-challenge cleanup depends on.
//
// A challenge notification is written with this link (notifyChallenge) and
// retired by matching on it (retireChallengeNotification ->
// db.MarkChallengeReadForRoom, a link = $2 equality). A link carrying a query
// or a fragment, or built differently on one of the two sides, makes that
// UPDATE match nothing — silently — and every accepted challenge stays lit in
// the recipient's bell with an Accept button for a game they are already
// playing. That is the whole reason both sides call this one function.
func TestChallengeLink(t *testing.T) {
	const id = "Ab3xY9"

	link := challengeLink(id)
	if link != "/"+id {
		t.Fatalf("challengeLink(%q) = %q, want %q", id, link, "/"+id)
	}
	// the match is an equality in SQL, so anything a browser would append (or a
	// caller would decorate the link with) breaks it
	if strings.ContainsAny(link, "?#") {
		t.Fatalf("challengeLink(%q) = %q: a decorated link cannot match by equality", id, link)
	}
	// the room the accepter is redirected to is this same path
	if got := strings.TrimPrefix(link, "/"); got != id {
		t.Fatalf("room id does not round-trip out of %q: got %q", link, got)
	}
}

// TestChallengeToRetire covers the guard in front of that cleanup: the write
// only happens for a direct challenge accepted by the account it names.
func TestChallengeToRetire(t *testing.T) {
	invited := int64(7)
	other := int64(9)

	cases := []struct {
		name    string
		invited *int64
		joiner  *int64
		want    string
	}{
		{"invited account accepts", &invited, &invited, "/room1"},
		// an ordinary open room names nobody: no notification can exist for it,
		// so the join must not spend a query
		{"open room", nil, &invited, ""},
		// an anonymous joiner holds no account, so holds no notifications
		{"anonymous joiner", &invited, nil, ""},
		{"neither", nil, nil, ""},
		// defence in depth: RoomJoinHandler already refuses this join, and the
		// query is scoped to the joiner's own rows either way
		{"some other account", &invited, &other, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			link, ok := challengeToRetire(tc.invited, tc.joiner, "room1")
			if ok != (tc.want != "") {
				t.Fatalf("ok = %v, want %v", ok, tc.want != "")
			}
			if link != tc.want {
				t.Fatalf("link = %q, want %q", link, tc.want)
			}
		})
	}
}
