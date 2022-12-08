package variant

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"time"

	"github.com/dechristopher/lio/clock"
)

// ThreeZeroHyper is the three second, zero second increment hyper variant
var ThreeZeroHyper = &wsv1.Variant{
	Name:     ":03",
	HtmlName: "three-zero-hyper",
	Group:    wsv1.VariantGroup_VARIANT_GROUP_HYPER,
	Control:  ThreeZeroHyperTC,
}

// ThreeZeroHyperTC is the three second, zero second increment hyper time control
var ThreeZeroHyperTC = &wsv1.TimeControl{
	Seconds:          clock.ToCTime(time.Second * 3).Seconds(),
	IncrementSeconds: clock.ToCTime(time.Second * 0).Seconds(),
}
