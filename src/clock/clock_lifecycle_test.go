package clock

import (
	"os"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/bus"
)

// TestMain brings up the event bus so the clock's publisher does not spin
// forever on `ready == false` (which would block handleCommand under the
// clock mutex). Production brings the bus up at startup via bus.Up().
func TestMain(m *testing.M) {
	bus.Up()
	os.Exit(m.Run())
}

// flip mimics room.Instance.flipClock: capture the ack channel for the current
// turn, send a Flip, and wait for the acknowledgement. After the ack, it waits
// for handleCommand to release the clock mutex so the turn flip is visible
// before a subsequent flip captures the next ack channel. (In production,
// flipClock is driven one move at a time by the room routine, so the goroutine
// always settles between flips.)
func flip(c *Clock) {
	ack := c.GetAck()
	c.ControlChannel <- Flip
	<-ack
	_ = c.State(true)
}

// withTimeout runs fn and fails the test if it does not finish in time,
// surfacing deadlocks instead of hanging the whole suite.
func withTimeout(t *testing.T, d time.Duration, msg string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal(msg)
	}
}

// TestClockFlipAckRoundtrip ensures a normal flip/ack handshake does not
// deadlock and can be repeated.
func TestClockFlipAckRoundtrip(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(time.Minute)})
	c.Start()

	withTimeout(t, time.Second, "flip handshake deadlocked", func() {
		flip(c) // first (free) move
		flip(c) // second move
	})

	c.Stop(false, true)
}

// TestClockStopIdempotent ensures Stop can be called repeatedly without
// panicking (e.g. tryGameOver stopping a clock that already flagged).
func TestClockStopIdempotent(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(time.Minute)})
	c.Start()

	withTimeout(t, time.Second, "first move deadlocked", func() { flip(c) })

	c.Stop(false, true)
	c.Stop(false, true) // must be a no-op, not a panic
	c.Stop(true, true)  // writeState branch must not panic either
}

// TestClockResetRestart ensures the clock can be stopped, reset, and started
// again without leaking the previous goroutine or breaking the handshake.
func TestClockResetRestart(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(time.Minute)})

	c.Start()
	withTimeout(t, time.Second, "first run deadlocked", func() { flip(c) })

	c.Reset()
	if !c.State(true).IsPaused {
		t.Fatal("clock should be paused after Reset")
	}
	if c.State(true).Victor != NoVictor {
		t.Fatal("victor should be cleared after Reset")
	}

	c.Start()
	withTimeout(t, time.Second, "flip after reset deadlocked", func() { flip(c) })

	c.Stop(false, true)
}

// TestClockFlagViaTimer ensures the flag timer path publishes a flagged state
// with a victor when a player runs out of time without moving.
func TestClockFlagViaTimer(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(10 * time.Millisecond)})
	c.Start()

	// white makes the first (free) move, handing the short clock to black
	withTimeout(t, time.Second, "first move deadlocked", func() { flip(c) })

	// black never moves and should flag; white wins
	select {
	case state := <-c.StateChannel:
		if state.Victor != White {
			t.Fatalf("expected White victor on black flag, got %d", state.Victor)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("clock did not flag via timer")
	}
}

// TestHandleCommandFlagAcksFlip is the regression test for the flag-on-own-move
// deadlock: when a flip is processed and the moving player has flagged,
// handleCommand must still acknowledge the flip (releasing a waiting flipClock)
// before stopping, otherwise the room routine and clock goroutine deadlock.
func TestHandleCommandFlagAcksFlip(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(10 * time.Millisecond)})

	// simulate a running clock on a non-first move where the side to move
	// (white) has used far more than their entire budget before flipping
	c.clockPaused = false
	c.firstMove = false
	c.timestamp = time.Now().Add(-time.Second)

	// a concurrent reader standing in for flipClock's <-ackChannel
	ack := c.GetAck()
	acked := make(chan FlipAck, 1)
	go func() {
		acked <- <-ack
	}()

	var flagged bool
	withTimeout(t, time.Second, "handleCommand deadlocked on flagged flip", func() {
		flagged = c.handleCommand(Flip)
	})

	if !flagged {
		t.Fatal("expected handleCommand to report a flag")
	}

	select {
	case fa := <-acked:
		// the flagged flip's ack reads truthfully: the whole budget was
		// charged (takeTime caps at it) and nothing remains
		if fa.Think.Milli() != 10 {
			t.Fatalf("flagged flip should charge the full budget, got %s", fa.Think)
		}
		if fa.Remaining.Milli() != 0 {
			t.Fatalf("flagged flip should leave zero remaining, got %s", fa.Remaining)
		}
	case <-time.After(time.Second):
		t.Fatal("flagged flip was not acknowledged (deadlock regression)")
	}

	// the flagged state must have been published to the buffered state channel
	select {
	case state := <-c.StateChannel:
		if state.Victor != Black {
			t.Fatalf("expected Black victor (white flagged), got %d", state.Victor)
		}
	default:
		t.Fatal("expected a buffered flagged state")
	}

	if c.State(true).Turn != octad.White {
		t.Fatal("turn should not advance when the moving player flags")
	}
}

// flipAck mirrors flip but returns the FlipAck payload for inspection.
func flipAck(c *Clock) FlipAck {
	ack := c.GetAck()
	c.ControlChannel <- Flip
	fa := <-ack
	_ = c.State(true)
	return fa
}

// TestFlipAckReportsMoveTiming verifies the per-move timing carried by the
// flip acknowledgement: the uncharged first move reads as zero think time and
// a full budget, and a later move reports its actual charge plus the
// post-increment remaining budget.
func TestFlipAckReportsMoveTiming(t *testing.T) {
	inc := 100 * time.Millisecond
	c := NewClock(TimeControl{Time: ToCTime(time.Minute), Increment: ToCTime(inc)})
	c.Start()

	withTimeout(t, time.Second, "flip handshake deadlocked", func() {
		// white's first move: never charged, no increment awarded
		fa := flipAck(c)
		if fa.Think.Milli() != 0 {
			t.Errorf("first move should charge nothing, got %s", fa.Think)
		}
		if fa.Remaining.Milli() != time.Minute.Milliseconds() {
			t.Errorf("first move should leave the full budget, got %s", fa.Remaining)
		}

		// black thinks for a measurable moment before moving
		think := 50 * time.Millisecond
		time.Sleep(think)
		fa = flipAck(c)
		if fa.Think.t < think/2 || fa.Think.t > think*4 {
			t.Errorf("think time %s not near the %s slept", fa.Think, think)
		}
		// remaining reflects the increment: budget - charge + increment
		want := time.Minute - fa.Think.t + inc
		if fa.Remaining.t != want {
			t.Errorf("remaining %s, want budget-think+inc = %s", fa.Remaining, ToCTime(want))
		}
	})

	c.Stop(false, true)
}
