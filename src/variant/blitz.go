package variant

import (
	"time"

	"github.com/dechristopher/lioctad/clock"
)

// HalfZeroBlitz is the 30 second, zero second increment blitz variant
var HalfZeroBlitz = Variant{
	Name:  "½ + 0 Blitz",
	Group: BlitzGroup,
	Time:  HalfZeroBlitzTC,
}

// HalfZeroBlitzTC is the 30 second, zero second increment blitz time control
var HalfZeroBlitzTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 30),
	Increment: clock.ToCTime(time.Second * 0),
}

// HalfOneBlitz is the 30 second, one second increment blitz variant
var HalfOneBlitz = Variant{
	Name:  "½ + 1 Blitz",
	Group: BlitzGroup,
	Time:  HalfOneBlitzTC,
}

// HalfOneBlitzTC is the 30 second, one second increment blitz time control
var HalfOneBlitzTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 30),
	Increment: clock.ToCTime(time.Second * 1),
}
