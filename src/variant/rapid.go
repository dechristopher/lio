package variant

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"time"

	"github.com/dechristopher/lio/clock"
)

// HalfZeroRapid is the half minute, one second increment rapid variant
var HalfZeroRapid = &wsv1.Variant{
	Name:     "½ + 1",
	HtmlName: "half-one-rapid",
	Group:    wsv1.VariantGroup_VARIANT_GROUP_RAPID,
	Control:  HalfZeroRapidTC,
}

// HalfZeroRapidTC is the half minute, zero second increment rapid time control
var HalfZeroRapidTC = &wsv1.TimeControl{
	Seconds:          clock.ToCTime(time.Second * 30).Seconds(),
	IncrementSeconds: clock.ToCTime(time.Second * 1).Seconds(),
}

// HalfTwoRapid is the half minute, two second increment rapid variant
var HalfTwoRapid = &wsv1.Variant{
	Name:     "½ + 2",
	HtmlName: "half-two-rapid",
	Group:    wsv1.VariantGroup_VARIANT_GROUP_RAPID,
	Control:  HalfTwoRapidTC,
}

// HalfTwoRapidTC is the one minute, two second increment rapid time control
var HalfTwoRapidTC = &wsv1.TimeControl{
	Seconds:          clock.ToCTime(time.Second * 30).Seconds(),
	IncrementSeconds: clock.ToCTime(time.Second * 2).Seconds(),
}
