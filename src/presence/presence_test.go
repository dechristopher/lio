package presence

import (
	"testing"
	"time"

	"github.com/dechristopher/lio/message"
)

// reset clears the package-level poller map between tests since presence state
// is process-global.
func reset() {
	mu.Lock()
	pollers = make(map[string]poller)
	mu.Unlock()
}

// anon is the identity an anonymous visitor is touched with.
var anon = message.OnlineMember{}

// named builds an account identity for a test poller/seat.
func named(username string) message.OnlineMember {
	return message.OnlineMember{Username: username}
}

// seats builds an in-room set from uid → identity pairs.
func seats(pairs map[string]message.OnlineMember) map[string]message.OnlineMember {
	return pairs
}

func TestTouchAndOnline(t *testing.T) {
	reset()
	Touch("a", anon)
	Touch("b", anon)
	if got := Online(nil, 5).Total; got != 2 {
		t.Fatalf("Total = %d, want 2", got)
	}
}

func TestTouchEmptyIgnored(t *testing.T) {
	reset()
	Touch("", anon)
	if got := Online(nil, 5).Total; got != 0 {
		t.Fatalf("Total = %d, want 0", got)
	}
}

// A user who is both polling the home page and sitting in a room must count
// once, not twice.
func TestOnlineUnionsInRoomWithoutDoubleCounting(t *testing.T) {
	reset()
	Touch("a", anon) // also in a room below
	Touch("c", anon) // home only
	inRoom := seats(map[string]message.OnlineMember{"a": anon, "b": anon})
	if got := Online(inRoom, 5).Total; got != 3 { // a, b, c
		t.Fatalf("Total = %d, want 3", got)
	}
}

func TestStalePollersExpireAndArePruned(t *testing.T) {
	reset()
	Touch("fresh", anon)
	mu.Lock()
	pollers["stale"] = poller{seen: time.Now().Add(-2 * ttl)}
	mu.Unlock()

	if got := Online(nil, 5).Total; got != 1 {
		t.Fatalf("Total = %d, want 1 (stale excluded)", got)
	}

	mu.Lock()
	_, stillThere := pollers["stale"]
	mu.Unlock()
	if stillThere {
		t.Fatal("stale poller was not pruned on read")
	}
}

// The roster names accounts from both sources and counts the rest as anonymous.
func TestOnlineNamesMembersAndCountsAnon(t *testing.T) {
	reset()
	Touch("u1", named("nova"))
	Touch("u2", anon)
	inRoom := seats(map[string]message.OnlineMember{
		"u3": {Username: "zed", Playing: true},
		"u4": anon,
	})

	snap := Online(inRoom, 5)
	if snap.Total != 4 {
		t.Fatalf("Total = %d, want 4", snap.Total)
	}
	if len(snap.Members) != 2 {
		t.Fatalf("Members = %d, want 2", len(snap.Members))
	}
	if snap.Anon != 2 {
		t.Fatalf("Anon = %d, want 2", snap.Anon)
	}
	// seated players sort ahead of browsers
	if snap.Members[0].Username != "zed" || !snap.Members[0].Playing {
		t.Fatalf("Members[0] = %+v, want the seated player zed", snap.Members[0])
	}
	if snap.Members[1].Username != "nova" {
		t.Fatalf("Members[1] = %+v, want nova", snap.Members[1])
	}
}

// The same account polling the home page while seated must be listed once, and
// as Playing — the in-room record is the interesting one.
func TestSeatedRecordWinsOverPoller(t *testing.T) {
	reset()
	Touch("u1", named("nova")) // browsing record, no Playing flag
	inRoom := seats(map[string]message.OnlineMember{
		"u1": {Username: "nova", Playing: true},
	})

	snap := Online(inRoom, 5)
	if snap.Total != 1 {
		t.Fatalf("Total = %d, want 1", snap.Total)
	}
	if len(snap.Members) != 1 {
		t.Fatalf("Members = %d, want 1", len(snap.Members))
	}
	if !snap.Members[0].Playing {
		t.Fatal("seated identity lost to the home-page poller record")
	}
	if snap.Anon != 0 {
		t.Fatalf("Anon = %d, want 0", snap.Anon)
	}
}

// One account signed in twice (laptop + phone, so two uids) is one player: one
// chip in the roster and one head in the count.
func TestSameAccountAcrossSessionsCountsOnce(t *testing.T) {
	reset()
	Touch("uid-laptop", named("nova"))
	Touch("uid-phone", named("Nova")) // display casing differs; identity does not
	Touch("uid-anon", anon)

	snap := Online(nil, 5)
	if len(snap.Members) != 1 {
		t.Fatalf("Members = %d, want 1 (same account twice)", len(snap.Members))
	}
	if snap.Total != 2 {
		t.Fatalf("Total = %d, want 2 (one member + one anonymous)", snap.Total)
	}
	if snap.Anon != 1 {
		t.Fatalf("Anon = %d, want 1", snap.Anon)
	}
}

// ...and if any one of those sessions is seated, the single chip says playing.
func TestPlayingWinsAcrossAccountSessions(t *testing.T) {
	reset()
	Touch("uid-laptop", named("nova")) // browsing
	inRoom := seats(map[string]message.OnlineMember{
		"uid-phone": {Username: "nova", Playing: true}, // seated
	})

	snap := Online(inRoom, 5)
	if len(snap.Members) != 1 {
		t.Fatalf("Members = %d, want 1", len(snap.Members))
	}
	if !snap.Members[0].Playing {
		t.Fatal("member with a seated session is not marked playing")
	}
	if snap.Total != 1 {
		t.Fatalf("Total = %d, want 1", snap.Total)
	}
}

// Capping the displayed roster must not distort the anonymous tally: Anon is
// the count of account-less visitors, not "everyone who didn't fit".
func TestLimitCapsMembersButNotAnonTally(t *testing.T) {
	reset()
	Touch("u1", named("aaa"))
	Touch("u2", named("bbb"))
	Touch("u3", named("ccc"))
	Touch("u4", anon)

	snap := Online(nil, 2)
	if len(snap.Members) != 2 {
		t.Fatalf("Members = %d, want 2 (capped)", len(snap.Members))
	}
	if snap.Total != 4 {
		t.Fatalf("Total = %d, want 4", snap.Total)
	}
	if snap.Anon != 1 {
		t.Fatalf("Anon = %d, want 1 (uncapped tally)", snap.Anon)
	}
}
