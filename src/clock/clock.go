package clock

import (
	"sync"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/bus"
)

// Channel is the engine monitoring bus channel
const Channel bus.Channel = "lio:clock"

// Clock represents the clock for a single game
type Clock struct {
	control TimeControl

	victor    Victor
	turn      octad.Color
	timestamp time.Time
	players   map[octad.Color]*playerClock

	firstMove    bool
	clockPaused  bool
	delayExpired bool

	ControlChannel chan Command
	StateChannel   chan State
	ackChannels    map[octad.Color]chan FlipAck

	// quit terminates the running clock goroutine. Start creates a fresh one
	// per run; Stop closes it. It is guarded by mutex.
	quit chan struct{}
	// stopped is closed by the clock goroutine when it exits. Reset waits on it
	// so a subsequent Start cannot race the winding-down goroutine over the
	// shared timer fields. Guarded by mutex.
	stopped chan struct{}

	flagTimer  *time.Timer // fires an event to check for a player flagging
	delayTimer *time.Timer // fires an event to represent delay time expiring
	// preStartTimer ends the bounded first-move grace (TimeControl.PreStart):
	// when it fires the side to move goes on the clock without having moved.
	// preStartDeadline mirrors it for State reporting. Guarded by mutex.
	preStartTimer    *time.Timer
	preStartDeadline time.Time
	mutex            *sync.Mutex // prevent concurrent clock state changes

	publisher *bus.Publisher
}

func (c *Clock) hasIncrement() bool {
	return c.control.Increment.t > 0
}

// time container for a single player
type playerClock struct {
	//lag *lag.Tracker
	control TimeControl
	elapsed CTime
}

// remaining time budget considering elapsed time
func (pc *playerClock) remaining() CTime {
	return pc.control.Time.Diff(pc.elapsed)
}

// takeTime adds the given time to the player's elapsed time
func (pc *playerClock) takeTime(t CTime) {
	pc.elapsed = pc.elapsed.Add(t)
	if pc.elapsed.t > pc.control.Time.t {
		pc.elapsed = pc.control.Time
	}
}

// giveTime subtracts the given time from the player's elapsed time
func (pc *playerClock) giveTime(t CTime) {
	pc.elapsed = pc.elapsed.Diff(t)
}

// flagged returns true if the player has exceeded their time budget
// exceeding time budget is quantified as time remaining > 0
func (pc *playerClock) flagged() bool {
	return pc.remaining().t <= flagged.t
}

// NewClock returns a clock configured for the given players at
// the specified time control
func NewClock(tc TimeControl) *Clock {
	clock := &Clock{
		control:        tc,
		victor:         NoVictor, // game in progress
		turn:           octad.White,
		players:        make(map[octad.Color]*playerClock),
		firstMove:      true,
		clockPaused:    true,
		delayExpired:   false,
		ControlChannel: make(chan Command),
		// buffered so Stop can publish the final (flagged) state without
		// blocking on a consumer that may itself be blocked waiting on the
		// flip acknowledgement — see Stop and handleCommand
		StateChannel: make(chan State, 1),
		ackChannels:  make(map[octad.Color]chan FlipAck),
		mutex:        &sync.Mutex{},
		publisher:    bus.NewPublisher("clock", Channel),
	}

	clock.players[octad.White] = &playerClock{control: tc, elapsed: ToCTime(0)}
	clock.players[octad.Black] = &playerClock{control: tc, elapsed: ToCTime(0)}

	clock.ackChannels[octad.White] = make(chan FlipAck)
	clock.ackChannels[octad.Black] = make(chan FlipAck)

	return clock
}

// flagged returns true if someone wins on time
// and updates the victor in the clock state
func (c *Clock) flagged() bool {
	if c.players[c.turn].flagged() {
		if c.turn == octad.Black {
			c.victor = White
		} else {
			c.victor = Black
		}
	}
	return c.victor != NoVictor
}

// EstimateRemaining time budget for the given color estimated based on elapsed time since last move
func (c *Clock) EstimateRemaining(color octad.Color) CTime {
	// estimate remaining time budget
	if c.turn == color {
		rem := c.players[color].remaining()

		// return exact time if still first move or the clock is not running
		// (pre-start, stopped on game end, or restored-paused after a restart):
		// time only drains against a running clock, so estimating off the last
		// flip timestamp would wrongly keep draining after a Stop and would
		// instantly zero a restored clock whose timestamp predates the restart
		if c.firstMove || c.clockPaused {
			return rem
		}

		// subtract time since last timestamp from remaining to get estimate
		additional := time.Since(c.timestamp)
		estimate := rem.Diff(ToCTime(additional))

		// if flagged, no need to return negative times
		if estimate.t <= flagged.t {
			return flagged
		}

		// return estimate
		return estimate
	}

	// return exact remaining time if not player's turn
	return c.players[color].remaining()
}

