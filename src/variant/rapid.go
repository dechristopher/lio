package variant

import (
	"time"

	"github.com/dechristopher/lio/clock"
)

// OneZeroRapid is the one minute, zero second increment rapid variant
var OneZeroRapid = Variant{
	Name:     "1 + 0",
	HTMLName: "one-zero-rapid",
	Group:    RapidGroup,
	Control:  OneZeroRapidTC,
}

// OneZeroRapidTC is the one minute, zero second increment rapid time control
var OneZeroRapidTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 60),
	Increment: clock.ToCTime(time.Second * 0),
}

// OneTwoRapid is the one minute, two second increment rapid variant
var OneTwoRapid = Variant{
	Name:     "1 + 2",
	HTMLName: "one-two-rapid",
	Group:    RapidGroup,
	Control:  OneTwoRapidTC,
}

// OneTwoRapidTC is the one minute, two second increment rapid time control
var OneTwoRapidTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 60),
	Increment: clock.ToCTime(time.Second * 2),
}
