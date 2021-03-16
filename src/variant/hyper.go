package variant

import "github.com/dechristopher/lioctad/clock"

// FiveZeroHyper is the five second, zero second increment hyper variant
var FiveZeroHyper = Variant{
	Name:  ":05 + 0",
	Group: HyperGroup,
	Time:  FiveZeroHyperTC,
}

// FiveZeroHyperTC is the five second, zero second increment hyper time control
var FiveZeroHyperTC = clock.TimeControl{
	Time:      5,
	Increment: 0,
}
