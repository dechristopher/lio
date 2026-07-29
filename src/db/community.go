package db

import (
	"sync"
	"time"

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
	newestShown = 5
	topShown    = 5
	// leaderboardMinGames is the games floor for appearing on the leaderboard.
	// The RD filter already excludes provisional ratings; this additionally
	// keeps an account that got established off a very short run from topping
	// the board on the strength of a hot streak.
	leaderboardMinGames = 5
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

var (
	newestCache panelCache[message.NewMember]
	topCache    panelCache[message.RatedMember]
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
				Username: r.Username,
				Title:    title.New(r.TitleCode, r.TitleName),
				Joined:   r.CreatedAt.Time,
			})
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
