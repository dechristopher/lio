package presence

import (
	"strconv"
	"strings"
	"testing"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
)

// chanSeq keeps every test's sockets on channels of its own. channel.Map is
// process-global, so two tests sharing a channel name would see each other's
// connections.
var chanSeq int

// connect tracks one socket per uid on a fresh channel and untracks them all
// when the test ends, which is what makes Online see exactly this test's
// visitors. Each uid gets its own connection id, so several tabs of one session
// can be modelled by calling this twice with the same uid.
func connect(t *testing.T, accounts map[string]channel.Account) {
	t.Helper()
	chanSeq++
	name := "presence-test-" + strconv.Itoa(chanSeq)
	sm := channel.Map.GetSockMap(name)
	for uid, acct := range accounts {
		sm.Track(channel.NewSocket(nil, uid, "c1", "", acct))
	}
	t.Cleanup(func() { sm.Cleanup() })
}

// anon is the account a signed-out visitor's socket carries.
var anon = channel.Account{}

// acctIDs numbers test accounts in order of first use, keyed the way the unique
// index keys them, so two sessions of one person share an id the way a real
// pair of logins does.
var acctIDs = map[string]int64{}

// named builds the account a signed-in visitor's socket carries.
func named(username string) channel.Account {
	key := strings.ToLower(username)
	id, ok := acctIDs[key]
	if !ok {
		id = int64(len(acctIDs) + 1)
		acctIDs[key] = id
	}
	return channel.Account{ID: id, Name: username}
}

func TestOnlineCountsConnectedSessions(t *testing.T) {
	connect(t, map[string]channel.Account{"a": anon, "b": anon})
	if got := Online(nil, 5).Total; got != 2 {
		t.Fatalf("Total = %d, want 2", got)
	}
}

// Presence is a property of a connection, so a site with no sockets open is
// empty however many seats the room registry still holds.
func TestSeatWithoutSocketIsNotOnline(t *testing.T) {
	connect(t, nil)
	seated := map[string]message.OnlineMember{
		"ghost": {Username: "ghost", Playing: true, Busy: true},
	}
	if got := Online(seated, 5).Total; got != 0 {
		t.Fatalf("Total = %d, want 0 (a dropped connection is not presence)", got)
	}
}

// Several tabs of one session are one person, not one per socket.
func TestExtraTabsCountOnce(t *testing.T) {
	chanSeq++
	sm := channel.Map.GetSockMap("presence-test-tabs-" + strconv.Itoa(chanSeq))
	sm.Track(channel.NewSocket(nil, "u1", "c1", "", anon))
	sm.Track(channel.NewSocket(nil, "u1", "c2", "", anon))
	t.Cleanup(func() { sm.Cleanup() })

	if got := Online(nil, 5).Total; got != 1 {
		t.Fatalf("Total = %d, want 1", got)
	}
}

// A session connected on two channels at once (the handoff between a room
// socket and /socket/me overlaps briefly) is still one person.
func TestSameSessionOnTwoChannelsCountsOnce(t *testing.T) {
	connect(t, map[string]channel.Account{"u1": named("nova")})
	connect(t, map[string]channel.Account{"u1": named("nova")})

	snap := Online(nil, 5)
	if snap.Total != 1 {
		t.Fatalf("Total = %d, want 1", snap.Total)
	}
	if len(snap.Members) != 1 {
		t.Fatalf("Members = %d, want 1", len(snap.Members))
	}
}

// The roster names accounts from their sockets and counts the rest as
// anonymous.
func TestOnlineNamesMembersAndCountsAnon(t *testing.T) {
	connect(t, map[string]channel.Account{
		"u1": named("nova"),
		"u2": anon,
		"u3": named("zed"),
		"u4": anon,
	})
	seated := map[string]message.OnlineMember{
		"u3": {Username: "zed", Playing: true, Busy: true},
	}

	snap := Online(seated, 5)
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

// A spectator holds a room socket but no seat. They are online, and — unlike
// under the old HTTP inference, which could only name home-page viewers — they
// are named.
func TestConnectedSpectatorIsNamed(t *testing.T) {
	connect(t, map[string]channel.Account{"watcher": named("nova")})

	snap := Online(nil, 5)
	if len(snap.Members) != 1 || snap.Members[0].Username != "nova" {
		t.Fatalf("Members = %+v, want the named spectator nova", snap.Members)
	}
	if snap.Members[0].Playing || snap.Members[0].Busy {
		t.Fatal("a spectator must not read as playing or busy")
	}
}

// Somebody on their own waiting page holds a wait-channel socket, not a room
// one. They must read as busy — they are committed to the next game — without
// reading as playing.
func TestWaitingCreatorIsBusyNotPlaying(t *testing.T) {
	connect(t, map[string]channel.Account{"u1": named("nova")})
	seated := map[string]message.OnlineMember{
		"u1": {Username: "nova", Busy: true},
	}

	snap := Online(seated, 5)
	if len(snap.Members) != 1 {
		t.Fatalf("Members = %d, want 1", len(snap.Members))
	}
	if !snap.Members[0].Busy || snap.Members[0].Playing {
		t.Fatalf("Members[0] = %+v, want busy but not playing", snap.Members[0])
	}
}

// One account signed in twice (laptop + phone, so two uids) is one player: one
// chip in the roster and one head in the count.
func TestSameAccountAcrossSessionsCountsOnce(t *testing.T) {
	connect(t, map[string]channel.Account{
		"uid-laptop": named("nova"),
		"uid-phone":  named("Nova"), // display casing differs; identity does not
		"uid-anon":   anon,
	})

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
	connect(t, map[string]channel.Account{
		"uid-laptop": named("nova"), // browsing
		"uid-phone":  named("nova"), // seated below
	})
	seated := map[string]message.OnlineMember{
		"uid-phone": {Username: "nova", Playing: true, Busy: true},
	}

	snap := Online(seated, 5)
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
	connect(t, map[string]channel.Account{
		"u1": named("aaa"),
		"u2": named("bbb"),
		"u3": named("ccc"),
		"u4": anon,
	})

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
