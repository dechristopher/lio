package variant

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"time"
)

// ZeroFiveUlti is the zero second, five second delay ulti variant
var ZeroFiveUlti = &wsv1.Variant{
	Name:     ":00 ~5",
	HtmlName: "zero-five-ulti",
	Group:    wsv1.VariantGroup_VARIANT_GROUP_ULTI,
	Control:  ZeroFiveUltiTC,
}

// ZeroFiveUltiTC is the zero second, five-second delay ulti time control
var ZeroFiveUltiTC = &wsv1.TimeControl{
	InitialTime: time.Second.Nanoseconds() * 0,
	Increment:   time.Second.Nanoseconds() * 0,
	Delay:       time.Second.Nanoseconds() * 5,
}
