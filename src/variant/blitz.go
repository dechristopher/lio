package variant

import (
	"time"

	"github.com/dechristopher/lio/clock"
)

// QuarterZeroBlitz is the 15 second, zero second increment blitz variant
var QuarterZeroBlitz = Variant{
	Name:     "¼",
	HTMLName: "quarter-zero-blitz",
	Group:    BlitzGroup,
	Control:  QuarterZeroBlitzTC,
}

// QuarterZeroBlitzTC is the 15 second, zero second increment blitz time control
var QuarterZeroBlitzTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 15),
	Increment: clock.ToCTime(time.Second * 0),
}

// QuarterOneBlitz is the 15 second, one second increment blitz variant
var QuarterOneBlitz = Variant{
	Name:     "¼ + 1",
	HTMLName: "quarter-one-blitz",
	Group:    BlitzGroup,
	Control:  QuarterOneBlitzTC,
}

// QuarterOneBlitzTC is the 15 second, one second increment blitz time control
var QuarterOneBlitzTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 15),
	Increment: clock.ToCTime(time.Second * 1),
}
