package variant

import (
	"time"

	"github.com/dechristopher/lio/clock"
)

// HalfZeroRapid is the half minute, one second increment rapid variant
var HalfZeroRapid = Variant{
	Name:     "½ + 1",
	HTMLName: "half-one-rapid",
	Group:    RapidGroup,
	Control:  HalfZeroRapidTC,
}

// HalfZeroRapidTC is the half minute, zero second increment rapid time control
var HalfZeroRapidTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 30),
	Increment: clock.ToCTime(time.Second * 1),
}

// HalfTwoRapid is the half minute, two second increment rapid variant
var HalfTwoRapid = Variant{
	Name:     "½ + 2",
	HTMLName: "half-two-rapid",
	Group:    RapidGroup,
	Control:  HalfTwoRapidTC,
}

// HalfTwoRapidTC is the one minute, two second increment rapid time control
var HalfTwoRapidTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 30),
	Increment: clock.ToCTime(time.Second * 2),
}
