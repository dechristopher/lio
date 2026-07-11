package www

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/lag"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// status is the JSON payload served by the internal health listener. Because
// the listener is loopback-only, it can carry operational detail (load,
// latency, runtime health) that the old public /lio route never could.
type status struct {
	Version  string  `json:"v"`      // current lio version
	Uptime   float64 `json:"uptime"` // uptime in seconds
	BootTime int64   `json:"boot"`   // time started, unix timestamp

	// MoveLagMs is the EWMA (~1min window) of server move-processing time —
	// the room routine's cost from dequeuing a move to finishing the game-over
	// check (lag.Move, tracked in room/handle_03_game_ongoing.go). The same
	// figure drives clock lag compensation and the client ClockPayload.
	MoveLagMs float64 `json:"move_lag_ms"`

	Rooms      int    `json:"rooms"`      // active rooms (all states)
	Players    int    `json:"players"`    // connected uids summed across ws channels
	Goroutines int    `json:"goroutines"` // runtime.NumGoroutine
	HeapBytes  uint64 `json:"heap_bytes"` // live heap (MemStats.HeapAlloc)
	GCRuns     uint32 `json:"gc_runs"`    // completed GC cycles since boot
}

// currentStatus samples the process for the health payload. Everything read
// here is independently synchronized (sync.Maps, the lag monitor's mutex), so
// sampling is safe from the health listener's goroutine; ReadMemStats' brief
// stop-the-world is negligible at healthcheck cadence.
func currentStatus() status {
	players := 0
	channel.Map.Range(func(_, raw any) bool {
		if sockMap, ok := raw.(*channel.SockMap); ok {
			players += sockMap.Length()
		}
		return true
	})

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return status{
		Version:    config.Version,
		Uptime:     util.TimeSinceBoot().Seconds(),
		BootTime:   config.BootTime.UnixNano(),
		MoveLagMs:  float64(lag.Move.Get()) / float64(time.Millisecond),
		Rooms:      room.Count(),
		Players:    players,
		Goroutines: runtime.NumGoroutine(),
		HeapBytes:  mem.HeapAlloc,
		GCRuns:     mem.NumGC,
	}
}

// serveHealth runs the internal health listener: a bare net/http server on
// the loopback-only config.GetHealthAddr(), separate from the public fiber
// app so the status endpoint is never exposed outside the container. It
// serves GET /lio (the lightweight status JSON above) for `lio --health` to
// probe from inside the same network namespace — the probe only checks for a
// 200; the payload detail is for operators (and any future scraper).
// The listener lives and dies with the process; a bind failure is logged and
// surfaces as the container going unhealthy, not as a crash.
func serveHealth() {
	mux := http.NewServeMux()
	mux.HandleFunc("/lio", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(currentStatus())
	})

	srv := &http.Server{
		Addr:         config.GetHealthAddr(),
		Handler:      mux,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		util.Error(str.CMain, "health listener failed: %s", err.Error())
	}
}
