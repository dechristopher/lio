package systems

import (
	"time"

	"github.com/dechristopher/lio/bus"
	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/dispatch"
	"github.com/dechristopher/lio/engine"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/store"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

var pub *bus.Publisher

// Up brings the system publisher online and sends a test message
// verifying that the bus has come online after some time
func Up() {
	pub = bus.NewPublisher("sys", bus.SystemChannel)
	time.Sleep(time.Millisecond * 500)
	pub.Publish("bus online")
}

// Initializers for subsystem components
var Initializers = []func(){
	bus.Up,
	store.Up,
	dispatch.UpEngine,
	engine.MonitorSub,
	clock.MonitorSub,
	game.MonitorSub,
	Up,
}

// Run all the subsystem initializer functions
func Run() {
	for i := range Initializers {
		Initializers[i]()
	}
	err := bus.SystemChannel.SubscribeOnce(func(e bus.Event) {
		util.Debug(str.CBus, str.DBusOk)
	})
	if err != nil {
		panic(err)
	}
}
