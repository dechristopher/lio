package variant

import "github.com/dechristopher/lioctad/clock"

// OneZeroRapid is the one minute, zero second increment rapid variant
var OneZeroRapid = Variant{
	Name:  "1 + 0 Rapid",
	Group: RapidGroup,
	Time:  OneZeroRapidTC,
}

// OneZeroRapidTC is the one minute, zero second increment rapid time control
var OneZeroRapidTC = clock.TimeControl{
	Time:      60,
	Increment: 0,
}

// OneTwoRapid is the one minute, two second increment rapid variant
var OneTwoRapid = Variant{
	Name:  "1 + 2 Rapid",
	Group: RapidGroup,
	Time:  OneTwoRapidTC,
}

// OneTwoRapidTC is the one minute, two second increment rapid time control
var OneTwoRapidTC = clock.TimeControl{
	Time:      60,
	Increment: 2,
}
