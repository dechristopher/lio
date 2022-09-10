package lag

import (
	"sync"
	"time"
)

// Monitor is a struct used for tracking latencies
type Monitor struct {
	Name string
	mut  *sync.Mutex
	avg  MovingAverage
}

// Move lag tracker
var Move = MakeMonitor("move")

// MakeMonitor makes a new monitor with the given name
func MakeMonitor(name string) *Monitor {
	return &Monitor{
		Name: name,
		mut:  &sync.Mutex{},
		avg:  NewMovingAverage(),
	}
}

// Get EWMA lag for the given monitor
func (m *Monitor) Get() time.Duration {
	return time.Duration(m.avg.Value())
}

// Track EWMA lag for the given monitor
func (m *Monitor) Track(start time.Time) {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.avg.Add(float64(time.Since(start)))
}
