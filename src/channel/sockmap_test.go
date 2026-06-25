package channel

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// newTestSocket builds a Socket with no real websocket connection. The tracking
// and listener paths under test never touch the connection (only Enqueue's
// drop-path and WritePump do, neither of which these tests drive), so a nil
// connection is fine here.
func newTestSocket(uid, connID string) *Socket {
	return NewSocket(nil, uid, connID, "")
}

// TestSockMapMultiSocketPerUID is the deterministic guard for the connID-keyed
// model (#1): a uid may hold several connections at once, and untracking one
// connection must not drop the uid while another connection for it is still
// live. This is exactly the reconnect "ghost-disconnect" bug the rewrite fixes.
func TestSockMapMultiSocketPerUID(t *testing.T) {
	s := NewSockMap("multi-test")
	defer s.Cleanup()

	s.Track(newTestSocket("a", "c1"))
	s.Track(newTestSocket("a", "c2"))
	s.Track(newTestSocket("b", "c1"))

	if got := s.Length(); got != 2 {
		t.Fatalf("Length = %d, want 2 distinct uids", got)
	}
	if !s.Connected("a") || !s.Connected("b") {
		t.Fatal("expected a and b connected")
	}
	if s.Connected("c") {
		t.Fatal("c was never tracked")
	}
	if got := len(s.Sockets()); got != 3 {
		t.Fatalf("Sockets() = %d, want 3 connections", got)
	}
	if got := len(s.SocketsFor("a")); got != 2 {
		t.Fatalf("SocketsFor(a) = %d, want 2", got)
	}

	// drop one of a's two connections: a must stay connected (this is the
	// ghost-disconnect fix — the old uid-keyed map would have dropped a here)
	s.UnTrack("a", "c1")
	if !s.Connected("a") {
		t.Fatal("a lost presence after dropping only one of its two connections")
	}
	if got := s.Length(); got != 2 {
		t.Fatalf("Length = %d after partial untrack, want 2", got)
	}

	// drop a's last connection: now a is gone
	s.UnTrack("a", "c2")
	if s.Connected("a") {
		t.Fatal("a still connected after dropping its last connection")
	}
	if got := s.Length(); got != 1 {
		t.Fatalf("Length = %d after a fully untracked, want 1", got)
	}
}

// TestSockMapConcurrentTrackingAndListeners hammers the SockMap tracking and
// crowd-listener plumbing from many goroutines while listeners register,
// consume, and unregister, then tears the map down. Under -race it catches
// concurrent map access and the listener-slice race; it also asserts that the
// post-cleanup notify path (Track/UnTrack racing Cleanup) never panics on a
// closed channel, and that a long-lived listener's range terminates on cleanup.
func TestSockMapConcurrentTrackingAndListeners(t *testing.T) {
	s := NewSockMap("race-test")

	const (
		workers  = 16
		perWork  = 500
		uniqUids = 32
		connsPer = 3
	)

	var wg sync.WaitGroup

	// churn: many goroutines tracking/untracking connections and reading state.
	// Each worker cycles several connIDs per uid to exercise the multi-socket
	// nested map.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWork; i++ {
				uid := fmt.Sprintf("u%d", (w*perWork+i)%uniqUids)
				connID := fmt.Sprintf("c%d", i%connsPer)
				s.Track(newTestSocket(uid, connID))
				_ = s.Connected(uid)
				_ = s.Length()
				_ = s.Sockets()
				_ = s.SocketsFor(uid)
				_ = s.Empty()
				s.UnTrack(uid, connID)
			}
		}(w)
	}

	// listener churn: register, consume a few wakeups, then unregister
	for l := 0; l < workers/2; l++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				listener := s.Listen()
				if listener == nil {
					return
				}
				drained := 0
				for drained < 4 {
					select {
					case _, ok := <-listener:
						if !ok {
							return // closed by Cleanup
						}
						drained++
					case <-time.After(time.Millisecond):
						drained = 4
					}
				}
				s.UnListen(listener)
			}
		}()
	}

	// one long-lived ranging consumer (like HandleCrowd): must exit when the
	// SockMap is cleaned up (its listener channel is closed)
	rangerDone := make(chan struct{})
	go func() {
		defer close(rangerDone)
		listener := s.Listen()
		for range listener {
			// re-derive truth from the live map, exactly like real consumers
			_ = s.Length()
		}
	}()

	wg.Wait()

	// tear down while late Track/UnTrack/notify calls may still race in; none of
	// these may panic (updateChannel is never closed; listeners close under mut)
	var late sync.WaitGroup
	for w := 0; w < workers; w++ {
		late.Add(1)
		go func() {
			defer late.Done()
			for i := 0; i < 100; i++ {
				s.Track(newTestSocket("late", "lc"))
				s.UnTrack("late", "lc")
			}
		}()
	}

	s.Cleanup()
	// second cleanup must be a no-op, not a double-close panic
	s.Cleanup()

	late.Wait()

	// the ranging consumer must have observed the close and exited
	select {
	case <-rangerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("long-lived listener did not exit after Cleanup")
	}

	// Listen after cleanup must return an already-closed channel (no block)
	if l := s.Listen(); l != nil {
		select {
		case _, ok := <-l:
			if ok {
				t.Fatal("expected closed listener after Cleanup")
			}
		case <-time.After(time.Second):
			t.Fatal("Listen after Cleanup returned an open, empty channel")
		}
	}
}
