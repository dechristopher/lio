package variant

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"time"

	"github.com/dechristopher/lio/clock"
)

// FiveZeroBullet is the 5 second, zero second increment bullet variant
var FiveZeroBullet = &wsv1.Variant{
	Name:     ":05",
	HtmlName: "five-zero-bullet",
	Group:    wsv1.VariantGroup_VARIANT_GROUP_BULLET,
	Control:  FiveZeroBulletTC,
}

// FiveZeroBulletTC is the 5 second, zero second increment bullet time control
var FiveZeroBulletTC = &wsv1.TimeControl{
	Seconds:          clock.ToCTime(time.Second * 5).Seconds(),
	IncrementSeconds: clock.ToCTime(time.Second * 0).Seconds(),
}

// FiveOneBullet is the 5 second, one second increment bullet variant
var FiveOneBullet = &wsv1.Variant{
	Name:     ":05 + 1",
	HtmlName: "five-one-bullet",
	Group:    wsv1.VariantGroup_VARIANT_GROUP_BULLET,
	Control:  FiveOneBulletTC,
}

// FiveOneBulletTC is the 5 second, one second increment bullet time control
var FiveOneBulletTC = &wsv1.TimeControl{
	Seconds:          clock.ToCTime(time.Second * 5).Seconds(),
	IncrementSeconds: clock.ToCTime(time.Second * 1).Seconds(),
}
