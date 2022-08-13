package util

import (
	"time"

	"github.com/dechristopher/lioctad/config"
)

// MilliTime returns the current millisecond time
func MilliTime() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// TimeSinceBoot returns the time elapsed since process boot
func TimeSinceBoot() time.Duration {
	return time.Since(config.BootTime)
}
