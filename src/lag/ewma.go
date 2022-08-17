package lag

const (
	// By default, we average over a one-minute period, which means the average
	// age of the metrics in the period is 30 seconds.
	avgMetricAge float64 = 30.0

	// The formula for computing the decay factor from the average age comes
	// from "Production and Operations Analysis" by Steven Nahmias.
	decay = 2 / (float64(avgMetricAge) + 1)
)

// MovingAverage is the interface that computes a moving average over a time-series
// stream of numbers. The average may be over a window or exponentially decaying.
type MovingAverage interface {
	Add(float64)
	Value() float64
}

// NewMovingAverage constructs a MovingAverage that computes an average with the
// desired characteristics in the moving window or exponential decay. If no
// age is given, it constructs a default exponentially weighted implementation
// that consumes minimal memory. The age is related to the decay factor alpha
// by the formula given for the decay constant. It signifies the average age
// of the samples as time goes to infinity.
func NewMovingAverage() MovingAverage {
	return new(SimpleEWMA)
}

// A SimpleEWMA represents the exponentially weighted moving average of a
// series of numbers. It has no warm-up period, and it uses a constant
// decay. These properties let it use less memory.  It will also behave
// differently when it's equal to zero, which is assumed to mean
// uninitialized, so if a value is likely to actually become zero over time,
// then any non-zero value will cause a sharp jump instead of a small change.
// However, note that this takes a long time, and the value may just
// decay to a stable value that's close to zero, but which won't be mistaken
// for uninitialized. See http://play.golang.org/p/litxBDr_RC for example.
type SimpleEWMA struct {
	// The current value of the average. After adding with Add(), this is
	// updated to reflect the average of all values seen thus far.
	value float64
}

// Add adds a value to the series and updates the moving average.
func (e *SimpleEWMA) Add(value float64) {
	if e.value == 0 { // this is a sentinel for "uninitialized"
		e.value = value
	} else {
		e.value = (value * decay) + (e.value * (1 - decay))
	}
}

// Value returns the current value of the moving average.
func (e *SimpleEWMA) Value() float64 {
	return e.value
}
