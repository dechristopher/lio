// Package prefs holds the per-account display preferences a signed-in player
// can change: the choices about what the site shows *them*, kept with the
// account so they follow the player from one device to the next.
//
// It is the per-account counterpart to the settings package, and is shaped like
// it on purpose — a key/value store whose values are TEXT and whose meaning
// lives here, so adding a preference costs a constant and nothing in SQL. An
// absent row means the built-in default, so a player's row set holds only what
// they actually changed.
//
// What belongs here is what has to follow the account. The color theme, the
// board and the piece set deliberately do not: they must resolve before first
// paint (view/layout.templ's no-flash script), and a round trip cannot. Those
// stay in localStorage, and so do the dismissals of the two anonymous account
// pitches — an anonymous visitor has no account to store anything against.
//
// Nothing here can change the outcome of a game, or what anybody else sees. A
// preference decides what its owner is shown, and only that. That boundary is
// what lets the write endpoint trust the session and check nothing else.
//
// Reads come from a per-account snapshot cached on a short TTL, because the
// header's preferences popover renders off them on every page. The TTL doubles
// as the cross-instance propagation delay: a change made on one instance is
// live on the others within refreshTTL, with no pubsub to operate. A write
// invalidates the local snapshot immediately, so the player who flipped a
// switch sees it hold on their very next request.
package prefs

import (
	"sync"
	"time"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// refreshTTL bounds both snapshot staleness and cross-instance propagation.
const refreshTTL = 30 * time.Second

// sweepAt is the cached-account count above which a read drops expired entries
// before inserting. The cache is keyed by account, so it grows with the number
// of distinct signed-in players an instance has served rather than with a fixed
// set of switches — it needs an eviction the site-wide settings cache does not.
const sweepAt = 2048

// Keys are the stored preference names. Values are "1"/"0"; an absent row means
// the default in flags below.
const (
	// KeyHomeAbout is whether the home page shows the "What is Octad?"
	// explainer and its demo board. On by default, and worth turning off once
	// you know what Octad is.
	KeyHomeAbout = "home.about"
)

// flags maps each boolean preference to the value a player gets when they have
// never chosen. It is also the accepted-key set: the write endpoint validates
// against this map rather than a second list that could drift from it.
var flags = map[string]bool{
	KeyHomeAbout: true,
}

// Valid reports whether key is a preference this site stores. Anything else is
// refused at the endpoint — a client must not be able to write arbitrary rows
// into an account's preference set.
func Valid(key string) bool {
	_, ok := flags[key]
	return ok
}

// Snapshot is one account's resolved preferences.
//
// The zero Snapshot is the default set, which is what an anonymous viewer gets
// and what a component test renders against without arranging anything. That
// property is why the overrides are held as a map read through accessors rather
// than as plain fields: a field would have to be named for its *non*-default
// state to keep the zero value honest, and the first preference whose default
// is "off" would break the pattern.
//
// The map is never written after the Snapshot is built, so copies of it are
// safe to share across goroutines.
type Snapshot struct {
	raw map[string]string
}

// Flag resolves one boolean preference, falling back to its built-in default.
func (s Snapshot) Flag(key string) bool {
	if v, ok := s.raw[key]; ok {
		return v == "1"
	}
	return flags[key]
}

// ShowHomeAbout reports whether the home page's "What is Octad?" card renders.
func (s Snapshot) ShowHomeAbout() bool { return s.Flag(KeyHomeAbout) }

// With returns a copy of s with one preference set. It is how a caller with no
// database — a component test arranging the Viewer it renders against — states
// what a player chose, without this package having to expose how the choice is
// stored. The receiver is not modified, so a cached Snapshot stays the one the
// account actually has.
func (s Snapshot) With(key string, on bool) Snapshot {
	raw := make(map[string]string, len(s.raw)+1)
	for k, v := range s.raw {
		raw[k] = v
	}
	if on {
		raw[key] = "1"
	} else {
		raw[key] = "0"
	}
	return Snapshot{raw: raw}
}

type entry struct {
	snap    Snapshot
	fetched time.Time
}

var cache = struct {
	sync.Mutex
	m map[int64]entry
}{m: make(map[int64]entry)}

// For returns one account's preferences, refreshing the cached snapshot once it
// has aged past refreshTTL. Safe for concurrent use and cheap enough for a
// render path — every signed-in page render reads it for the popover.
//
// Returns defaults for a zero id (no account) and when Postgres is unconfigured
// (PG-less local dev), which is the right answer either way: a site that cannot
// store a preference must render as though none was ever set.
// The query runs outside the lock. Two requests for the same cold account can
// therefore both read it, which costs one extra indexed lookup and settles on
// the same answer — cheaper than holding a process-wide mutex across a round
// trip while every other player's render waits behind it.
func For(userID int64) Snapshot {
	if userID == 0 {
		return Snapshot{}
	}
	if snap, ok := cached(userID); ok {
		return snap
	}
	raw, err := db.LoadUserPrefs(userID)
	if err != nil {
		util.Error(str.CDB, "user prefs load failed user=%d error=%s", userID, err.Error())
		// keep serving the last good snapshot (defaults, when there is none)
		// and retry after the TTL rather than querying once per render for the
		// length of a database outage
		return touch(userID)
	}
	snap := Snapshot{raw: raw}
	store(userID, snap)
	return snap
}

// cached returns the account's snapshot when one is present and still fresh.
func cached(userID int64) (Snapshot, bool) {
	cache.Lock()
	defer cache.Unlock()
	e, ok := cache.m[userID]
	return e.snap, ok && time.Since(e.fetched) < refreshTTL
}

// store caches a freshly read snapshot, sweeping expired entries first once the
// map has grown past sweepAt.
func store(userID int64, snap Snapshot) {
	cache.Lock()
	defer cache.Unlock()
	if len(cache.m) > sweepAt {
		sweepLocked()
	}
	cache.m[userID] = entry{snap: snap, fetched: time.Now()}
}

// touch restarts the TTL on whatever is cached for the account (the defaults,
// when nothing is) and returns it. The failure path.
func touch(userID int64) Snapshot {
	cache.Lock()
	defer cache.Unlock()
	e := cache.m[userID]
	e.fetched = time.Now()
	cache.m[userID] = e
	return e.snap
}

// SetFlag stores one boolean preference for one account and drops the cached
// snapshot so the next read reflects it.
//
// Setting a preference back to its built-in default deletes the row rather than
// writing the default value: that keeps "never chosen" and "chose what happens
// to be today's default" from drifting apart if a default later changes.
func SetFlag(userID int64, key string, on bool) error {
	if !Valid(key) {
		return errUnknownKey{key}
	}
	var err error
	if on == flags[key] {
		err = db.ClearUserPref(userID, key)
	} else if on {
		err = db.SetUserPref(userID, key, "1")
	} else {
		err = db.SetUserPref(userID, key, "0")
	}
	if err != nil {
		return err
	}
	Invalidate(userID)
	return nil
}

// Invalidate drops one account's cached snapshot so the next read re-queries.
func Invalidate(userID int64) {
	cache.Lock()
	delete(cache.m, userID)
	cache.Unlock()
}

// sweepLocked drops every expired entry. Called with the cache held, from the
// read path, only once the map has grown past sweepAt.
func sweepLocked() {
	for id, e := range cache.m {
		if time.Since(e.fetched) >= refreshTTL {
			delete(cache.m, id)
		}
	}
}

// errUnknownKey reports a write against a preference this site does not store.
type errUnknownKey struct{ key string }

func (e errUnknownKey) Error() string { return "prefs: unknown key " + e.key }
