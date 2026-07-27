package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/dechristopher/lio/cache"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/store"
	"github.com/dechristopher/lio/sysinfo"
)

// The /system console's instance panel: the Go process and the three backing
// services it depends on.
//
// This is the layer that turns raw counters into something an operator can read
// at a glance. Two rules shape it:
//
//   - Every number is labelled and tooltipped. "EmptyAcquireCount: 12" means
//     nothing to someone who has not read pgxpool's source; "waited for a
//     connection 12 times" is the same fact, usable.
//   - "Not configured" and "unreachable" never render alike. Local dev runs
//     without Redis, Postgres and the object store as a matter of course, and a
//     panel that painted that red would train everyone to ignore red.

// SystemStats is the whole panel, as rendered.
type SystemStats struct {
	// Sampled is when this sample was taken, in UTC. The panel refreshes itself,
	// so a timestamp is what tells an operator whether they are looking at live
	// data or at a tab that stopped polling an hour ago.
	Sampled string
	Runtime RuntimeView
	// Backends is Postgres, Redis, then the object store — ordered by how much
	// of the site stops working without them.
	Backends []BackendView
}

// RuntimeView is the Go process.
type RuntimeView struct {
	// Identity line: what is running, built from what, on what.
	Version  string
	Env      string
	GoVer    string
	Platform string
	Uptime   string
	// BootExact is the absolute boot timestamp, shown on hover — "3 days" is
	// what you read, but a restart is something you correlate against a clock.
	BootExact string

	Tiles    []StatTile
	Sections []StatSection
}

// BackendView is one backing service.
type BackendView struct {
	Name   string
	Status string
	// Class tints the status pill: reusing the site's semantic tokens, so an
	// unreachable database reads the same red as a lost game.
	Class string
	// Detail identifies the instance (address, bucket) without ever carrying a
	// credential — the DSN and Redis password are read from secrets and stay
	// there.
	Detail   string
	Latency  string
	Err      string
	Sections []StatSection
}

// StatSection groups related rows under a heading.
type StatSection struct {
	Title string
	Rows  []StatRow
}

// StatRow is one labelled figure.
type StatRow struct {
	Label string
	Value string
	Help  string
	// Class optionally tints the value when it is worth noticing.
	Class string
}

// StatTile is a headline number, rendered large.
type StatTile struct {
	Label string
	Value string
	Help  string
	Class string
}

// Status pill / value tints.
const (
	statOK   = "sys-ok"
	statWarn = "sys-warn"
	statBad  = "sys-bad"
	statOff  = "sys-off"
)

// moveLagWarn is the move-processing EWMA above which the tile is tinted. It is
// a display hint rather than an SLO: normal is well under a millisecond, so
// anything at this scale means the room routines are contending for something
// and is worth an operator's attention.
const moveLagWarn = 25 * time.Millisecond

// SystemStatsOf assembles the panel from one sample of each source.
func SystemStatsOf(rt sysinfo.Runtime, pg db.Stats, rd cache.Stats, obj store.Stats) SystemStats {
	return SystemStats{
		Sampled:  time.Now().UTC().Format("15:04:05 MST"),
		Runtime:  runtimeView(rt),
		Backends: []BackendView{postgresView(pg), redisView(rd), objectView(obj)},
	}
}

