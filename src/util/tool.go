package util

import (
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/rng"
)

// DoBothColors runs the given function on both colors
func DoBothColors(f func(color octad.Color)) {
	f(octad.White)
	f(octad.Black)
}

// BothColors returns true if a provided check function returns true
// when given both white and black
func BothColors(check func(color octad.Color) bool) bool {
	return check(octad.White) && check(octad.Black)
}

// RandomColor randomly returns either white or black
func RandomColor() octad.Color {
	if rng.Bool() {
		return octad.White
	}
	return octad.Black
}

// MilliTime returns the current millisecond time
func MilliTime() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// TimeSinceBoot returns the time elapsed since process boot
func TimeSinceBoot() time.Duration {
	return time.Since(config.BootTime)
}
