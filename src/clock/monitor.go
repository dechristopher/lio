package clock

import (
	"github.com/dechristopher/lioctad/bus"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// MonitorSub creates the clock monitoring subscription
func MonitorSub() {
	err := Channel.Subscribe(monitorClock)
	if err != nil {
		panic(err)
	}
}

// monitorGame watches the bus channel and catalogues
// all game state and moves made for all games
func monitorClock(e bus.Event) {
	util.DebugFlag("clock", str.CClk, str.DClockEvent, e.Data...)
}