// runtimeView renders the process.
func runtimeView(rt sysinfo.Runtime) RuntimeView {
	v := RuntimeView{
		Version:   rt.Version,
		Env:       rt.Env,
		GoVer:     rt.GoVer,
		Platform:  rt.Platform,
		Uptime:    longDuration(rt.Uptime),
		BootExact: rt.BootTime.UTC().Format("2006-01-02 15:04:05 MST"),
	}

	lagClass := ""
	if rt.MoveLag >= moveLagWarn {
		lagClass = statWarn
	}
	v.Tiles = []StatTile{
		{Label: "Goroutines", Value: count(int64(rt.Goroutines)),
			Help: "Live goroutines. Grows with rooms and connections; a number that climbs and never falls is a leak"},
		{Label: "Heap", Value: bytes(int64(rt.HeapAlloc)),
			Help: "Live heap objects right now, excluding memory freed but not yet returned to the OS"},
		{Label: "Sockets", Value: count(int64(rt.Sockets)),
			Help: "Open websockets across all channels. Higher than the people online — one player with two tabs is two sockets"},
		{Label: "Move lag", Value: shortDuration(rt.MoveLag), Class: lagClass,
			Help: "Rolling average of server move-processing time. The same figure that drives clock lag compensation"},
	}

	v.Sections = []StatSection{
		{
			Title: "Scheduler",
			Rows: []StatRow{
				{Label: "CPUs", Value: count(int64(rt.NumCPU)),
					Help: "Cores the machine reports"},
				{Label: "GOMAXPROCS", Value: count(int64(rt.GOMAXPROCS)), Class: procsClass(rt),
					Help: "Cores Go will actually schedule on. Below the CPU count means the runtime is capped — usually a container CPU limit"},
				{Label: "Rooms", Value: count(int64(rt.Rooms)),
					Help: "Room state machines held in memory, including finished rooms not yet reaped"},
				{Label: "Channels", Value: count(int64(rt.Channels)),
					Help: "Websocket channels with at least one connection"},
			},
		},
		{
			Title: "Memory",
			Rows: []StatRow{
				{Label: "Heap in use", Value: bytes(int64(rt.HeapAlloc)),
					Help: "Live objects"},
				{Label: "Heap reserved", Value: bytes(int64(rt.HeapSys)),
					Help: "Heap address space taken from the OS, including memory freed but retained for reuse"},
				{Label: "Objects", Value: count(int64(rt.HeapObjects)),
					Help: "Allocated heap objects"},
				{Label: "Stacks", Value: bytes(int64(rt.StackSys)),
					Help: "Goroutine stack memory"},
				{Label: "Process total", Value: bytes(int64(rt.Sys)),
					Help: "Everything the process has taken from the OS. This is the figure a container memory limit applies to"},
				{Label: "Next GC at", Value: bytes(int64(rt.GCTarget)),
					Help: "Heap size the next collection is targeting"},
				{Label: "Memory limit", Value: memLimitValue(rt.MemLimit),
					Help: "GOMEMLIMIT — the soft ceiling the collector works to keep the process under"},
			},
		},
		{
			Title: "Garbage collection",
			Rows: []StatRow{
				{Label: "Collections", Value: count(int64(rt.NumGC)),
					Help: "Completed GC cycles since boot"},
				{Label: "CPU share", Value: percent(rt.GCCPUFrac),
					Help: "Share of the process's total CPU time spent collecting since boot"},
				{Label: "Last pause", Value: shortDuration(rt.LastPause),
					Help: "Stop-the-world time of the most recent cycle"},
				{Label: "Total paused", Value: shortDuration(rt.TotalPause),
					Help: "Cumulative stop-the-world time since boot"},
				{Label: "Last run", Value: agoValue(rt.LastGC),
					Help: "When the most recent cycle finished"},
				{Label: "Forced", Value: count(int64(rt.ForcedGC)),
					Help: "Collections triggered by an explicit runtime.GC call rather than by the pacer"},
			},
		},
	}
	return v
}

// procsClass flags a runtime scheduling on fewer cores than the machine has —
// the standard container-CPU-limit trap, and invisible from inside the app
// unless something says so.
func procsClass(rt sysinfo.Runtime) string {
	if rt.GOMAXPROCS < rt.NumCPU {
		return statWarn
	}
	return ""
}

