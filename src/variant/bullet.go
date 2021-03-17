package variant

import (
	"time"

	"github.com/dechristopher/lioctad/clock"
)

// QuarterZeroBullet is the 15 second, zero second increment bullet variant
var QuarterZeroBullet = Variant{
	Name:  "¼ + 0 Bullet",
	Group: BulletGroup,
	Time:  QuarterZeroBulletTC,
}

// QuarterZeroBulletTC is the 15 second, zero second increment bullet time control
var QuarterZeroBulletTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 15),
	Increment: clock.ToCTime(time.Second * 0),
}

// QuarterOneBullet is the 15 second, one second increment bullet variant
var QuarterOneBullet = Variant{
	Name:  "¼ + 1 Bullet",
	Group: BulletGroup,
	Time:  QuarterOneBulletTC,
}

// QuarterOneBulletTC is the 15 second, one second increment bullet time control
var QuarterOneBulletTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Second * 15),
	Increment: clock.ToCTime(time.Second * 1),
}
