package clock

import (
	"sync"
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lioctad/bus"
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
		StateChannel:   make(chan State),
		ackChannels:    make(map[octad.Color]chan bool),
		mutex:          &sync.Mutex{},
		publisher:      bus.NewPublisher("clock", Channel),
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
	// "start" clock
	c.clockPaused = false

	// set flag timer so we can reset it later on
	c.flagTimer = time.NewTimer(time.Nanosecond)

	go func(cl *Clock) {
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
					cl.Stop(true)
					return
				}
			}
		}
	}(c)
}

// Reset the clock times and prepare for another game
func (c *Clock) Reset() {
	c.Stop(false)
	c.players[octad.White].elapsed = ToCTime(0)
	c.players[octad.Black].elapsed = ToCTime(0)

	c.ControlChannel = make(chan Command)
	c.StateChannel = make(chan State)
	c.ackChannels[octad.White] = make(chan bool)
	c.ackChannels[octad.Black] = make(chan bool)
}

// Stop the clock and write state to state channel
func (c *Clock) Stop(writeState bool) {
	c.clockPaused = true

	if c.flagTimer != nil {
		c.flagTimer.Stop()
	}
	if c.delayTimer != nil {
		c.delayTimer.Stop()
	}

	if writeState {
		c.StateChannel <- c.State()
	}
	close(c.StateChannel)
	close(c.ControlChannel)
	close(c.ackChannels[octad.White])
	close(c.ackChannels[octad.Black])
}

// State returns the current clock state
func (c *Clock) State() State {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return State{
		WhiteTime: c.EstimateRemaining(octad.White),
		BlackTime: c.EstimateRemaining(octad.Black),
		Turn:      c.turn,
		IsPaused:  c.clockPaused,
		Victor:    c.victor,
	}
}