// postgresView renders the relational archive.
func postgresView(s db.Stats) BackendView {
	v := BackendView{Name: "Postgres", Detail: "durable games archive"}
	applyState(&v, s.Configured, s.Reachable, s.Latency, s.Err,
		"no DSN configured — finished games are not archived")
	if !s.Configured {
		return v
	}

	v.Sections = append(v.Sections, StatSection{
		Title: "Connection pool",
		Rows: []StatRow{
			{Label: "In use", Value: count(int64(s.AcquiredConns)) + " / " + count(int64(s.MaxConns)),
				Class: poolClass(s), Help: "Connections currently checked out, against the pool maximum"},
			{Label: "Idle", Value: count(int64(s.IdleConns)),
				Help: "Open connections sitting available for reuse"},
			{Label: "Opening", Value: count(int64(s.ConstructingConns)),
				Help: "Connections being established right now"},
			{Label: "Acquires", Value: count(s.AcquireCount),
				Help: "Total connection checkouts since boot"},
			{Label: "Waited", Value: count(s.EmptyAcquireCount), Class: warnIf(s.EmptyAcquireCount > 0),
				Help: "Checkouts that found the pool empty and had to wait. Persistently rising means the pool is too small"},
			{Label: "Wait time", Value: shortDuration(s.EmptyAcquireWait),
				Help: "Total time spent waiting on an empty pool since boot"},
			{Label: "Canceled", Value: count(s.CanceledAcquireCount), Class: warnIf(s.CanceledAcquireCount > 0),
				Help: "Checkouts abandoned before they got a connection — a request gave up or timed out"},
			{Label: "Connections made", Value: count(s.NewConnsCount),
				Help: "Connections established since boot. Far above the pool maximum means connections are churning rather than being reused"},
		},
	})

	if s.Reachable {
		storage := []StatRow{
			{Label: "Database size", Value: bytes(s.DatabaseBytes),
				Help: "Total on-disk size, including indexes"},
		}
		for _, t := range s.Tables {
			storage = append(storage, StatRow{
				Label: t.Name,
				Value: count(t.Rows) + " rows · " + bytes(t.Bytes),
				Help:  "Row count is the planner's estimate, not an exact count; size includes indexes",
			})
		}
		v.Sections = append(v.Sections, StatSection{Title: "Storage", Rows: storage})
	}

	v.Sections = append(v.Sections, StatSection{
		Title: "Archive writes",
		Rows:  archiveRows(s),
	})
	return v
}

// archiveRows describes how game archival has actually gone this boot, plus the
// most recent failure. A failed archive write is logged and dropped — the game
// is already over — so without this nothing would tell an operator that games
// have stopped being recorded.
func archiveRows(s db.Stats) []StatRow {
	rows := []StatRow{
		{Label: "Archived", Value: count(s.ArchiveOK),
			Help: "Games written to the archive since boot"},
		{Label: "Failed", Value: count(s.ArchiveFail), Class: badIf(s.ArchiveFail > 0),
			Help: "Games that could not be archived. These are lost from the relational archive; the PGN copy may still exist"},
		{Label: "Total on record", Value: count(db.TotalGames()),
			Help: "Every game ever archived, the count the home page shows"},
	}
	if f := s.LastFailure; f != nil {
		rows = append(rows, StatRow{
			Label: "Last failure", Value: agoValue(f.When), Class: statBad,
			Help: f.Err,
		})
	}
	return rows
}

// poolClass flags a pool running at its ceiling. Being fully checked out for an
// instant is normal; being there when someone looks is the shape of a pool
// about to start making requests wait.
func poolClass(s db.Stats) string {
	if s.MaxConns > 0 && s.AcquiredConns >= s.MaxConns {
		return statWarn
	}
	return ""
}