// EstimateFlagged uses EstimateRemaining to determine flag status out
// of band from player moves and state updates
func (c *Clock) EstimateFlagged() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// player's estimated remaining time budget has fallen to zero
	return c.EstimateRemaining(c.turn).t <= flagged.t
}

// GetAck returns the ack channel for the current player
func (c *Clock) GetAck() chan FlipAck {
	return c.ackChannels[c.turn]
}

// Start begins the clock and its internal routines for
// handling time decrement and player command input
func (c *Clock) Start() {
	c.mutex.Lock()
	// "start" clock
	c.clockPaused = false

	// fresh quit/stopped channels so this run has its own lifecycle; Stop
	// closes quit to terminate the goroutine, which closes stopped on exit so
	// Reset can wait for it before a later Start reuses the timer fields
	c.quit = make(chan struct{})
	c.stopped = make(chan struct{})
	quit := c.quit
	stopped := c.stopped

	// set flag timer so we can reset it later on
	c.flagTimer = time.NewTimer(time.Nanosecond)

	// set up the delay timer here, under the mutex, not in the goroutine below:
	// Stop reads delayTimer under the mutex, and an immediate Start→Stop (e.g. a
	// restored room resumed and finished at once) would race the goroutine's
	// unlocked write of the field
	if c.control.Delay.t != 0 {
		c.delayTimer = time.NewTimer(c.control.Delay.t)
	} else {
		// default to true for immediate decrement
		c.delayTimer = time.NewTimer(time.Hour)
		c.delayExpired = true
	}

	// arm the pre-start countdown when the time control bounds the first-move
	// grace; a flip inside the window disarms it (handleCommand), expiry ends
	// the grace (expirePreStart). Without one, the stopped timer's channel
	// simply never fires in the select below.
	if c.control.PreStart.t > 0 && c.firstMove {
		c.preStartDeadline = time.Now().Add(c.control.PreStart.t)
		c.preStartTimer = time.NewTimer(c.control.PreStart.t)
	} else {
		c.preStartDeadline = time.Time{}
		c.preStartTimer = time.NewTimer(time.Hour)
		c.preStartTimer.Stop()
	}
	c.mutex.Unlock()

	go func(cl *Clock, quit, stopped chan struct{}) {
		defer close(stopped)
		for {
			select {
			case <-quit:
				// clock stopped; terminate this goroutine
				return
			case cmd := <-cl.ControlChannel:
				// process clock commands
				if cl.handleCommand(cmd) {
					return
				}
			case <-cl.delayTimer.C:
				// set delay expired after delay timer has ended
				cl.delayExpired = true
			case <-cl.preStartTimer.C:
				// pre-start countdown expired; put the side to move on the clock
				cl.expirePreStart()
			case <-cl.flagTimer.C:
				// check to see if any player has flagged
				if cl.EstimateFlagged() {
					cl.mutex.Lock()
					// charge the flagged player's un-flipped think time, as a
					// flip would have, so the final state reads truthfully:
					// takeTime caps at the full budget, leaving remaining 0
					// rather than the stale as-of-last-flip value (a player
					// flagging without ever moving would otherwise show a
					// full clock under an "out of time" result)
					cl.players[cl.turn].takeTime(ToCTime(time.Since(cl.timestamp)))
					if cl.turn == octad.Black {
						cl.victor = White
					} else {
						cl.victor = Black
					}
					cl.mutex.Unlock()

					cl.Stop(true, true)
					return
				}
			}
		}
	}(c, quit, stopped)
}

// expirePreStart ends the bounded first-move grace when the pre-start
// countdown lapses: the side to move goes on the clock as if the game had
// just commenced — time drains from now and the flag timer is armed for
// their full budget, so a player who never moves flags through the normal
// path. A no-op if a flip already started the game or the clock stopped
// (the timer fire races both harmlessly).
func (c *Clock) expirePreStart() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.firstMove || c.clockPaused {
		return
	}

	c.firstMove = false
	c.preStartDeadline = time.Time{}
	c.timestamp = time.Now()
	c.flagTimer.Reset(c.players[c.turn].remaining().t)

	// publish clock state to monitors
	c.publisher.Publish(Flip, c.State(false))
}

// preStartRemaining reports the time left in a running pre-start countdown,
// zero once the game has commenced or when none is configured. The caller
// must hold mutex.
func (c *Clock) preStartRemaining() CTime {
	if !c.firstMove || c.clockPaused || c.preStartDeadline.IsZero() {
		return ToCTime(0)
	}
	rem := time.Until(c.preStartDeadline)
	if rem < 0 {
		rem = 0
	}
	return ToCTime(rem)
}

