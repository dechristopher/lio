package presence

import (
	"strconv"
	"strings"
	"testing"
	"time"

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
func connect(t *testing.T, accounts map[string]channel.Account) *channel.SockMap {
	t.Helper()
	// Departures are process-global and outlive the SockMap that produced them,
	// so a test that closes a socket would leave its uid "recently active" for
	// the next one. Clear on both sides of the test: entering, in case something
	// else left a stamp, and leaving, so this test's own do not escape.
	channel.ForgetDepartures()
	chanSeq++
	name := "presence-test-" + strconv.Itoa(chanSeq)
	sm := channel.Map.GetSockMap(name)
	for uid, acct := range accounts {
		sm.Track(channel.NewSocket(nil, uid, "c1", "", acct))
	}
	t.Cleanup(func() {
		sm.Cleanup()
		channel.ForgetDepartures()
	})
	return sm
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
	if got := Online(nil, 5, nil).Total; got != 2 {
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
	if got := Online(seated, 5, nil).Total; got != 0 {
		t.Fatalf("Total = %d, want 0 (a dropped connection is not presence)", got)
	}
}

// Several tabs of one session are one person, not one per socket.
func TestExtraTabsCountOnce(t *testing.T) {
	sm := connect(t, nil)
	sm.Track(channel.NewSocket(nil, "u1", "c1", "", anon))
	sm.Track(channel.NewSocket(nil, "u1", "c2", "", anon))

	if got := Online(nil, 5, nil).Total; got != 1 {
		t.Fatalf("Total = %d, want 1", got)
	}
}

// A session connected on two channels at once (the handoff between a room
// socket and /socket/me overlaps briefly) is still one person.
func TestSameSessionOnTwoChannelsCountsOnce(t *testing.T) {
	connect(t, map[string]channel.Account{"u1": named("nova")})
	connect(t, map[string]channel.Account{"u1": named("nova")})

	snap := Online(nil, 5, nil)
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

	snap := Online(seated, 5, nil)
	if snap.Total != 4 {
		t.Fatalf("Total = %d, want 4", snap.Total)
	}
	if len(snap.Members) != 2 {
		t.Fatalf("Members = %d, want 2", len(snap.Members))
	}
	if snap.Anon != 2 {
		t.Fatalf("Anon = %d, want 2", snap.Anon)
	}
	// Free players sort ahead of seated ones. This used to be the reverse, on
	// the reasoning that a game in progress is the more interesting fact; it is
	// not the fact a visitor is looking for. Somebody at a board cannot be
	// challenged, so leading with them puts the rows with nothing to offer at
	// the top of the list (see SortRoster).
	if snap.Members[0].Username != "nova" || snap.Members[0].Playing {
		t.Fatalf("Members[0] = %+v, want the free player nova", snap.Members[0])
	}
	if snap.Members[1].Username != "zed" || !snap.Members[1].Playing {
		t.Fatalf("Members[1] = %+v, want the seated player zed", snap.Members[1])
	}
}

// A spectator holds a room socket but no seat. They are online, and — unlike
// under the old HTTP inference, which could only name home-page viewers — they
// are named.
func TestConnectedSpectatorIsNamed(t *testing.T) {
	connect(t, map[string]channel.Account{"watcher": named("nova")})

	snap := Online(nil, 5, nil)
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

	snap := Online(seated, 5, nil)
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

	snap := Online(nil, 5, nil)
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

	snap := Online(seated, 5, nil)
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

	snap := Online(nil, 2, nil)
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

// Snapshot.Accounts is the uncapped, id-keyed view the follow feature filters
// (arch/FOLLOWING.md). It must hold every online member even when Members has
// been trimmed for display — a followed player who is fortieth in the site-wide
// order still has to be findable.
func TestSnapshotAccountsIsUncapped(t *testing.T) {
	connect(t, map[string]channel.Account{
		"uid-a": named("alpha"),
		"uid-b": named("bravo"),
		"uid-c": named("charlie"),
		"uid-d": anon,
	})

	snap := Online(nil, 1, nil)
	if len(snap.Members) != 1 {
		t.Fatalf("Members = %d, want 1 (capped)", len(snap.Members))
	}
	if len(snap.Accounts) != 3 {
		t.Fatalf("Accounts = %d, want 3 (uncapped)", len(snap.Accounts))
	}
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		id := acctIDs[name]
		m, ok := snap.Accounts[id]
		if !ok {
			t.Fatalf("%s missing from Accounts", name)
		}
		if m.ID != id {
			t.Fatalf("%s carries ID %d, want %d", name, m.ID, id)
		}
	}
	// the anonymous session is counted, never keyed: it has no account to key on
	if _, ok := snap.Accounts[0]; ok {
		t.Fatal("an anonymous session reached Accounts")
	}
}

// Accounts is produced even when no roster was asked for. /system wants the
// headcount only (limit 0) and takes the early return, but the follow filter
// asks the same question of a snapshot it may not want names from.
func TestAccountsPresentWithoutLimit(t *testing.T) {
	connect(t, map[string]channel.Account{"uid-a": named("delta")})

	snap := Online(nil, 0, nil)
	if len(snap.Members) != 0 {
		t.Fatalf("Members = %d, want 0 at limit 0", len(snap.Members))
	}
	if len(snap.Accounts) != 1 {
		t.Fatalf("Accounts = %d, want 1", len(snap.Accounts))
	}
}

// A seated record that carries its account id folds with the same person's
// socket record rather than standing beside it. This is the production path:
// room.snapshot copies p.UserID onto the member it publishes.
func TestSeatedRecordFoldsByID(t *testing.T) {
	acct := named("echo")
	connect(t, map[string]channel.Account{
		"uid-laptop": acct,
		"uid-phone":  acct,
	})
	seated := map[string]message.OnlineMember{
		"uid-phone": {ID: acct.ID, Username: "echo", Playing: true, Busy: true},
	}

	snap := Online(seated, 5, nil)
	if snap.Total != 1 {
		t.Fatalf("Total = %d, want 1", snap.Total)
	}
	if len(snap.Accounts) != 1 {
		t.Fatalf("Accounts = %d, want 1", len(snap.Accounts))
	}
	if m := snap.Accounts[acct.ID]; !m.Playing || !m.Busy {
		t.Fatalf("seated flags lost in the fold: %+v", m)
	}
}

// ---- the active window (arch/HOME_ACTIVITY_STREAMING.md) ----

// A closed session keeps its place in the roster, and says it has gone. This is
// what makes the panel answer "who is around" rather than "who is connected at
// this exact instant", which empties every time a tab closes.
func TestDepartedSessionStaysInRoster(t *testing.T) {
	sm := connect(t, map[string]channel.Account{"uid-gone": named("golf")})
	sm.UnTrack("uid-gone", "c1")

	snap := Online(nil, 5, nil)
	if len(snap.Members) != 1 || snap.Members[0].Username != "golf" {
		t.Fatalf("Members = %+v, want the departed member golf", snap.Members)
	}
	if snap.Total != 1 {
		t.Fatalf("Total = %d, want 1 (the window counts them)", snap.Total)
	}
}

// Total covers the window; Live does not. The home page's tile reads the first
// and the operator console reads the second, so the two must actually differ
// once somebody has left.
func TestLiveExcludesDepartedWhileTotalIncludesThem(t *testing.T) {
	sm := connect(t, map[string]channel.Account{
		"uid-here": named("hotel"),
		"uid-gone": named("india"),
	})
	sm.UnTrack("uid-gone", "c1")

	snap := Online(nil, 5, nil)
	if snap.Total != 2 {
		t.Fatalf("Total = %d, want 2 (both are inside the window)", snap.Total)
	}
	if snap.Live != 1 {
		t.Fatalf("Live = %d, want 1 (only the open socket)", snap.Live)
	}
}

// Live counts people, not sockets: one account on two devices is one head, the
// same fold Total makes. A count that disagreed with the roster beside it about
// what one person is would be worse than no count.
func TestLiveCountsPeopleNotSessions(t *testing.T) {
	connect(t, map[string]channel.Account{
		"uid-laptop": named("juliet"),
		"uid-phone":  named("juliet"),
		"uid-anon":   anon,
	})

	snap := Online(nil, 5, nil)
	if snap.Live != 2 {
		t.Fatalf("Live = %d, want 2 (one account + one anonymous visitor)", snap.Live)
	}
}

// Navigating is a close followed by an open. Inside navGrace the closed session
// still reads as fully here — no lost dot, no lost challenge button, no jump to
// the bottom of the list on somebody else's screen for the length of a page
// load.
func TestDepartureInsideGraceStillReadsOnline(t *testing.T) {
	sm := connect(t, map[string]channel.Account{"uid-nav": named("kilo")})
	sm.UnTrack("uid-nav", "c1")

	snap := Online(nil, 5, nil)
	if len(snap.Members) != 1 {
		t.Fatalf("Members = %d, want 1", len(snap.Members))
	}
	if !snap.Members[0].Online {
		t.Fatal("a session that closed a moment ago must still read as online")
	}
	if !snap.Members[0].Left.IsZero() {
		t.Fatalf("Left = %v, want zero while the member still reads as online",
			snap.Members[0].Left)
	}
}

// Coming back clears the departure outright, so a visitor who follows a link is
// never both connected and remembered as gone.
func TestReconnectClearsDeparture(t *testing.T) {
	sm := connect(t, map[string]channel.Account{"uid-nav": named("lima")})
	sm.UnTrack("uid-nav", "c1")
	sm.Track(channel.NewSocket(nil, "uid-nav", "c2", "", named("lima")))

	if got := len(channel.Departed(ActiveWindow)); got != 0 {
		t.Fatalf("departures = %d, want 0 after the session came back", got)
	}
	snap := Online(nil, 5, nil)
	if snap.Live != 1 || snap.Total != 1 {
		t.Fatalf("Live/Total = %d/%d, want 1/1", snap.Live, snap.Total)
	}
}

// The fold's window and grace rules, at a fixed instant — the two things that
// cannot be tested by waiting for real time to pass.
func TestSessionsFromWindowAndGrace(t *testing.T) {
	now := time.Now()
	conn := map[string]channel.Account{"uid-live": {ID: 1, Name: "mike"}}
	gone := map[string]channel.Departure{
		"uid-grace": {Account: channel.Account{ID: 2, Name: "november"}, At: now.Add(-5 * time.Second)},
		"uid-away":  {Account: channel.Account{ID: 3, Name: "oscar"}, At: now.Add(-5 * time.Minute)},
	}

	got := sessionsFrom(conn, gone, now)
	if len(got) != 3 {
		t.Fatalf("sessions = %d, want 3", len(got))
	}
	if s := got["uid-live"]; !s.online || !s.live {
		t.Fatalf("uid-live = %+v, want online and live", s)
	}
	if s := got["uid-grace"]; !s.online || s.live {
		t.Fatalf("uid-grace = %+v, want online (inside the grace) but not live", s)
	}
	if s := got["uid-away"]; s.online || s.live {
		t.Fatalf("uid-away = %+v, want neither online nor live", s)
	}
	if s := got["uid-away"]; !s.left.Equal(now.Add(-5 * time.Minute)) {
		t.Fatalf("uid-away left = %v, want the departure time", s.left)
	}
}

// A live socket beats a stale departure stamp for the same uid — another tab
// closed, the person is still here.
func TestSessionsFromPrefersTheLiveRecord(t *testing.T) {
	now := time.Now()
	conn := map[string]channel.Account{"uid-both": {ID: 1, Name: "papa"}}
	gone := map[string]channel.Departure{
		"uid-both": {Account: channel.Account{ID: 1, Name: "papa"}, At: now.Add(-10 * time.Minute)},
	}

	got := sessionsFrom(conn, gone, now)
	if s := got["uid-both"]; !s.online || !s.live || !s.left.IsZero() {
		t.Fatalf("uid-both = %+v, want a live record with no departure", s)
	}
}

// ---- ordering ----

// The four tiers, in the order the question "who can I play right now" is asked
// in: free, waiting, playing, gone.
func TestSortRosterTiers(t *testing.T) {
	members := []message.OnlineMember{
		{ID: 4, Username: "gone", Left: time.Now().Add(-time.Minute)},
		{ID: 3, Username: "playing", Online: true, Playing: true, Busy: true},
		{ID: 2, Username: "waiting", Online: true, Busy: true},
		{ID: 1, Username: "free", Online: true},
	}
	SortRoster(members, nil)

	want := []string{"free", "waiting", "playing", "gone"}
	for i, name := range want {
		if members[i].Username != name {
			t.Fatalf("position %d = %q, want %q (order: %+v)", i, members[i].Username, name, members)
		}
	}
}

// Within a tier, whoever played most recently reads first. That is what
// separates a player who has just finished a game from one who signed in and
// has been reading.
func TestSortRosterRecentPlayFirst(t *testing.T) {
	now := time.Now()
	members := []message.OnlineMember{
		{ID: 1, Username: "alpha", Online: true},
		{ID: 2, Username: "bravo", Online: true},
		{ID: 3, Username: "charlie", Online: true},
	}
	lastPlayed := map[int64]time.Time{
		1: now.Add(-time.Hour),
		3: now.Add(-time.Minute),
	}
	SortRoster(members, lastPlayed)

	want := []string{"charlie", "alpha", "bravo"}
	for i, name := range want {
		if members[i].Username != name {
			t.Fatalf("position %d = %q, want %q (order: %+v)", i, members[i].Username, name, members)
		}
	}
}

// The departed tier orders by when they left, most recent first — the person a
// visitor has only just missed reads first. Their last game does not enter into
// it; when they were here is the more useful fact about somebody who is gone.
func TestSortRosterDepartedByRecency(t *testing.T) {
	now := time.Now()
	members := []message.OnlineMember{
		{ID: 1, Username: "long-gone", Left: now.Add(-10 * time.Minute)},
		{ID: 2, Username: "just-left", Left: now.Add(-time.Minute)},
	}
	SortRoster(members, map[int64]time.Time{1: now})

	if members[0].Username != "just-left" {
		t.Fatalf("order = %+v, want the most recent departure first", members)
	}
}

// A nil recency map is the unconfigured-archive case. It must order by the
// tiers and then alphabetically rather than by map iteration order.
func TestSortRosterWithoutRecencyIsAlphabetical(t *testing.T) {
	members := []message.OnlineMember{
		{ID: 1, Username: "zulu", Online: true},
		{ID: 2, Username: "Alpha", Online: true},
	}
	SortRoster(members, nil)

	if members[0].Username != "Alpha" {
		t.Fatalf("order = %+v, want a case-insensitive alphabetical fallback", members)
	}
}

// The cap trims the roster and says how many it dropped, so a busy evening
// reads as a busy evening rather than as exactly onlineShown people.
func TestMoreCountsWhatTheCapDropped(t *testing.T) {
	connect(t, map[string]channel.Account{
		"u1": named("quebec"),
		"u2": named("romeo"),
		"u3": named("sierra"),
	})

	snap := Online(nil, 1, nil)
	if len(snap.Members) != 1 {
		t.Fatalf("Members = %d, want 1 (capped)", len(snap.Members))
	}
	if snap.More != 2 {
		t.Fatalf("More = %d, want 2", snap.More)
	}
}
