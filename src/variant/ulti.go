package variant

import (
	"time"

	"github.com/dechristopher/lioctad/clock"
)

// ZeroFiveUlti is the zero second, five second delay ulti variant
var ZeroFiveUlti = Variant{
	Name:     ":00 ~5",
	HTMLName: "zero-five-ulti",
	Group:    UltiGroup,
	Control:  ZeroFiveUltiTC,
}

// ZeroFiveUltiTC is the zero second, five-second delay ulti time control
var ZeroFiveUltiTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 0),
	Increment: clock.ToCTime(time.Second * 0),
	Delay:     clock.ToCTime(time.Second * 5),
}
