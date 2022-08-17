package lag

import "time"

type Monitor string

const Move Monitor = "move"

var monitors = make(map[Monitor]MovingAverage)

// Get EWMA lag for the given monitor
func (m Monitor) Get() time.Duration {
	if monitors[m] == nil {
		monitors[m] = NewMovingAverage()
	}

	return time.Duration(monitors[m].Value())
}

// Track EWMA lag for the given monitor
func (m Monitor) Track(start time.Time) {
	if monitors[m] == nil {
		monitors[m] = NewMovingAverage()
	}

	dur := time.Since(start)
	monitors[m].Add(float64(dur))
}
