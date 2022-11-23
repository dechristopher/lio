package variant

import (
	"time"

	"github.com/dechristopher/lio/clock"
)

// ThreeZeroHyper is the three second, zero second increment hyper variant
var ThreeZeroHyper = Variant{
	Name:     ":03",
	HTMLName: "three-zero-hyper",
	Group:    HyperGroup,
	Control:  ThreeZeroHyperTC,
}

// ThreeZeroHyperTC is the three second, zero second increment hyper time control
var ThreeZeroHyperTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 3),
	Increment: clock.ToCTime(time.Second * 0),
}
