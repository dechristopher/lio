package clock

import (
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
)

// TestPreStartStateRemaining ensures a freshly started clock with a pre-start
// countdown reports the remaining grace via State, and that a zero PreStart
// reports none.
func TestPreStartStateRemaining(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(time.Minute), PreStart: ToCTime(time.Second)})
	c.Start()
	defer c.Stop(false, true)

	rem := c.State(true).PreStart
	if rem.t <= 0 || rem.t > time.Second {
		t.Fatalf("expected pre-start remaining in (0, 1s], got %s", rem)
	}

	plain := NewClock(TimeControl{Time: ToCTime(time.Minute)})
	plain.Start()
	defer plain.Stop(false, true)
	if plain.State(true).PreStart.t != 0 {
		t.Fatal("expected zero pre-start remaining without PreStart configured")
	}
}

// TestPreStartMoveWithinGrace ensures a first move inside the pre-start window
// behaves exactly like the unbounded grace: no time charged, countdown
// disarmed, and no later expiry disturbing the running game.
func TestPreStartMoveWithinGrace(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(time.Minute), PreStart: ToCTime(50 * time.Millisecond)})
	c.Start()
	defer c.Stop(false, true)

	withTimeout(t, time.Second, "first move deadlocked", func() { flip(c) })

	state := c.State(true)
	if state.PreStart.t != 0 {
		t.Fatal("pre-start countdown should be disarmed after the first move")
	}
	if state.Turn != octad.Black {
		t.Fatal("turn should have flipped to black")
	}
	if state.WhiteTime.t != time.Minute {
		t.Fatalf("white's first move must be free, got %s remaining", state.WhiteTime)
	}

	// wait past the original deadline: the disarmed timer must not fire and
	// mutate the running game (e.g. re-basing black's think timestamp)
	time.Sleep(80 * time.Millisecond)
	select {
	case s := <-c.StateChannel:
		t.Fatalf("unexpected clock state after disarmed pre-start: %+v", s)
	default:
	}
}

// TestPreStartExpiryPutsWhiteOnClock ensures an expired countdown ends the
// first-move grace: white's time drains without a move, and their eventual
// first move is charged like any other.
func TestPreStartExpiryPutsWhiteOnClock(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(time.Minute), PreStart: ToCTime(10 * time.Millisecond)})
	c.Start()
	defer c.Stop(false, true)

	// let the countdown lapse and some clock drain
	time.Sleep(60 * time.Millisecond)

	state := c.State(true)
	if state.PreStart.t != 0 {
		t.Fatal("pre-start remaining should be zero after expiry")
	}
	if state.WhiteTime.t >= time.Minute {
		t.Fatalf("white should be draining after expiry, got %s remaining", state.WhiteTime)
	}

	// white finally moves: the flip must charge the elapsed think time
	withTimeout(t, time.Second, "post-expiry flip deadlocked", func() { flip(c) })

	c.mutex.Lock()
	elapsed := c.players[octad.White].elapsed
	turn := c.turn
	c.mutex.Unlock()
	if elapsed.t <= 0 {
		t.Fatal("white's post-expiry move should have been charged time")
	}
	if turn != octad.Black {
		t.Fatal("turn should have flipped to black")
	}
}

// TestPreStartExpiryFlags ensures a player who never moves flags through the
// normal flag-timer path once the pre-start countdown has put them on the
// clock — the whole point of the bounded grace.
func TestPreStartExpiryFlags(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(20 * time.Millisecond), PreStart: ToCTime(10 * time.Millisecond)})
	c.Start()

	select {
	case state := <-c.StateChannel:
		if state.Victor != Black {
			t.Fatalf("expected Black victor on white flag, got %d", state.Victor)
		}
		// the timer-path flag must charge the un-flipped think time so the
		// final state shows the flagged player at zero, not a full clock
		if state.WhiteTime.Centi() != 0 {
			t.Fatalf("flagged white should read 0 remaining, got %s", state.WhiteTime)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("white never flagged after pre-start expiry")
	}
}

// TestPreStartResetRearms ensures Reset restores the bounded grace for the
// next game (a rematch), with a fresh countdown armed by the next Start.
func TestPreStartResetRearms(t *testing.T) {
	c := NewClock(TimeControl{Time: ToCTime(time.Minute), PreStart: ToCTime(time.Second)})
	c.Start()
	withTimeout(t, time.Second, "first run deadlocked", func() { flip(c) })

	c.Reset()
	if c.State(true).PreStart.t != 0 {
		t.Fatal("a paused (reset) clock should report no pre-start remaining")
	}

	c.Start()
	defer c.Stop(false, true)
	if c.State(true).PreStart.t <= 0 {
		t.Fatal("pre-start countdown should be re-armed after Reset + Start")
	}
}
