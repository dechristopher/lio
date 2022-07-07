package clock

import (
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// handleCommand will perform the command on the given clock
func handleCommand(c *Clock, cmd Command) {
	util.DebugFlag("clock", str.CClk, "Command: %d", cmd)

	switch cmd {
	case Flip:
		// don't increment on first move of game
		if c.firstMove {
			c.firstMove = false
		} else {
			if c.timeControl.Increment.t != 0 {
				if c.isBlack {
					old := c.blackTime
					c.blackTime += c.timeControl.Increment.t
					util.DebugFlag("clock", str.CClk, "black clock incr %s -> %s", old, c.blackTime)
				} else {
					old := c.whiteTime
					c.whiteTime += c.timeControl.Increment.t
					util.DebugFlag("clock", str.CClk, "white clock incr %s -> %s", old, c.whiteTime)
				}
			}
		}

		// reset delay if enabled
		if c.timer != nil && c.timeControl.Delay.t != 0 {
			c.timer.Reset(c.timeControl.Delay.t)
			c.delayExpired = false
		}

		if c.isBlack {
			c.BlackAck <- true
		} else {
			c.WhiteAck <- true
		}

		// flip clock
		c.isBlack = !c.isBlack

		// publish clock state to monitors
		c.publisher.Publish(cmd, c.whiteTime, c.blackTime)
		return
	}
}
