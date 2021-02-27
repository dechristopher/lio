package clock

import (
	"log"
	"sync"
	"time"
)

// clock represents the clock for a single game
type Clock struct {
	black  string
	white  string
	victor Victor

	isBlack      bool
	delayExpired bool
	clockPaused  bool

	blackTime time.Duration
	whiteTime time.Duration

	timeControl TimeControl

	ControlChannel chan Command
	StateChannel   chan State

	ticker *time.Ticker // fires per-cs events for decrementing time
	timer  *time.Timer  // fires an event to represent delay time expiring
	mutex  sync.Mutex
}

// NewClock returns a clock configured for the given players at
// the specified time control
func NewClock(player1, player2 string, tc TimeControl) *Clock {
	return &Clock{
		black:          player1,
		white:          player2,
		victor:         NoVictor, // game in play
		isBlack:        false,
		delayExpired:   false,
		clockPaused:    true,
		blackTime:      tc.Time,
		whiteTime:      tc.Time,
		timeControl:    tc,
		ControlChannel: make(chan Command),
		StateChannel:   make(chan State),
		ticker:         nil,
		timer:          nil,
		mutex:          sync.Mutex{},
	}
}

// Start begins the clock and its internal routines for
// handling time decrement and player command input
func (c *Clock) Start() {
	c.clockPaused = false
	c.ticker = time.NewTicker(Centi)

	go func() {
		if c.timeControl.Delay != 0 {
			c.timer = time.NewTimer(c.timeControl.Delay)
		} else {
			// default to true for immediate decrement
			c.timer = time.NewTimer(time.Hour)
			c.delayExpired = true
		}
		for {
			select {
			case cmd := <-c.ControlChannel:
				log.Printf("Command: %d", cmd)
				handleCommand(c, cmd)
			case <-c.timer.C:
				c.delayExpired = true
				// clean up clock if it somehow persists through
				// the hour and ticks over.
				if c.timeControl.Delay == 0 {
					log.Printf("WARNING: cleanup not working")
					return
				}
			case <-c.ticker.C:
				if c.Flagged() {
					c.ticker.Stop()
					c.timer.Stop()
					c.StateChannel <- c.State()
					log.Printf("Game over, victor: %d", c.victor)
					return
				}
				if c.delayExpired {
					if c.isBlack {
						c.blackTime -= Centi
					} else {
						c.whiteTime -= Centi
					}
				}
			}
			// write state every tick
			c.StateChannel <- c.State()
		}
	}()
}

// Flagged returns true if someone wins on time
// and updates the victor in the clock state
func (c *Clock) Flagged() bool {
	if c.blackTime <= 0 {
		c.victor = White
	}
	if c.whiteTime <= 0 {
		c.victor = Black
	}
	return c.victor != NoVictor
}

// State returns the current clock state
func (c *Clock) State() State {
	return State{
		BlackTime: c.blackTime,
		WhiteTime: c.whiteTime,
		IsBlack:   c.isBlack,
		IsPaused:  c.clockPaused,
		Victor:    c.victor,
	}
}
