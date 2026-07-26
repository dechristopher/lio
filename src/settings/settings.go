// Package settings holds the runtime site controls an admin can change without
// a deploy (arch/ADMIN_MODERATION.md Phase 3): the site notice banner, whether
// registration is open, whether new games are rated, and maintenance mode.
//
// Every switch is *operational*. Nothing here can change the outcome of a game
// — only whether new ones can start, and what visitors are told. That boundary
// is deliberate: an admin control that could alter play would be a control
// worth attacking.
//
// Reads come from a whole-table snapshot refreshed on a short TTL, so the hot
// paths that consult it (page renders, room creation) never touch Postgres.
// The TTL doubles as the propagation delay when more than one instance is
// running — a change is live everywhere within refreshTTL, with no pubsub to
// operate. A write invalidates the local snapshot immediately, so the admin who
// flipped a switch sees it take effect on their very next request rather than
// wondering whether it worked.
package settings

import (
	"sync"
	"time"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// refreshTTL bounds both snapshot staleness and cross-instance propagation.
const refreshTTL = 5 * time.Second

// Keys are the stored setting names. An absent row means the built-in default,
// so this table starts empty and only records what an admin actually changed.
const (
	KeyNoticeText   = "notice.text"
	KeyNoticeLevel  = "notice.level"
	KeyRegistration = "registration.enabled"
	KeyRated        = "rated.enabled"
	KeyMaintenance  = "maintenance.mode"
)

// Notice levels, styling the banner. Anything unrecognized reads as info.
const (
	LevelInfo = "info"
	LevelWarn = "warn"
)

// Snapshot is the resolved state of every switch.
type Snapshot struct {
	// NoticeText is the site-wide banner shown on every page; empty renders
	// nothing. NoticeLevel styles it (info/warn).
	NoticeText  string
	NoticeLevel string
	// RegistrationOpen gates new account creation. Existing accounts are
	// unaffected — closing registration is not a lockout.
	RegistrationOpen bool
	// RatedEnabled gates whether *newly created* games count toward ratings.
	// Games already in flight keep the rated flag they were stamped with: a
	// game's own terms cannot change under the players mid-match.
	RatedEnabled bool
	// Maintenance stops new games from starting (creation and joining) while
	// letting every game in progress play out. It is the "quiet the site before
	// a restart" switch, and composes with the shutdown drain rather than
	// replacing it.
	Maintenance bool
}

// defaults is the state of a site with no overrides stored: everything open,
// nothing announced. It is also the fallback when Postgres is unconfigured
// (PG-less local dev) or unreadable — the site fails *open*, because a
// database blip must not silently close registration or halt play.
func defaults() Snapshot {
	return Snapshot{
		NoticeLevel:      LevelInfo,
		RegistrationOpen: true,
		RatedEnabled:     true,
	}
}

var cache = struct {
	sync.Mutex
	snap    Snapshot
	fetched time.Time
	loaded  bool
}{snap: defaults()}

// Current returns the live settings snapshot, refreshing it from Postgres when
// the cached copy has aged past refreshTTL. Safe for concurrent use and cheap
// enough for a render path.
func Current() Snapshot {
	cache.Lock()
	defer cache.Unlock()
	if cache.loaded && time.Since(cache.fetched) < refreshTTL {
		return cache.snap
	}
	raw, err := db.LoadSettings()
	if err != nil {
		util.Error(str.CDB, "settings load failed error=%s", err.Error())
		// keep serving the last good snapshot (or defaults) and retry after the
		// TTL rather than flapping the whole site on one failed query
		cache.fetched = time.Now()
		return cache.snap
	}
	cache.snap = resolve(raw)
	cache.fetched = time.Now()
	cache.loaded = true
	return cache.snap
}

// Invalidate drops the cached snapshot so the next Current re-reads. Called
// after a write so the admin who made the change sees it immediately.
func Invalidate() {
	cache.Lock()
	cache.loaded = false
	cache.Unlock()
}

// OverrideForTest pins the snapshot to s and returns a function restoring the
// previous state. It exists because the view layer now renders off these
// switches — a closed signup form, a locked casual toggle, a maintenance bar —
// and those states have to be assertable without a Postgres to write them to.
// The pin is a normal cache fill, so Current serves it until the caller
// restores or the TTL lapses.
func OverrideForTest(s Snapshot) (restore func()) {
	cache.Lock()
	prevSnap, prevFetched, prevLoaded := cache.snap, cache.fetched, cache.loaded
	cache.snap, cache.fetched, cache.loaded = s, time.Now(), true
	cache.Unlock()
	return func() {
		cache.Lock()
		cache.snap, cache.fetched, cache.loaded = prevSnap, prevFetched, prevLoaded
		cache.Unlock()
	}
}

// resolve overlays stored overrides onto the defaults.
func resolve(raw map[string]string) Snapshot {
	s := defaults()
	if v, ok := raw[KeyNoticeText]; ok {
		s.NoticeText = v
	}
	if v, ok := raw[KeyNoticeLevel]; ok && v == LevelWarn {
		s.NoticeLevel = LevelWarn
	}
	if v, ok := raw[KeyRegistration]; ok {
		s.RegistrationOpen = truthy(v)
	}
	if v, ok := raw[KeyRated]; ok {
		s.RatedEnabled = truthy(v)
	}
	if v, ok := raw[KeyMaintenance]; ok {
		s.Maintenance = truthy(v)
	}
	return s
}

// truthy reads a stored flag. Only "1" is true, so a malformed value reads as
// off — which for the two "enabled" switches means failing *closed*. That is
// the safe direction for a value that should only ever have been written by
// this package.
func truthy(v string) bool {
	return v == "1"
}

// Flag renders a bool for storage.
func Flag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
