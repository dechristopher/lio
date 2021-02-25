package clock

// handleCommand will perform the command on the given clock
func handleCommand(c *Clock, cmd Command) {
	switch cmd {
	case Flip:
		if c.timeControl.Increment != 0 {
			if c.isBlack {
				c.blackTime += c.timeControl.Increment
			} else {
				c.whiteTime += c.timeControl.Increment
			}
		}
		// flip turn
		c.isBlack = !c.isBlack
		// reset delay if enabled
		if c.timer != nil && c.timeControl.Delay != 0 {
			c.timer.Reset(c.timeControl.Delay)
			c.delayExpired = false
		}
		return
	}
}
