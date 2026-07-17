package room

import (
	"testing"
	"time"

	"github.com/valyala/fastjson"

	"github.com/dechristopher/lio/clock"
)

// runDeployToCompletion drives a full deploy phase (both humans submitting) on
// an already-driven room and waits for the game to begin.
func runDeployToCompletion(t *testing.T, r *Instance) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		r.handleDeploy()
		close(done)
	}()

	submitDeploy(r, "white", "knpp")
	submitDeploy(r, "black", "nkpp")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deploy did not complete in time")
	}
	if r.State() != StateGameOngoing {
		t.Fatalf("expected StateGameOngoing after deploy, got %s", r.State())
	}
}

// TestDeployPreStartCountdownWiring verifies the pre-start countdown flows
// from the variant time control through the deploy rebuild into the running
// clock and onto the wire: after the reveal, the deployed game's clock reports
// a running countdown and board snapshots carry ps/pst for the client overlay.
func TestDeployPreStartCountdownWiring(t *testing.T) {
	r := newTestInstance(t, "white", "black")
	// deployAndStart rebuilds the game from params.GameConfig, so the pre-start
	// configured here reaches the deployed game's fresh clock
	r.params.GameConfig.Variant.Control.PreStart = clock.ToCTime(time.Second)
	driveToDeploy(t, r)

	runDeployToCompletion(t, r)
	defer r.game.Clock.Stop(false, true)

	state := r.game.Clock.State(true)
	if state.IsPaused {
		t.Fatal("clock should be running after the deploy reveal")
	}
	if state.PreStart.Centi() <= 0 {
		t.Fatal("expected a running pre-start countdown after the reveal")
	}

	raw := r.CurrentGameStateMessage(false, false)
	if ps := fastjson.GetInt(raw, "d", "c", "ps"); ps <= 0 || ps > 100 {
		t.Fatalf("board snapshot ps = %d, want (0, 100] centi-seconds", ps)
	}
	if pst := fastjson.GetInt(raw, "d", "c", "pst"); pst != 100 {
		t.Fatalf("board snapshot pst = %d, want 100 centi-seconds", pst)
	}
}

// TestDeployPreStartExpiryDrainsWhite verifies the countdown expiring ends
// white's first-move grace on the live deployed game: their clock drains
// without a move ever being played, and the wire payload stops carrying the
// countdown fields.
func TestDeployPreStartExpiryDrainsWhite(t *testing.T) {
	r := newTestInstance(t, "white", "black")
	r.params.GameConfig.Variant.Control.PreStart = clock.ToCTime(20 * time.Millisecond)
	driveToDeploy(t, r)

	runDeployToCompletion(t, r)
	defer r.game.Clock.Stop(false, true)

	// let the countdown lapse and some clock drain
	time.Sleep(100 * time.Millisecond)

	state := r.game.Clock.State(true)
	if state.PreStart.Centi() != 0 {
		t.Fatal("pre-start remaining should be zero after expiry")
	}
	full := r.game.Variant.Control.Time.Centi()
	if state.WhiteTime.Centi() >= full {
		t.Fatalf("white should be draining after expiry, got %d of %d centi-seconds",
			state.WhiteTime.Centi(), full)
	}

	raw := r.CurrentGameStateMessage(false, false)
	if fastjson.Exists(raw, "d", "c", "ps") {
		t.Fatal("board snapshot should omit ps once the countdown has expired")
	}
}
