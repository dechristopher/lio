package db

import (
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/rating"
	"github.com/dechristopher/lio/title"
)

// Home-page community reads: the newest accounts and the rating leaderboard.
//
// Both sit behind a TTL cache because of where they are read from. The home
// page's activity region re-renders every 5 seconds *per viewer*, so an
// uncached read here would multiply into a steady query stream that grows with
// traffic — to answer a question ("who joined recently?") whose answer changes
// a few times a day. The TTL is the staleness a visitor could notice, and a few
// seconds of it costs nothing: nobody is watching the leaderboard for
// sub-minute movement.
//
// Like the other archive accessors these degrade quietly: an unconfigured
// Postgres yields empty slices, so a PG-less local dev server simply renders no
// community panels rather than erroring the home page.

const (
	// communityTTL bounds how stale the cached panels may be.
	communityTTL = 30 * time.Second
	// newestShown / topShown bound each panel. Both are display caps, not
	// pagination: these are teasers that point at the player base, not a
	// directory.
	newestShown = 20
	topShown    = 5
	// leaderboardMinGames is the games floor for appearing on the leaderboard.
	// The RD filter already excludes provisional ratings; this additionally
	// keeps an account that got established off a very short run from topping
	// the board on the strength of a hot streak.
	leaderboardMinGames = 10
	// lastPlayedWindow bounds the recency map below. It is long enough that
	// everybody who plays with any regularity is in it, and short enough that
	// the map stays proportional to the active player base. Anybody older sorts
	// last within their roster tier, which is where an absent row lands anyway.
	lastPlayedWindow = 30 * 24 * time.Hour
)

// panelCache is one cached community panel. The zero value is an expired cache
// holding nothing, which is exactly the right starting state.
type panelCache[T any] struct {
	sync.Mutex
	rows    []T
	fetched time.Time
}

// get returns the cached rows, refreshing via load once they have aged past
// communityTTL. A failed refresh keeps serving the previous rows rather than
// blanking the panel: a database blip should not read as "nobody has ever
// signed up".
func (c *panelCache[T]) get(load func() ([]T, error)) []T {
	c.Lock()
	defer c.Unlock()
	if time.Since(c.fetched) < communityTTL {
		return c.rows
	}
	rows, err := load()
	if err != nil {
		return c.rows
	}
	c.rows = rows
	c.fetched = time.Now()
	return c.rows
}

// lookupCache is panelCache's map-shaped sibling, for a panel input that is
// read by key rather than rendered in order. Same TTL and the same "a failed
// refresh keeps the previous answer" rule; separate because building a map from
// a cached slice on every read would allocate once per digest tick, which is the
// cost the cache exists to remove.
//
// The map it hands back is shared and must be treated as read-only by callers.
type lookupCache[K comparable, V any] struct {
	sync.Mutex
	m       map[K]V
	fetched time.Time
}

func (c *lookupCache[K, V]) get(load func() (map[K]V, error)) map[K]V {
	c.Lock()
	defer c.Unlock()
	if time.Since(c.fetched) < communityTTL {
		return c.m
	}
	m, err := load()
	if err != nil {
		return c.m
	}
	c.m = m
	c.fetched = time.Now()
	return c.m
}

var (
	newestCache     panelCache[message.NewMember]
	topCache        panelCache[message.RatedMember]
	lastPlayedCache lookupCache[int64, time.Time]
)

// NewestMembers returns the most recently registered accounts, newest first.
func NewestMembers() []message.NewMember {
	if Pool == nil {
		return nil
	}
	return newestCache.get(func() ([]message.NewMember, error) {
		ctx, cancel := Ctx()
		defer cancel()

		rows, err := gen.New(Pool).ListNewestUsers(ctx, newestShown)
		if err != nil {
			return nil, err
		}
		out := make([]message.NewMember, 0, len(rows))
		for _, r := range rows {
			out = append(out, message.NewMember{
				ID:       r.ID,
				Username: r.Username,
				Title:    title.New(r.TitleCode, r.TitleName),
				Joined:   r.CreatedAt.Time,
			})
		}
		return out, nil
	})
}

// LastPlayed returns when each recently active account last finished a game,
// keyed by account id. It is the roster's ordering key: within a tier, whoever
// played most recently reads first (see presence.SortRoster).
//
// The returned map is shared with every other caller until the TTL expires —
// read it, never write it. An account with no entry has not played inside
// lastPlayedWindow and sorts last within its tier, which is also what an
// unconfigured Postgres produces for everybody: the roster then falls back to
// its alphabetical tiebreak rather than failing.
func LastPlayed() map[int64]time.Time {
	if Pool == nil {
		return nil
	}
	return lastPlayedCache.get(func() (map[int64]time.Time, error) {
		ctx, cancel := Ctx()
		defer cancel()

		since := pgtype.Timestamptz{Time: time.Now().Add(-lastPlayedWindow), Valid: true}
		rows, err := gen.New(Pool).ListLastPlayed(ctx, since)
		if err != nil {
			return nil, err
		}
		out := make(map[int64]time.Time, len(rows))
		for _, r := range rows {
			// a bot seat is NULL and cannot be ranked
			if r.UserID == nil || !r.PlayedAt.Valid {
				continue
			}
			out[*r.UserID] = r.PlayedAt.Time
		}
		return out, nil
	})
}

// TopRated returns the rating leaderboard: the highest established ratings,
// one row per account.
func TopRated() []message.RatedMember {
	if Pool == nil {
		return nil
	}
	return topCache.get(func() ([]message.RatedMember, error) {
		ctx, cancel := Ctx()
		defer cancel()

		rows, err := gen.New(Pool).ListTopRated(ctx, gen.ListTopRatedParams{
			Rd:    rating.ProvisionalRD,
			Games: leaderboardMinGames,
			Limit: topShown,
		})
		if err != nil {
			return nil, err
		}
		out := make([]message.RatedMember, 0, len(rows))
		for _, r := range rows {
			out = append(out, message.RatedMember{
				Username: r.Username,
				Title:    title.New(r.TitleCode, r.TitleName),
				Category: r.Category,
				Rating:   int(r.Rating + 0.5),
				Games:    int(r.Games),
			})
		}
		return out, nil
	})
}