// redisView renders the room-snapshot cache.
func redisView(s cache.Stats) BackendView {
	v := BackendView{Name: "Redis", Detail: s.Addr}
	applyState(&v, s.Configured, s.Reachable, s.Latency, s.Err,
		"no address configured — live rooms will not survive a restart")
	if v.Detail == "" {
		v.Detail = "room snapshot persistence"
	}
	if !s.Configured {
		return v
	}

	if s.Reachable {
		v.Sections = append(v.Sections, StatSection{
			Title: "Server",
			Rows: []StatRow{
				{Label: "Version", Value: orDash(s.Version), Help: "Redis server version"},
				{Label: "Uptime", Value: longDuration(s.ServerUptime),
					Help: "How long the Redis server has been up. Shorter than lio's uptime means it restarted under us"},
				{Label: "Clients", Value: count(s.ConnectedClients),
					Help: "Clients connected to the server, lio's connections among them"},
				{Label: "Memory", Value: memoryValue(s),
					Help: "Memory in use, against the configured maxmemory limit if there is one"},
				{Label: "Peak memory", Value: bytes(s.UsedMemoryPeak),
					Help: "High-water mark since the server started"},
				{Label: "Fragmentation", Value: ratio(s.MemFragRatio),
					Help: "Memory the OS has given Redis divided by what Redis is using. Well above 1 means memory is being held but not used"},
				{Label: "Evicted keys", Value: count(s.EvictedKeys), Class: badIf(s.EvictedKeys > 0),
					Help: "Keys dropped to stay under maxmemory. Any eviction here can mean a live room's snapshot was thrown away"},
				{Label: "Expired keys", Value: count(s.ExpiredKeys),
					Help: "Keys removed by TTL. Room snapshots expire this way in the normal course of things"},
				{Label: "Rejected connections", Value: count(s.RejectedConns), Class: badIf(s.RejectedConns > 0),
					Help: "Connections refused because the server was at its client limit"},
			},
		})
		v.Sections = append(v.Sections, StatSection{
			Title: "Keyspace",
			Rows: []StatRow{
				{Label: "Room snapshots", Value: count(s.RoomSnapshots),
					Help: "Live rooms persisted for restart recovery"},
				{Label: "Keys total", Value: count(s.Keys),
					Help: "Every key in the database. Above the snapshot count means something else shares this instance"},
				{Label: "Commands", Value: count(s.TotalCommands),
					Help: "Commands the server has processed since it started"},
				{Label: "Ops/sec", Value: count(s.OpsPerSec),
					Help: "Commands per second, sampled by the server right now"},
				{Label: "Hit rate", Value: hitRate(s.KeyspaceHits, s.KeyspaceMisses),
					Help: "Share of key lookups that found something. Low is expected here — lio mostly writes snapshots and reads them once, at boot"},
			},
		})
	}

	v.Sections = append(v.Sections, StatSection{
		Title: "Client pool",
		Rows: []StatRow{
			{Label: "Connections", Value: count(int64(s.TotalConns)),
				Help: "Connections lio is holding to the server"},
			{Label: "Idle", Value: count(int64(s.IdleConns)),
				Help: "Pooled connections available for reuse"},
			{Label: "Reused", Value: count(int64(s.Hits)),
				Help: "Times a pooled connection was available"},
			{Label: "Created", Value: count(int64(s.Misses)),
				Help: "Times none was available and a new connection had to be made"},
			{Label: "Timeouts", Value: count(int64(s.Timeouts)), Class: badIf(s.Timeouts > 0),
				Help: "Waits for a pooled connection that timed out"},
			{Label: "Stale", Value: count(int64(s.StaleConns)),
				Help: "Connections retired for age or idleness"},
		},
	})
	return v
}

// objectView renders the PGN object store.
func objectView(s store.Stats) BackendView {
	v := BackendView{Name: "Object store", Detail: objectDetail(s)}
	applyState(&v, s.Configured, s.Reachable, s.Latency, s.Err,
		"no endpoint configured — PGNs are not written")
	if !s.Configured {
		return v
	}

	rows := []StatRow{
		{Label: "PGNs written", Value: count(s.PutOK),
			Help: "Game PGNs stored since boot"},
		{Label: "Writes failed", Value: count(s.PutFail), Class: badIf(s.PutFail > 0),
			Help: "PGNs that could not be stored. The game is over by then, so the failure is logged and dropped — this counter is the only other trace"},
		{Label: "Objects read", Value: count(s.GetOK),
			Help: "Objects fetched since boot, mostly archive page views"},
		{Label: "Reads failed", Value: count(s.GetFail), Class: warnIf(s.GetFail > 0),
			Help: "Fetches that failed"},
	}
	if f := s.LastFailure; f != nil {
		rows = append(rows, StatRow{
			Label: "Last failure", Value: f.Op + " " + agoValue(f.When), Class: statBad,
			Help: f.Key + ": " + f.Err,
		})
	}
	v.Sections = append(v.Sections, StatSection{Title: "Archive I/O", Rows: rows})
	return v
}

// objectDetail names the endpoint and bucket, or says what is missing.
func objectDetail(s store.Stats) string {
	switch {
	case s.Endpoint == "":
		return "game PGN archive"
	case s.Bucket == "":
		return s.Endpoint
	default:
		return s.Endpoint + " / " + s.Bucket
	}
}

// applyState fills in the status pill shared by every backend card. The three
// states are distinct on purpose: "off" is a deployment that never had this
// service, and a local dev box showing three grey pills is correct rather than
// alarming.
func applyState(v *BackendView, configured, reachable bool, latency time.Duration, errMsg, offNote string) {
	switch {
	case !configured:
		v.Status, v.Class, v.Err = "not configured", statOff, offNote
	case reachable:
		v.Status, v.Class = "online", statOK
		v.Latency = shortDuration(latency)
		// reachable but with an error means the probe passed and a follow-up
		// read did not — worth showing without calling the service down
		v.Err = errMsg
	default:
		v.Status, v.Class, v.Err = "unreachable", statBad, errMsg
	}
}

