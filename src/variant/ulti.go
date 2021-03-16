package variant

import "github.com/dechristopher/lioctad/clock"

// ZeroFiveUlti is the zero second, five second delay ulti variant
var ZeroFiveUlti = Variant{
	Name:  ":00 ~5",
	Group: UltiGroup,
	Time:  ZeroFiveUltiTC,
}

// ZeroFiveUltiTC is the zero second, five second delay ulti time control
var ZeroFiveUltiTC = clock.TimeControl{
	Time:      0,
	Increment: 0,
	Delay:     5,
}
