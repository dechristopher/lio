package clock

import (
	"time"

	"github.com/dechristopher/lio/lag"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// handleCommand will perform the command on the given clock and
// return true if someone has flagged
func (c *Clock) handleCommand(cmd Command) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.flagTimer != nil {
		c.flagTimer.Stop()
	}

	util.DebugFlag("clock", str.CClk, "Command: %d", cmd)

	switch cmd {
	case Flip:
		// don't subtract time or increment on first move of game
		if c.firstMove {
			c.firstMove = false
		} else {
			// update elapsed time of current player
			c.players[c.turn].giveTime(ToCTime(time.Since(c.timestamp)))

			// compensate player for move processing lag
			c.players[c.turn].takeTime(ToCTime(lag.Move.Get()))

			// check to see if someone flagged
			if c.flagged() {
				c.Stop(true, false)
				return true
			}

			// add increment if enabled
			if c.hasIncrement() {
				c.players[c.turn].takeTime(c.timeControl.Increment)
			}
		}

		// reset delay if enabled
		if c.delayTimer != nil && c.timeControl.Delay.t != 0 {
			c.delayTimer.Reset(c.timeControl.Delay.t)
			c.delayExpired = false
		}

		// acknowledge clock flip
		c.ackChannels[c.turn] <- true

		// flip clock
		c.turn = c.turn.Other()

		// update last move timestamp
		c.timestamp = time.Now()

		// set flag timer to check for the player flagging
		// after their current time budget expires
		c.flagTimer.Reset(c.players[c.turn].remaining().t)

		// publish clock state to monitors
		c.publisher.Publish(cmd, c.State(false))

		return false
	default:
		return false
	}
}
