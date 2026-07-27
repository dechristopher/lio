// Package sysinfo samples the running process: build identity, Go runtime
// health, and the in-process counters that describe what the instance is
// carrying.
//
// It exists so there is exactly one sampler behind both consumers — the
// loopback health listener's JSON (www/health.go) and the /system console's
// live panel. Two samplers drift, and an operator comparing a container probe
// against the page it links to should not have to wonder which one is lying.
//
// Everything here is process-local. On a single instance that is the whole
// site; if lio ever runs more than one, a sample describes the instance that
// produced it, and the surfaces that render it say so.
package sysinfo

import (
	"runtime"
	"runtime/debug"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/lag"
	"github.com/dechristopher/lio/room"
)

// Runtime is one sample of the process.
type Runtime struct {
	// Build identity: what is running, from where.
	Version  string // "v1.6.0+abc1234"
	Env      string // prod / dev / local
	GoVer    string // runtime.Version()
	Platform string // "linux/amd64"

	// Lifetime.
	BootTime time.Time
	Uptime   time.Duration

	// Scheduler. NumCPU is what the machine has; GOMAXPROCS what the runtime
	// will use — on a container with a CPU limit these disagree, and that gap is
	// a common cause of "why is it slow" that is invisible from inside the app.
	NumCPU     int
	GOMAXPROCS int
	Goroutines int
	// Cgo is the count of live cgo calls; a nonzero-and-climbing value on this
	// binary (pure Go) would mean something unexpected got linked in.
	Cgo int64

	// Memory. HeapAlloc is what is live; Sys is what the process has taken from
	// the OS and is the figure that matters against a container memory limit.
	HeapAlloc   uint64
	HeapSys     uint64
	HeapObjects uint64
	StackSys    uint64
	Sys         uint64
	// GCTarget is the heap size the next collection is aimed at (NextGC).
	GCTarget uint64
	// MemLimit is GOMEMLIMIT, or 0 when unset (the default math.MaxInt64).
	MemLimit int64

	// Garbage collection. GCCPUFrac is the share of total process CPU spent
	// collecting since boot — the one GC number worth alarming on.
	NumGC      uint32
	LastGC     time.Time
	LastPause  time.Duration
	TotalPause time.Duration
	GCCPUFrac  float64
	ForcedGC   uint32

	// Workload: what this instance is actually carrying.
	Rooms int
	// Sockets is the number of connected websockets summed across channels, and
	// Channels how many channels hold at least one. Sockets exceeds the number
	// of people — one player with two tabs is two sockets.
	Sockets  int
	Channels int
	// MoveLag is the EWMA (~1min) of server move-processing time, the same
	// figure that drives clock lag compensation.
	MoveLag time.Duration
}

// Sample reads the process. runtime.ReadMemStats briefly stops the world, which
// is negligible at the cadences this is called at (a container healthcheck, and
// a console page an operator has open) but is the reason this is not something
// to call per request on a hot path.
func Sample() Runtime {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	sockets, channels := socketCounts()

	r := Runtime{
		Version:    config.VersionString(),
		Env:        string(env.GetEnv()),
		GoVer:      runtime.Version(),
		Platform:   runtime.GOOS + "/" + runtime.GOARCH,
		BootTime:   config.BootTime,
		Uptime:     time.Since(config.BootTime),
		NumCPU:     runtime.NumCPU(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		Goroutines: runtime.NumGoroutine(),
		Cgo:        runtime.NumCgoCall(),

		HeapAlloc:   mem.HeapAlloc,
		HeapSys:     mem.HeapSys,
		HeapObjects: mem.HeapObjects,
		StackSys:    mem.StackSys,
		Sys:         mem.Sys,
		GCTarget:    mem.NextGC,
		MemLimit:    memLimit(),

		NumGC:      mem.NumGC,
		TotalPause: time.Duration(mem.PauseTotalNs),
		GCCPUFrac:  mem.GCCPUFraction,
		ForcedGC:   mem.NumForcedGC,

		Rooms:    room.Count(),
		Sockets:  sockets,
		Channels: channels,
		MoveLag:  lag.Move.Get(),
	}

	// PauseNs is a 256-entry ring indexed by (NumGC+255)%256; reading it before
	// the first collection would report a pause that never happened.
	if mem.NumGC > 0 {
		r.LastGC = time.Unix(0, int64(mem.LastGC))
		r.LastPause = time.Duration(mem.PauseNs[(mem.NumGC+255)%256])
	}

	return r
}

// memLimit reports GOMEMLIMIT, normalising the "unset" sentinel to 0 so callers
// can render it as "none" rather than as 8 exabytes.
func memLimit() int64 {
	limit := debug.SetMemoryLimit(-1) // -1 reads without setting
	if limit == 1<<63-1 {
		return 0
	}
	return limit
}

// socketCounts sums live websockets across every channel. channel.Map is a
// sync.Map of *channel.SockMap, each independently locked, so this is safe to
// walk from any goroutine — it is a sample, not a consistent snapshot, which is
// the right trade for a counter nobody acts on transactionally.
func socketCounts() (sockets, channels int) {
	channel.Map.Range(func(_, raw any) bool {
		sockMap, ok := raw.(*channel.SockMap)
		if !ok {
			return true
		}
		if n := sockMap.Length(); n > 0 {
			sockets += n
			channels++
		}
		return true
	})
	return sockets, channels
}
