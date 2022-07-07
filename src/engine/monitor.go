package engine

import (
	"github.com/dechristopher/lioctad/bus"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// init the engine monitoring
func init() {
	go monitorSub()
}

// monitorSub creates the engine monitoring subscription
func monitorSub() {
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