// Reset the clock times and prepare for another game. Stop terminates the
// running goroutine (via quit), so the command/ack channels are safe to reuse;
// only the state channel is recreated to discard any buffered flagged state.
func (c *Clock) Reset() {
	c.mutex.Lock()
	running := !c.clockPaused
	stopped := c.stopped
	c.mutex.Unlock()

	c.Stop(false, true)

	// wait for the running goroutine to fully exit before mutating the shared
	// timer fields below, so a subsequent Start cannot race it. Reset is only
	// driven by the room routine on an invalid first move, when no flip is in
	// flight, so the goroutine is parked in its select and exits promptly.
	if running && stopped != nil {
		<-stopped
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.players[octad.White].elapsed = ToCTime(0)
	c.players[octad.Black].elapsed = ToCTime(0)

	// restore fresh-game state
	c.firstMove = true
	c.victor = NoVictor
	c.turn = octad.White

	// drop any buffered (flagged) state from the previous game
	c.StateChannel = make(chan State, 1)
}

// Stop the clock, terminate its goroutine, and optionally publish the final
// state. Safe to call multiple times (the clockPaused guard makes repeat calls
// no-ops). The command/ack/state channels are intentionally left open so a
// late flipClock send can never panic on a closed channel; the goroutine is
// terminated via the quit channel instead.
func (c *Clock) Stop(writeState, lock bool) {
	if lock {
		c.mutex.Lock()
		defer c.mutex.Unlock()
	}

	// already stopped: avoid a double quit-close and duplicate state publish
	if c.clockPaused {
		return
	}

	c.clockPaused = true

	if c.flagTimer != nil {
		c.flagTimer.Stop()
	}
	if c.delayTimer != nil {
		c.delayTimer.Stop()
	}
	if c.preStartTimer != nil {
		c.preStartTimer.Stop()
	}
	c.preStartDeadline = time.Time{}

	if writeState {
		// StateChannel is buffered(1), so this never blocks
		c.StateChannel <- c.State(false)
	}

	// signal the clock goroutine to exit
	if c.quit != nil {
		close(c.quit)
		c.quit = nil
	}
}

// Snapshot is the serializable state of a clock as-of-last-flip: per-player
// elapsed time, whose turn it is, and the first-move/victor flags. It
// deliberately excludes the live flip timestamp — a restored clock never
// charges wall time that passed while the process was down (restart
// persistence policy: players are not charged for a deploy).
type Snapshot struct {
	WhiteElapsedMs int64       `json:"we"`
	BlackElapsedMs int64       `json:"be"`
	Turn           octad.Color `json:"t"`
	FirstMove      bool        `json:"fm,omitempty"`
	Victor         Victor      `json:"v,omitempty"`
}

// Snapshot captures the clock's persistable state. Safe to call on a running
// clock: elapsed time is only ever advanced at a flip, so a mid-think capture
// reads as-of-last-flip — exactly the restore semantics we want.
func (c *Clock) Snapshot() Snapshot {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return Snapshot{
		WhiteElapsedMs: c.players[octad.White].elapsed.Milli(),
		BlackElapsedMs: c.players[octad.Black].elapsed.Milli(),
		Turn:           c.turn,
		FirstMove:      c.firstMove,
		Victor:         c.victor,
	}
}

// Restore builds a paused clock from a persisted snapshot. The clock does not
// run until Resume (mid-game restore) or Start (fresh-game paths) is called;
// until then State/EstimateRemaining report the exact restored remaining time.
func Restore(tc TimeControl, s Snapshot) *Clock {
	c := NewClock(tc)
	c.players[octad.White].elapsed = ToCTime(time.Duration(s.WhiteElapsedMs) * Millisecond)
	c.players[octad.Black].elapsed = ToCTime(time.Duration(s.BlackElapsedMs) * Millisecond)
	c.turn = s.Turn
	c.firstMove = s.FirstMove
	c.victor = s.Victor
	// defensive: never leave the zero timestamp in place — anything estimating
	// off it would charge decades of "elapsed" time
	c.timestamp = time.Now()
	return c
}

// Resume unpauses a restored clock: it re-bases the flip timestamp to now (so
// the side to move is charged from the resume instant, not from the pre-restart
// flip) and starts the clock goroutine. The flag timer is then armed for the
// side to move's remaining budget — Start's primer only checks once, and the
// per-flip re-arm in handleCommand hasn't run for a restored mid-game clock.
func (c *Clock) Resume() {
	c.mutex.Lock()
	c.timestamp = time.Now()
	c.mutex.Unlock()

	c.Start()

	c.mutex.Lock()
	if !c.firstMove && c.flagTimer != nil {
		// a pending fire from Start's 1ns primer may still be delivered; that
		// stray delivery is just an extra not-flagged check and is harmless
		c.flagTimer.Reset(c.players[c.turn].remaining().t)
	}
	c.mutex.Unlock()
}

// State returns the current clock state
func (c *Clock) State(lock bool) State {
	if lock {
		c.mutex.Lock()
		defer c.mutex.Unlock()
	}
	return State{
		WhiteTime: c.EstimateRemaining(octad.White),
		BlackTime: c.EstimateRemaining(octad.Black),
		Turn:      c.turn,
		IsPaused:  c.clockPaused,
		Victor:    c.victor,
		PreStart:  c.preStartRemaining(),
	}
}
