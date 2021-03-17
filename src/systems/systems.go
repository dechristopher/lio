package systems

import (
	"time"

	"github.com/dechristopher/lioctad/bus"
	"github.com/dechristopher/lioctad/store"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

var pub *bus.Publisher

// Up brings up the systems publisher and sends a test message
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
	Up,
}

// Run all of the subsystem initializer functions
func Run() {
	for _, i := range Initializers {
		go i()
	}
	err := bus.SystemChannel.SubscribeOnce(func(e bus.Event) {
		util.Debug(str.CBus, str.DBusOk)
	})
	if err != nil {
		panic(err)
	}
}
