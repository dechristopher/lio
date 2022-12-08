package variant

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"time"

	"github.com/dechristopher/lio/clock"
)

// QuarterZeroBlitz is the 15 second, zero second increment blitz variant
var QuarterZeroBlitz = &wsv1.Variant{
	Name:     "¼",
	HtmlName: "quarter-zero-blitz",
	Group:    wsv1.VariantGroup_VARIANT_GROUP_BLITZ,
	Control:  QuarterZeroBlitzTC,
}

// QuarterZeroBlitzTC is the 15 second, zero second increment blitz time control
var QuarterZeroBlitzTC = &wsv1.TimeControl{
	Seconds:          clock.ToCTime(time.Second * 15).Seconds(),
	IncrementSeconds: clock.ToCTime(time.Second * 0).Seconds(),
}

// QuarterOneBlitz is the 15 second, one second increment blitz variant
var QuarterOneBlitz = &wsv1.Variant{
	Name:     "¼ + 1",
	HtmlName: "quarter-one-blitz",
	Group:    wsv1.VariantGroup_VARIANT_GROUP_BLITZ,
	Control:  QuarterOneBlitzTC,
}

// QuarterOneBlitzTC is the 15 second, one second increment blitz time control
var QuarterOneBlitzTC = &wsv1.TimeControl{
	Seconds:          clock.ToCTime(time.Second * 15).Seconds(),
	IncrementSeconds: clock.ToCTime(time.Second * 1).Seconds(),
}
