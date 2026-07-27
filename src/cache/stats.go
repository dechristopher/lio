package cache

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Operational stats for the /system console: go-redis' client-side pool
// counters plus the server's own INFO, and the two key counts that say what lio
// is actually keeping in there.
//
// INFO is a single round trip against sections Redis maintains continuously, so
// this is cheap enough to poll. The counts are deliberately not: DBSIZE is O(1),
// but the room-snapshot count uses a cursored SCAN rather than KEYS, because
// KEYS blocks the server for the duration of the walk and this is a page a
// moderator may leave open.

// probeTimeout bounds the console's probe. Tighter than opTimeout for the same
// reason as in db: during an incident the panel must render "unreachable"
// promptly instead of hanging on the same deadline the persister uses.
const probeTimeout = time.Second

// Stats is the cache's operational picture.
type Stats struct {
	// Configured is whether an address was supplied at all; Reachable whether
	// the probe just succeeded. Local dev without Redis is the former without
	// the latter, and must not read as an outage.
	Configured bool
	Reachable  bool
	Addr       string
	Latency    time.Duration
	Err        string

	// Client-side pool. Timeouts and StaleConns are the ones worth watching:
	// both mean the client is churning connections rather than reusing them.
	Hits       uint32
	Misses     uint32
	Timeouts   uint32
	TotalConns uint32
	IdleConns  uint32
	StaleConns uint32

	// Server, from INFO.
	Version          string
	ServerUptime     time.Duration
	ConnectedClients int64
	UsedMemory       int64
	UsedMemoryPeak   int64
	MaxMemory        int64 // 0 = no limit configured
	MemFragRatio     float64
	EvictedKeys      int64
	ExpiredKeys      int64
	TotalCommands    int64
	OpsPerSec        int64
	KeyspaceHits     int64
	KeyspaceMisses   int64
	RejectedConns    int64

	// What lio is storing. Keys is the whole database; RoomSnapshots the subset
	// under lio's room namespace — the two differ once anything else shares the
	// instance, and the gap is worth being able to see.
	Keys          int64
	RoomSnapshots int64
}

// snapshotScanCount is the SCAN batch size for counting room snapshots. Room
// counts are small (tens), so this walks the keyspace in one or two batches
// without ever blocking the server the way KEYS would.
const snapshotScanCount = 500

// GetStats samples the cache. Like db.GetStats it never returns an error — an
// unreachable cache is a result to display, not a reason to fail the page.
func GetStats() Stats {
	s := Stats{Configured: C != nil}
	if !s.Configured {
		return s
	}
	s.Addr = C.Options().Addr

	if p := C.PoolStats(); p != nil {
		s.Hits = p.Hits
		s.Misses = p.Misses
		s.Timeouts = p.Timeouts
		s.TotalConns = p.TotalConns
		s.IdleConns = p.IdleConns
		s.StaleConns = p.StaleConns
	}

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	start := time.Now()
	if err := C.Ping(ctx).Err(); err != nil {
		s.Latency = time.Since(start)
		s.Err = err.Error()
		return s
	}
	s.Latency = time.Since(start)
	s.Reachable = true

	if raw, err := C.Info(ctx, "server", "clients", "memory", "stats").Result(); err == nil {
		applyInfo(&s, parseInfo(raw))
	} else {
		s.Err = err.Error()
	}

	if n, err := C.DBSize(ctx).Result(); err == nil {
		s.Keys = n
	}
	s.RoomSnapshots = countRoomSnapshots(ctx)

	return s
}

// countRoomSnapshots walks lio's room namespace with a cursor. A scan that
// errors part way returns what it counted rather than zero: a partial count is
// still informative, and the panel shows it beside the total key count anyway.
func countRoomSnapshots(ctx context.Context) int64 {
	var n int64
	iter := C.Scan(ctx, 0, roomKeyPrefix+"*", snapshotScanCount).Iterator()
	for iter.Next(ctx) {
		n++
	}
	return n
}

// parseInfo turns an INFO reply into a field map. The reply is CRLF-delimited
// `key:value` lines with `# Section` headers and blank lines between sections;
// anything that is not a key/value pair is skipped.
func parseInfo(raw string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[key] = value
	}
	return fields
}

// applyInfo copies the fields worth showing out of an INFO map. Every field is
// optional: INFO's shape varies across Redis versions and forks (Valkey,
// managed providers), and a missing key must leave a zero rather than break the
// panel.
func applyInfo(s *Stats, f map[string]string) {
	s.Version = f["redis_version"]
	s.ServerUptime = time.Duration(infoInt(f, "uptime_in_seconds")) * time.Second
	s.ConnectedClients = infoInt(f, "connected_clients")
	s.UsedMemory = infoInt(f, "used_memory")
	s.UsedMemoryPeak = infoInt(f, "used_memory_peak")
	s.MaxMemory = infoInt(f, "maxmemory")
	s.MemFragRatio = infoFloat(f, "mem_fragmentation_ratio")
	s.EvictedKeys = infoInt(f, "evicted_keys")
	s.ExpiredKeys = infoInt(f, "expired_keys")
	s.TotalCommands = infoInt(f, "total_commands_processed")
	s.OpsPerSec = infoInt(f, "instantaneous_ops_per_sec")
	s.KeyspaceHits = infoInt(f, "keyspace_hits")
	s.KeyspaceMisses = infoInt(f, "keyspace_misses")
	s.RejectedConns = infoInt(f, "rejected_connections")
}

func infoInt(f map[string]string, key string) int64 {
	n, _ := strconv.ParseInt(f[key], 10, 64)
	return n
}

func infoFloat(f map[string]string, key string) float64 {
	v, _ := strconv.ParseFloat(f[key], 64)
	return v
}
