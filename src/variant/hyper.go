package variant

import (
	"time"

	"github.com/dechristopher/lioctad/clock"
)

// FiveZeroHyper is the five second, zero second increment hyper variant
var FiveZeroHyper = Variant{
	Name:     ":05 + 0",
	HTMLName: "five-zero-hyper",
	Group:    HyperGroup,
	Control:  FiveZeroHyperTC,
}

// FiveZeroHyperTC is the five second, zero second increment hyper time control
var FiveZeroHyperTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 5),
	Increment: clock.ToCTime(time.Second * 0),
}
