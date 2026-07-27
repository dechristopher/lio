package view

import (
	"strings"
	"testing"
	"time"

	"github.com/dechristopher/lio/cache"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/store"
	"github.com/dechristopher/lio/sysinfo"
)

func TestBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{-1, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{10 * 1024, "10.0 KB"},
		{200 * 1024, "200 KB"},
		{5 * 1024 * 1024, "5.00 MB"},
		{3 * 1024 * 1024 * 1024, "3.00 GB"},
	}
	for _, c := range cases {
		if got := bytes(c.in); got != c.want {
			t.Errorf("bytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCount(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{99999, "99,999"},
		// past five figures it abbreviates: the last digits of a move count are
		// noise in a panel this dense
		{100000, "100K"},
		{4127338, "4.13M"},
		{2500000000, "2.50B"},
	}
	for _, c := range cases {
		if got := count(c.in); got != c.want {
			t.Errorf("count(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSeparated(t *testing.T) {
	cases := map[int64]string{
		0: "0", 1: "1", 12: "12", 123: "123",
		1234: "1,234", 12345: "12,345", 123456: "123,456", 1234567: "1,234,567",
	}
	for in, want := range cases {
		if got := separated(in); got != want {
			t.Errorf("separated(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestShortDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0"},
		{500 * time.Nanosecond, "<1µs"},
		{1500 * time.Nanosecond, "1.50µs"},
		{250 * time.Microsecond, "250µs"},
		{2500 * time.Microsecond, "2.50ms"},
		{1500 * time.Millisecond, "1.50s"},
		// past a minute it hands off to the coarse renderer
		{90 * time.Second, "1m"},
	}
	for _, c := range cases {
		if got := shortDuration(c.in); got != c.want {
			t.Errorf("shortDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLongDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "—"},
		{45 * time.Second, "45s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{(2*60 + 15) * time.Minute, "2h 15m"},
		{50 * time.Hour, "2d 2h"},
		{48 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := longDuration(c.in); got != c.want {
			t.Errorf("longDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHitRate(t *testing.T) {
	if got := hitRate(0, 0); got != "—" {
		t.Errorf("hitRate(0,0) = %q, want an em dash", got)
	}
	if got := hitRate(3, 1); got != "75.00%" {
		t.Errorf("hitRate(3,1) = %q, want 75.00%%", got)
	}
}

// A backend that was never configured must not render like one that is down:
// local dev routinely runs without all three, and a panel that painted that red
// would teach everyone to ignore red.
func TestUnconfiguredIsNotAnOutage(t *testing.T) {
	s := SystemStatsOf(sysinfo.Runtime{}, db.Stats{}, cache.Stats{}, store.Stats{})
	if len(s.Backends) != 3 {
		t.Fatalf("got %d backends, want 3", len(s.Backends))
	}
	for _, b := range s.Backends {
		if b.Status != "not configured" {
			t.Errorf("%s: status = %q, want %q", b.Name, b.Status, "not configured")
		}
		if b.Class != statOff {
			t.Errorf("%s: class = %q, want the neutral %q", b.Name, b.Class, statOff)
		}
		if b.Err == "" {
			t.Errorf("%s: unconfigured backend should say what is missing", b.Name)
		}
		// nothing was probed, so nothing should claim a probe result
		if b.Latency != "" {
			t.Errorf("%s: latency = %q, want empty for an unprobed backend", b.Name, b.Latency)
		}
		if len(b.Sections) != 0 {
			t.Errorf("%s: unconfigured backend should render no detail sections", b.Name)
		}
	}
}

// A reachable backend reports its probe latency and its detail; an unreachable
// one reports the error and no latency-implied health.
func TestBackendStates(t *testing.T) {
	up := SystemStatsOf(sysinfo.Runtime{},
		db.Stats{Configured: true, Reachable: true, Latency: 2 * time.Millisecond, MaxConns: 4},
		cache.Stats{Configured: true, Reachable: true, Addr: "localhost:6379"},
		store.Stats{Configured: true, Reachable: true, Endpoint: "obj.example", Bucket: "pgn"})

	for _, b := range up.Backends {
		if b.Status != "online" || b.Class != statOK {
			t.Errorf("%s: got %q/%q, want online", b.Name, b.Status, b.Class)
		}
		if b.Err != "" {
			t.Errorf("%s: healthy backend carries error %q", b.Name, b.Err)
		}
	}
	if got := up.Backends[0].Latency; got != "2.00ms" {
		t.Errorf("postgres latency = %q, want 2.00ms", got)
	}
	if got := up.Backends[1].Detail; got != "localhost:6379" {
		t.Errorf("redis detail = %q, want the address", got)
	}
	if got := up.Backends[2].Detail; got != "obj.example / pgn" {
		t.Errorf("store detail = %q, want endpoint / bucket", got)
	}

	down := SystemStatsOf(sysinfo.Runtime{},
		db.Stats{Configured: true, Err: "connection refused"},
		cache.Stats{Configured: true, Err: "i/o timeout"},
		store.Stats{Configured: true, Err: "no such host"})
	for _, b := range down.Backends {
		if b.Status != "unreachable" || b.Class != statBad {
			t.Errorf("%s: got %q/%q, want unreachable", b.Name, b.Status, b.Class)
		}
		if b.Err == "" {
			t.Errorf("%s: unreachable backend must surface the error", b.Name)
		}
	}
}

// Failure counters are the panel's whole reason for existing on the archive
// side: a PGN write failure is logged and dropped, so this is the only lasting
// trace. A nonzero count must be tinted, and a zero one must not be.
func TestFailureCountersAreTinted(t *testing.T) {
	clean := postgresView(db.Stats{Configured: true, Reachable: true})
	if got := rowClass(t, clean, "Failed"); got != "" {
		t.Errorf("zero failures tinted %q, want untinted", got)
	}

	dirty := postgresView(db.Stats{
		Configured: true, Reachable: true, ArchiveFail: 3,
		LastFailure: &db.ArchiveFailure{When: time.Now(), Err: "deadlock detected"},
	})
	if got := rowClass(t, dirty, "Failed"); got != statBad {
		t.Errorf("failures tinted %q, want %q", got, statBad)
	}
	// the failure's message rides the row's tooltip rather than the page body:
	// it is a raw driver error, useful on demand and noise inline
	row := findRow(t, dirty, "Last failure")
	if !strings.Contains(row.Help, "deadlock detected") {
		t.Errorf("last failure help = %q, want the driver error", row.Help)
	}
}

// A pool that has never had to wait should be entirely untinted — the panel's
// contract is that anything coloured is worth reading.
func TestHealthyPoolIsUntinted(t *testing.T) {
	v := postgresView(db.Stats{Configured: true, Reachable: true, MaxConns: 10, AcquiredConns: 2})
	for _, sec := range v.Sections {
		for _, r := range sec.Rows {
			if r.Class != "" {
				t.Errorf("healthy pool tinted %q = %q", r.Label, r.Class)
			}
		}
	}
}

// GOMAXPROCS below the machine's core count is the classic container CPU-limit
// trap, invisible from inside the process unless something says so.
func TestCappedSchedulerIsFlagged(t *testing.T) {
	v := runtimeView(sysinfo.Runtime{NumCPU: 8, GOMAXPROCS: 2})
	rows := sectionRows(t, v.Sections, "Scheduler")
	for _, r := range rows {
		if r.Label == "GOMAXPROCS" && r.Class != statWarn {
			t.Errorf("capped GOMAXPROCS tinted %q, want %q", r.Class, statWarn)
		}
	}

	uncapped := runtimeView(sysinfo.Runtime{NumCPU: 8, GOMAXPROCS: 8})
	for _, r := range sectionRows(t, uncapped.Sections, "Scheduler") {
		if r.Label == "GOMAXPROCS" && r.Class != "" {
			t.Errorf("uncapped GOMAXPROCS tinted %q, want untinted", r.Class)
		}
	}
}

// Every figure on the panel is labelled in internal vocabulary, so every row
// must carry the sentence that explains it.
func TestEveryRowIsExplained(t *testing.T) {
	s := SystemStatsOf(
		sysinfo.Runtime{NumCPU: 4, GOMAXPROCS: 4},
		db.Stats{Configured: true, Reachable: true, Tables: []db.TableStat{{Name: "moves", Rows: 10, Bytes: 2048}}},
		cache.Stats{Configured: true, Reachable: true, Addr: "localhost:6379"},
		store.Stats{Configured: true, Reachable: true, Endpoint: "e", Bucket: "b"},
	)
	sections := append([]StatSection{}, s.Runtime.Sections...)
	for _, b := range s.Backends {
		sections = append(sections, b.Sections...)
	}
	if len(sections) == 0 {
		t.Fatal("no sections rendered")
	}
	for _, sec := range sections {
		if sec.Title == "" {
			t.Error("section with no title")
		}
		for _, r := range sec.Rows {
			if r.Label == "" || r.Value == "" || r.Help == "" {
				t.Errorf("section %q row %+v is missing a label, value or explanation", sec.Title, r)
			}
		}
	}
	for _, tile := range s.Runtime.Tiles {
		if tile.Label == "" || tile.Value == "" || tile.Help == "" {
			t.Errorf("tile %+v is missing a label, value or explanation", tile)
		}
	}
}

// --- helpers ----------------------------------------------------------------

func sectionRows(t *testing.T, sections []StatSection, title string) []StatRow {
	t.Helper()
	for _, sec := range sections {
		if sec.Title == title {
			return sec.Rows
		}
	}
	t.Fatalf("no section %q", title)
	return nil
}

func findRow(t *testing.T, v BackendView, label string) StatRow {
	t.Helper()
	for _, sec := range v.Sections {
		for _, r := range sec.Rows {
			if r.Label == label {
				return r
			}
		}
	}
	t.Fatalf("no row %q in %s", label, v.Name)
	return StatRow{}
}

func rowClass(t *testing.T, v BackendView, label string) string {
	t.Helper()
	return findRow(t, v, label).Class
}
