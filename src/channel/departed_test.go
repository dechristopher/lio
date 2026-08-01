package channel

import (
	"testing"
	"time"
)

// UnTrack remembers who left, so presence can still name somebody who was here
// a moment ago (see departed.go).
func TestUnTrackRecordsDeparture(t *testing.T) {
	ForgetDepartures()
	t.Cleanup(ForgetDepartures)

	sm := Map.GetSockMap("departed-test-record")
	t.Cleanup(sm.Cleanup)
	sm.Track(NewSocket(nil, "uid-1", "c1", "", Account{ID: 7, Name: "nova"}))
	sm.UnTrack("uid-1", "c1")

	gone := Departed(time.Minute)
	d, ok := gone["uid-1"]
	if !ok {
		t.Fatal("uid-1 missing from Departed after its socket closed")
	}
	if d.Account.ID != 7 || d.Account.Name != "nova" {
		t.Fatalf("departure carries %+v, want the account the socket held", d.Account)
	}
}

// Coming back forgets the departure. Every page holds its own socket, so a
// navigation is a close followed by an open — without this, following a link
// would leave a stamp saying the visitor had gone.
func TestTrackClearsDeparture(t *testing.T) {
	ForgetDepartures()
	t.Cleanup(ForgetDepartures)

	sm := Map.GetSockMap("departed-test-clear")
	t.Cleanup(sm.Cleanup)
	sm.Track(NewSocket(nil, "uid-2", "c1", "", Account{ID: 8}))
	sm.UnTrack("uid-2", "c1")
	sm.Track(NewSocket(nil, "uid-2", "c2", "", Account{ID: 8}))

	if _, ok := Departed(time.Minute)["uid-2"]; ok {
		t.Fatal("a session that came back is still remembered as departed")
	}
}

// The window is the caller's, not the store's: a short window hides an older
// departure without deleting it, so a caller asking a longer question still
// gets its answer.
func TestDepartedFiltersToTheWindow(t *testing.T) {
	ForgetDepartures()
	t.Cleanup(ForgetDepartures)

	departed.Lock()
	departed.at = map[string]Departure{
		"uid-old": {Account: Account{ID: 9}, At: time.Now().Add(-10 * time.Minute)},
	}
	departed.Unlock()

	if _, ok := Departed(time.Minute)["uid-old"]; ok {
		t.Fatal("a departure older than the window was returned")
	}
	if _, ok := Departed(time.Hour)["uid-old"]; !ok {
		t.Fatal("a short window deleted a record a longer one still wanted")
	}
}

// Anything past the retention bound is dropped on read, so the map cannot grow
// without limit on a long-running process.
func TestDepartedPrunesPastRetention(t *testing.T) {
	ForgetDepartures()
	t.Cleanup(ForgetDepartures)

	departed.Lock()
	departed.at = map[string]Departure{
		"uid-ancient": {At: time.Now().Add(-departedRetention - time.Minute)},
	}
	departed.Unlock()

	Departed(time.Hour)

	departed.Lock()
	n := len(departed.at)
	departed.Unlock()
	if n != 0 {
		t.Fatalf("stored departures = %d, want 0 after pruning", n)
	}
}
