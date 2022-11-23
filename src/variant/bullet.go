package variant

import (
	"time"

	"github.com/dechristopher/lio/clock"
)

// FiveZeroBullet is the 5 second, zero second increment bullet variant
var FiveZeroBullet = Variant{
	Name:     ":05",
	HTMLName: "five-zero-bullet",
	Group:    BulletGroup,
	Control:  FiveZeroBulletTC,
}

// FiveZeroBulletTC is the 5 second, zero second increment bullet time control
var FiveZeroBulletTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 5),
	Increment: clock.ToCTime(time.Second * 0),
}

// FiveOneBullet is the 5 second, one second increment bullet variant
var FiveOneBullet = Variant{
	Name:     ":05 + 1",
	HTMLName: "five-one-bullet",
	Group:    BulletGroup,
	Control:  FiveOneBulletTC,
}

// FiveOneBulletTC is the 5 second, one second increment bullet time control
var FiveOneBulletTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 5),
	Increment: clock.ToCTime(time.Second * 1),
}
