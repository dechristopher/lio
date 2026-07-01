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
	ackChannels    map[octad.Color]chan bool

	// quit terminates the running clock goroutine. Start creates a fresh one
	// per run; Stop closes it. It is guarded by mutex.
	quit chan struct{}
	// stopped is closed by the clock goroutine when it exits. Reset waits on it
	// so a subsequent Start cannot race the winding-down goroutine over the
	// shared timer fields. Guarded by mutex.
	stopped chan struct{}

	flagTimer  *time.Timer // fires an event to check for a player flagging
	delayTimer *time.Timer // fires an event to represent delay time expiring
	mutex      *sync.Mutex // prevent concurrent clock state changes

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
		ackChannels:  make(map[octad.Color]chan bool),
		mutex:        &sync.Mutex{},
		publisher:    bus.NewPublisher("clock", Channel),
	}

	clock.players[octad.White] = &playerClock{control: tc, elapsed: ToCTime(0)}
	clock.players[octad.Black] = &playerClock{control: tc, elapsed: ToCTime(0)}

	clock.ackChannels[octad.White] = make(chan bool)
	clock.ackChannels[octad.Black] = make(chan bool)

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

		// return full time if still first move
		if c.firstMove {
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
func (c *Clock) GetAck() chan bool {
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
	c.mutex.Unlock()

	go func(cl *Clock, quit, stopped chan struct{}) {
		defer close(stopped)
		// set up delay timer
		if cl.control.Delay.t != 0 {
			cl.delayTimer = time.NewTimer(cl.control.Delay.t)
		} else {
			// default to true for immediate decrement
			cl.delayTimer = time.NewTimer(time.Hour)
			cl.delayExpired = true
		}
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
			case <-cl.flagTimer.C:
				// check to see if any player has flagged
				if cl.EstimateFlagged() {
					cl.mutex.Lock()
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
	}
}
