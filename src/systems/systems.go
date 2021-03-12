package systems

import "github.com/dechristopher/lioctad/store"

// Initializers for subsystem components
var Initializers = []func(){
	store.Up,
}

// Run all of the subsystem initializer functions
func Run() {
	for _, i := range Initializers {
		go i()
	}
}
