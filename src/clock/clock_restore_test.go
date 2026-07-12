package clock

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
)

// TestSnapshotRestoreRoundTrip locks the restore semantics: a restored clock
// carries the exact persisted elapsed times, turn, and first-move flag, starts
// paused, and — because it is paused — does not drain while it waits for the
// players to return.
func TestSnapshotRestoreRoundTrip(t *testing.T) {
	tc := TimeControl{Time: ToCTime(60 * time.Second), Increment: ToCTime(time.Second)}

	c := NewClock(tc)
	c.players[octad.White].elapsed = ToCTime(10 * time.Second)
	c.players[octad.Black].elapsed = ToCTime(20 * time.Second)
	c.turn = octad.Black
	c.firstMove = false

	snap := c.Snapshot()
	r := Restore(tc, snap)

	if got := r.Snapshot(); got != snap {
		t.Fatalf("restored snapshot = %+v, want %+v", got, snap)
	}

	state := r.State(true)
	if !state.IsPaused {
		t.Fatal("restored clock must start paused")
	}
	if state.WhiteTime.Milli() != 50000 || state.BlackTime.Milli() != 40000 {
		t.Fatalf("restored remaining = %s / %s, want 50s / 40s",
			state.WhiteTime, state.BlackTime)
	}
	if state.Turn != octad.Black {
		t.Fatalf("restored turn = %v, want black", state.Turn)
	}

	// a paused clock must not drain: the side to move's remaining time is
	// reported exactly, not estimated off the (stale) flip timestamp
	time.Sleep(20 * time.Millisecond)
	if got := r.State(true).BlackTime.Milli(); got != 40000 {
		t.Fatalf("paused restored clock drained: black remaining %dms, want 40000ms", got)
	}
}

// TestRestoreResumeFlagsExpiredMover verifies Resume arms the flag timer for
// the side to move: a restored clock with a nearly-exhausted mover flags them
// on its own (no move required), publishing the flagged state exactly like a
// live clock does.
func TestRestoreResumeFlagsExpiredMover(t *testing.T) {
	tc := TimeControl{Time: ToCTime(200 * time.Millisecond)}

	c := NewClock(tc)
	c.players[octad.Black].elapsed = ToCTime(100 * time.Millisecond)
	c.turn = octad.Black
	c.firstMove = false

	r := Restore(tc, c.Snapshot())
	r.Resume()

	select {
	case s := <-r.StateChannel:
		if s.Victor != White {
			t.Fatalf("flag victor = %v, want white (black exhausted)", s.Victor)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restored clock never flagged the expired side to move")
	}
}

// TestResumeChargesFromResumeInstant verifies the restart-persistence policy:
// wall time between the snapshot and the resume (the deploy downtime plus the
// reconnect grace) is never charged to the side to move.
func TestResumeChargesFromResumeInstant(t *testing.T) {
	tc := TimeControl{Time: ToCTime(time.Minute)}

	c := NewClock(tc)
	c.turn = octad.White
	c.firstMove = false

	r := Restore(tc, c.Snapshot())
	// simulated downtime between restore and resume
	time.Sleep(50 * time.Millisecond)
	r.Resume()
	defer r.Stop(false, true)

	charged := time.Minute - time.Duration(r.State(true).WhiteTime.Milli())*time.Millisecond
	if charged > 25*time.Millisecond {
		t.Fatalf("resume charged %s of downtime to the mover", charged)
	}
}

// TestTimeControlJSONRoundTrip covers CTime.UnmarshalJSON: variant time
// controls embedded in room snapshots must survive a JSON round trip,
// fractional seconds included (the ¼+0 control).
func TestTimeControlJSONRoundTrip(t *testing.T) {
	tc := TimeControl{
		Time:      ToCTime(15 * time.Second),
		Increment: ToCTime(time.Second),
		Delay:     ToCTime(250 * time.Millisecond),
	}

	b, err := json.Marshal(tc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got TimeControl
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != tc {
		t.Fatalf("round trip = %+v, want %+v", got, tc)
	}
}
