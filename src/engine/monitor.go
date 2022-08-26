package engine

import (
	"github.com/dechristopher/lio/bus"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// MonitorSub creates the engine monitoring subscription
func MonitorSub() {
	err := Channel.Subscribe(monitorEngine)
	if err != nil {
		panic(err)
	}
}

// monitor engine watches the bus channel and catalogues
// all engine search output events into a datastore
func monitorEngine(e bus.Event) {
	if len(e.Data) == 2 {
		util.DebugFlag("engine", str.CEng, str.DEngStart, e.Data...)
		return
	}
	util.DebugFlag("engine", str.CEng, str.DEngSearch, e.Data...)
}