// --- formatting -------------------------------------------------------------

// bytes renders a byte count at three significant figures. Binary steps with
// the short unit names, which is what every other operational tool in this
// stack (Redis, Postgres, docker stats) prints.
func bytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	value := float64(n)
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	i := -1
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	return trimFloat(value) + " " + units[i]
}

// count renders a cardinal number: exact with thousands separators up to five
// figures, abbreviated above that. An archive with 4,127,338 moves is "4.13M"
// — nobody reading this panel needs the last three digits, and the full number
// costs more width than the whole row it sits in.
func count(n int64) string {
	if n < 0 {
		return "—"
	}
	if n < 100_000 {
		return separated(n)
	}
	value := float64(n)
	for _, suffix := range []string{"K", "M", "B"} {
		value /= 1000
		if value < 1000 {
			return trimFloat(value) + suffix
		}
	}
	return trimFloat(value) + "T"
}

// separated groups digits in threes.
func separated(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// trimFloat renders at three significant figures without a trailing ".0".
func trimFloat(v float64) string {
	switch {
	case v >= 100:
		return strconv.FormatFloat(v, 'f', 0, 64)
	case v >= 10:
		return strconv.FormatFloat(v, 'f', 1, 64)
	default:
		return strconv.FormatFloat(v, 'f', 2, 64)
	}
}

// shortDuration renders a latency or pause, picking the unit that keeps three
// significant figures. Sub-microsecond rounds to "0" rather than printing a
// nanosecond count nobody can act on.
func shortDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return "0"
	case d < time.Microsecond:
		return "<1µs"
	case d < time.Millisecond:
		return trimFloat(float64(d)/float64(time.Microsecond)) + "µs"
	case d < time.Second:
		return trimFloat(float64(d)/float64(time.Millisecond)) + "ms"
	case d < time.Minute:
		return trimFloat(d.Seconds()) + "s"
	default:
		return longDuration(d)
	}
}

// longDuration renders an uptime as the two coarsest units that matter: "3d 4h",
// "2h 15m", "40m". Seconds appear only under a minute, where they are the whole
// story.
func longDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) - h*60
		if m == 0 {
			return strconv.Itoa(h) + "h"
		}
		return strconv.Itoa(h) + "h " + strconv.Itoa(m) + "m"
	default:
		days := int(d.Hours()) / 24
		h := int(d.Hours()) - days*24
		if h == 0 {
			return strconv.Itoa(days) + "d"
		}
		return strconv.Itoa(days) + "d " + strconv.Itoa(h) + "h"
	}
}

// agoValue renders how long ago something happened, or an em dash if it never
// has.
func agoValue(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return longDuration(time.Since(t)) + " ago"
}

// percent renders a 0..1 fraction.
func percent(f float64) string {
	if f <= 0 {
		return "0%"
	}
	return strconv.FormatFloat(f*100, 'f', 2, 64) + "%"
}

// ratio renders a multiplier like Redis' fragmentation figure.
func ratio(f float64) string {
	if f <= 0 {
		return "—"
	}
	return strconv.FormatFloat(f, 'f', 2, 64) + "×"
}

// hitRate renders a hit/miss pair as the share that hit.
func hitRate(hits, misses int64) string {
	total := hits + misses
	if total == 0 {
		return "—"
	}
	return percent(float64(hits) / float64(total))
}

// memoryValue renders Redis memory against its limit, when one is set.
func memoryValue(s cache.Stats) string {
	if s.MaxMemory > 0 {
		return bytes(s.UsedMemory) + " / " + bytes(s.MaxMemory)
	}
	return bytes(s.UsedMemory) + " (no limit)"
}

// memLimitValue renders GOMEMLIMIT, which is unset far more often than not.
func memLimitValue(limit int64) string {
	if limit <= 0 {
		return "none"
	}
	return bytes(limit)
}

// orDash falls back to an em dash for a value the server did not report.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// warnIf / badIf tint a value only when it is nonzero, so a healthy panel is
// entirely untinted and anything coloured is worth reading.
func warnIf(cond bool) string {
	if cond {
		return statWarn
	}
	return ""
}

func badIf(cond bool) string {
	if cond {
		return statBad
	}
	return ""
}
