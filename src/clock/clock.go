package clock

import (
	"log"
	"sync"
	"time"

	"github.com/dechristopher/lioctad/bus"
)

// Channel is the engine monitoring bus channel
const Channel bus.Channel = "lio:clock"

// Clock represents the clock for a single game
type Clock struct {
	black  string
	white  string
	victor Victor

	isBlack      bool
	delayExpired bool
	clockPaused  bool
	firstMove    bool

	blackTime time.Duration
	whiteTime time.Duration

	timeControl TimeControl

	ControlChannel chan Command
	StateChannel   chan State
	WhiteAck       chan bool
	BlackAck       chan bool

	ticker *time.Ticker // fires per-cs events for decrementing time
	timer  *time.Timer  // fires an event to represent delay time expiring
	mutex  *sync.Mutex

	publisher *bus.Publisher
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
		firstMove:      true,
		blackTime:      tc.Time.t,
		whiteTime:      tc.Time.t,
		timeControl:    tc,
		ControlChannel: make(chan Command),
		StateChannel:   make(chan State),
		WhiteAck:       make(chan bool),
		BlackAck:       make(chan bool),
		ticker:         nil,
		timer:          nil,
		mutex:          &sync.Mutex{},
		publisher:      bus.NewPublisher("clock", Channel),
	}
}

// Start begins the clock and its internal routines for
// handling time decrement and player command input
func (c *Clock) Start() {
	c.clockPaused = false
	c.ticker = time.NewTicker(Centi)

	go func(cl *Clock) {
		c.publisher.Publish("start", c.whiteTime, c.blackTime, c.State())
		if cl.timeControl.Delay.t != 0 {
			cl.timer = time.NewTimer(cl.timeControl.Delay.t)
		} else {
			// default to true for immediate decrement
			cl.timer = time.NewTimer(time.Hour)
			cl.delayExpired = true
		}
		for {
			select {
			case cmd := <-cl.ControlChannel:
				c.mutex.Lock()
				handleCommand(cl, cmd)
				c.mutex.Unlock()
			case <-cl.timer.C:
				cl.delayExpired = true
				// clean up clock if it somehow persists through
				// the hour and ticks over.
				if cl.timeControl.Delay.t == 0 {
					log.Printf("WARNING: cleanup not working")
					return
				}
			case <-cl.ticker.C:
				if cl.Flagged() {
					cl.Stop()
					return
				}
				if cl.delayExpired {
					if cl.isBlack {
						cl.blackTime -= Centi
					} else {
						cl.whiteTime -= Centi
					}
				}
			}
		}
	}(c)
}

// Stop the clock and write state to state channel
func (c *Clock) Stop() {
	c.clockPaused = true

	if c.ticker != nil {
		c.ticker.Stop()
	}
	if c.timer != nil {
		c.timer.Stop()
	}

	c.StateChannel <- c.State()
	close(c.StateChannel)
	close(c.ControlChannel)
	close(c.WhiteAck)
	close(c.BlackAck)
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
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return State{
		BlackTime: CTime{c.blackTime},
		WhiteTime: CTime{c.whiteTime},
		IsBlack:   c.isBlack,
		IsPaused:  c.clockPaused,
		Victor:    c.victor,
	}
}
