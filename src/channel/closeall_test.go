package channel

import (
	"testing"
	"time"
)

// TestCloseAllNilConnSafe: the shutdown drain's socket sweep must survive
// connections with no live *websocket.Conn (the test double, and any socket
// mid-teardown) — it skips the close frame but still shuts the writer down.
func TestCloseAllNilConnSafe(t *testing.T) {
	sm := Map.GetSockMap("closeall-test")
	s1 := NewSocket(nil, "u1", "c1", "")
	s2 := NewSocket(nil, "u2", "c2", "")
	sm.Track(s1)
	sm.Track(s2)
	t.Cleanup(func() {
		sm.UnTrack("u1", "c1")
		sm.UnTrack("u2", "c2")
	})

	// must not panic on nil connections
	CloseAll(1012, "server restarting")

	for _, s := range []*Socket{s1, s2} {
		select {
		case <-s.closed:
		case <-time.After(time.Second):
			t.Fatal("CloseAll did not close a tracked socket")
		}
	}
}
