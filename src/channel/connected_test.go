package channel

import (
	"strings"
	"testing"
)

// mine filters a Connected() walk down to one test's own uids. The directory is
// process-global and every test in this package tracks sockets on it, so a bare
// length assertion would depend on what else has run.
func mine(prefix string, all map[string]Account) map[string]Account {
	out := make(map[string]Account)
	for uid, acct := range all {
		if strings.HasPrefix(uid, prefix) {
			out[uid] = acct
		}
	}
	return out
}

// TestConnectedSpansChannelsAndDedupesSessions: presence is a walk of the whole
// directory, because a person's one socket can be on any channel — the room
// they are playing in, the home page's TV stream, or the notification channel.
// Several tabs of one session are one entry: it is one person.
func TestConnectedSpansChannelsAndDedupesSessions(t *testing.T) {
	const p = "conn-span-"

	room := Map.GetSockMap("connected-room")
	tv := Map.GetSockMap("connected-tv")
	t.Cleanup(func() { room.Cleanup(); tv.Cleanup() })

	room.Track(NewSocket(nil, p+"player", "c1", "", Account{ID: 1, Name: "nova"}))
	// two tabs of one session, on two different channels
	room.Track(NewSocket(nil, p+"reader", "c1", "", Account{ID: 2, Name: "zed"}))
	tv.Track(NewSocket(nil, p+"reader", "c2", "", Account{ID: 2, Name: "zed"}))
	// a signed-out visitor: present, but with no identity to name
	tv.Track(NewSocket(nil, p+"anon", "c1", "", Account{}))

	got := mine(p, Connected())
	if len(got) != 3 {
		t.Fatalf("Connected returned %d sessions, want 3: %+v", len(got), got)
	}
	if got[p+"reader"].Name != "zed" {
		t.Errorf("reader = %+v, want the account carried on both its tabs", got[p+"reader"])
	}
	if got[p+"anon"].ID != 0 || got[p+"anon"].Name != "" {
		t.Errorf("anonymous session = %+v, want the zero Account", got[p+"anon"])
	}
}

// TestConnectedPrefersTheNamedRecord: a session keeps its uid when it signs in,
// so a socket opened before the login still carries the zero Account. The
// roster must show the person, not the anonymous record that happens to be
// walked first.
func TestConnectedPrefersTheNamedRecord(t *testing.T) {
	const p = "conn-login-"

	before := Map.GetSockMap("connected-before")
	after := Map.GetSockMap("connected-after")
	t.Cleanup(func() { before.Cleanup(); after.Cleanup() })

	before.Track(NewSocket(nil, p+"uid", "c1", "", Account{}))
	after.Track(NewSocket(nil, p+"uid", "c2", "", Account{ID: 5, Name: "nova"}))

	got := mine(p, Connected())
	if len(got) != 1 {
		t.Fatalf("Connected returned %d sessions, want 1: %+v", len(got), got)
	}
	if got[p+"uid"].Name != "nova" {
		t.Fatalf("session = %+v, want the signed-in record", got[p+"uid"])
	}
}

// TestConnectedSkipsUnidentifiedSockets: an upgrade with no identity is closed
// immediately (ws.closeNoIdentity), but nothing may key presence on the empty
// uid it would otherwise contribute — every one of them would collapse into a
// single phantom visitor.
func TestConnectedSkipsUnidentifiedSockets(t *testing.T) {
	sm := Map.GetSockMap("connected-noid")
	t.Cleanup(func() { sm.Cleanup() })

	sm.Track(NewSocket(nil, "", "c1", "", Account{}))

	if _, present := Connected()[""]; present {
		t.Fatal("Connected included a socket with no uid")
	}
}
