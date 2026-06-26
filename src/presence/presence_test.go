package presence

import (
	"testing"
	"time"
)

// reset clears the package-level poller map between tests since presence state
// is process-global.
func reset() {
	mu.Lock()
	pollers = make(map[string]time.Time)
	mu.Unlock()
}

func TestTouchAndOnline(t *testing.T) {
	reset()
	Touch("a")
	Touch("b")
	if got := Online(nil); got != 2 {
		t.Fatalf("Online = %d, want 2", got)
	}
}

func TestTouchEmptyIgnored(t *testing.T) {
	reset()
	Touch("")
	if got := Online(nil); got != 0 {
		t.Fatalf("Online = %d, want 0", got)
	}
}

// A user who is both polling the home page and sitting in a room must count
// once, not twice.
func TestOnlineUnionsInRoomWithoutDoubleCounting(t *testing.T) {
	reset()
	Touch("a") // also in a room below
	Touch("c") // home only
	inRoom := map[string]struct{}{"a": {}, "b": {}}
	if got := Online(inRoom); got != 3 { // a, b, c
		t.Fatalf("Online = %d, want 3", got)
	}
}

func TestStalePollersExpireAndArePruned(t *testing.T) {
	reset()
	Touch("fresh")
	mu.Lock()
	pollers["stale"] = time.Now().Add(-2 * ttl)
	mu.Unlock()

	if got := Online(nil); got != 1 {
		t.Fatalf("Online = %d, want 1 (stale excluded)", got)
	}

	mu.Lock()
	_, stillThere := pollers["stale"]
	mu.Unlock()
	if stillThere {
		t.Fatal("stale poller was not pruned on read")
	}
}
