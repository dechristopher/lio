package game

import (
	"github.com/dechristopher/lio/bus"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// MonitorSub creates the game monitoring subscription
func MonitorSub() {
	err := Channel.Subscribe(monitorGame)
	if err != nil {
		panic(err)
	}
}

// monitorGame watches the bus channel and catalogues
// all game state and moves made for all games
func monitorGame(e bus.Event) {
	util.DebugFlag("game", str.CGme, str.DGameMove, e.Data...)
}
