package room

import (
	"sync/atomic"

	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// draining is the process-wide shutdown gate (arch/STATE_PERSISTENCE_SCALING.md,
// drain step 1). Once set, every inbound mutation — moves, deploys, controls,
// joins, cancels, room creation — is dropped at its entry point, so nothing can
// change a room after its final snapshot is captured.
var draining atomic.Bool

// Draining reports whether shutdown drain has begun. Checked by every
// mutation entry point; a gated call is dropped exactly like the wrong-state
// drops those paths already perform.
func Draining() bool {
	return draining.Load()
}

// Drain quiesces the process for shutdown: gate all inbound mutations, freeze
// every room's clock (Stop without publishing a flag state — the paused,
// as-of-last-flip clock is precisely what the restore path expects), then
// synchronously flush every room's final snapshot. The caller (the www signal
// handler) then closes all sockets with 1012 Service Restart and shuts the
// listener down. Total wall time is a couple of Redis round trips.
func Drain() {
	draining.Store(true)

	rooms.Range(func(_, v interface{}) bool {
		v.(*Instance).quiesceClock()
		return true
	})

	FlushSnapshots()

	util.Info(str.CRoom, "drained %d room(s): mutations gated, clocks frozen, snapshots flushed", Count())
}

// quiesceClock freezes the room's clock for drain. Stop is idempotent (a
// finished or never-started clock is a no-op) and terminates the clock
// goroutine; the room routine stays parked in its select, which is fine — the
// process is exiting. An in-flight makeMove that raced past the gate completes
// harmlessly: flipClock skips flipping a paused clock.
func (r *Instance) quiesceClock() {
	r.stateMu.Lock()
	clk := r.game.Clock
	r.stateMu.Unlock()
	clk.Stop(false, true)
}
