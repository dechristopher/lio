package db

import (
	"errors"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/title"
)

// The follow graph (arch/FOLLOWING.md): one directed edge per row, no consent
// step, no state beyond the presence of the row.
//
// Like the other social accessors these degrade quietly. An unconfigured
// Postgres reports "nobody follows anybody", so a PG-less local dev server
// simply renders no follow controls rather than erroring every player page.
//
// Nothing here notifies. A new follower is worth telling somebody about, but
// notify imports db, so the announcement belongs to the caller that made the
// follow (www/handlers/api/follow) — the same shape every other notification
// producer has.

// MaxFollowing caps how many accounts one account may follow.
//
// This is an abuse bound, not a product limit. It bounds one account's
// contribution to the table and, with it, the cost of every query that scans
// one person's edges. Nobody reaches a thousand by using the site.
const MaxFollowing = 1000

// ErrFollowLimit is returned by Follow when the follower is already at
// MaxFollowing. It is a distinct error because the caller has something
// specific to say about it — a generic failure would read as a bug.
var ErrFollowLimit = errors.New("follow limit reached")

// FollowCounts is the pair of numbers a player page shows: how many accounts
// follow this one, and how many it follows.
type FollowCounts struct {
	Followers int64
	Following int64
}

// Follow records that followerID follows followeeID, and reports whether that
// created a new edge.
//
// created is false when the edge already existed, which is not an error: the
// caller asked for a state and that state now holds. It matters because it is
// what keeps a follow/unfollow/refollow loop from generating notifications —
// only a genuinely new follower is announced.
//
// The cap is enforced here rather than in the handler so every caller inherits
// it. The check is not atomic with the insert, so two concurrent follows from
// one account can both pass it and land at MaxFollowing+1. That is acceptable:
// this bounds storage against a script, and a cap overshot by one row under a
// deliberate race has not failed at that.
func Follow(followerID, followeeID int64) (created bool, err error) {
	if Pool == nil {
		return false, nil
	}
	// A self-follow would be refused by the CHECK constraint. Refuse it here as
	// well, so it reads as the nonsense it is rather than as a storage failure.
	if followerID == followeeID || followerID == 0 || followeeID == 0 {
		return false, nil
	}
	ctx, cancel := Ctx()
	defer cancel()

	q := gen.New(Pool)
	n, err := q.CountFollowing(ctx, followerID)
	if err != nil {
		return false, err
	}
	if n >= MaxFollowing {
		return false, ErrFollowLimit
	}

	rows, err := q.Follow(ctx, gen.FollowParams{
		FollowerID: followerID,
		FolloweeID: followeeID,
	})
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// Unfollow removes the edge, and reports whether there was one to remove.
func Unfollow(followerID, followeeID int64) (removed bool, err error) {
	if Pool == nil {
		return false, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).Unfollow(ctx, gen.UnfollowParams{
		FollowerID: followerID,
		FolloweeID: followeeID,
	})
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// IsFollowing reports whether followerID follows followeeID.
//
// It swallows its error, like UnreadNotifications: this decides which label a
// button carries, and a failed read should render the button in its default
// state rather than fail the page around it. The write path re-reads the truth
// anyway — Follow and Unfollow are both idempotent, so a button that started
// with the wrong label still reaches the right state.
func IsFollowing(followerID, followeeID int64) bool {
	if Pool == nil || followerID == 0 || followeeID == 0 || followerID == followeeID {
		return false
	}
	ctx, cancel := Ctx()
	defer cancel()
	following, err := gen.New(Pool).IsFollowing(ctx, gen.IsFollowingParams{
		FollowerID: followerID,
		FolloweeID: followeeID,
	})
	if err != nil {
		return false
	}
	return following
}

// FollowSummary is the header control's state: how many accounts the viewer
// follows, and how many of those are online right now.
type FollowSummary struct {
	Following int64
	Online    int64
}

// FollowSummaryFor answers both of the header's questions in one read, given
// the account ids currently connected (presence.OnlineIDs).
//
// This runs on every render of every signed-in page, which is the same budget
// UnreadNotifications already spends there. It swallows its error for the same
// reason: this decides whether a header control renders, and a failed read
// should leave the header as it was rather than fail the page around it.
func FollowSummaryFor(userID int64, onlineIDs []int64) FollowSummary {
	if Pool == nil || userID == 0 {
		return FollowSummary{}
	}
	ctx, cancel := Ctx()
	defer cancel()
	// A nil array is not the same as an empty one to ANY(), and pgx encodes nil
	// as NULL — against which every comparison is NULL rather than false. Send
	// an empty array so "nobody is online" counts zero instead of erroring.
	if onlineIDs == nil {
		onlineIDs = []int64{}
	}
	row, err := gen.New(Pool).FollowSummary(ctx, gen.FollowSummaryParams{
		FollowerID: userID, Ids: onlineIDs,
	})
	if err != nil {
		return FollowSummary{}
	}
	return FollowSummary{Following: row.Following, Online: row.Online}
}

// FollowMember is one row of a follow list: an account, named. Deliberately
// thin — no rating, no record. The list is a directory of people, and the name
// links to the profile that holds the numbers; resolving a rating per row would
// mean the DISTINCT ON sub-select ListTopRated uses, twenty-five times over.
type FollowMember struct {
	ID       int64
	Username string
	Title    title.Title
}

// ListFollowing returns one page of the accounts userID follows, newest follow
// first. ListFollowers is its mirror.
func ListFollowing(userID int64, limit, offset int32) ([]FollowMember, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListFollowing(ctx, gen.ListFollowingParams{
		FollowerID: userID, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]FollowMember, 0, len(rows))
	for _, r := range rows {
		out = append(out, FollowMember{
			ID: r.ID, Username: r.Username, Title: title.New(r.TitleCode, r.TitleName),
		})
	}
	return out, nil
}

// ListFollowers returns one page of the accounts that follow userID, newest
// follow first.
func ListFollowers(userID int64, limit, offset int32) ([]FollowMember, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListFollowers(ctx, gen.ListFollowersParams{
		FolloweeID: userID, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]FollowMember, 0, len(rows))
	for _, r := range rows {
		out = append(out, FollowMember{
			ID: r.ID, Username: r.Username, Title: title.New(r.TitleCode, r.TitleName),
		})
	}
	return out, nil
}

// FollowedAmong returns which of ids the given account follows, as a set.
//
// This is the feature's load-bearing read (arch/FOLLOWING.md). Its cost is
// bounded by len(ids) rather than by how many accounts followerID follows, so
// asking "which of these 25 rows do I follow" and "which of the 200 people
// online do I follow" both cost the same small index probe.
//
// A zero follower (an anonymous viewer) or an empty id set returns an empty
// set without a query: there is nothing an anonymous session can follow.
func FollowedAmong(followerID int64, ids []int64) (map[int64]struct{}, error) {
	out := make(map[int64]struct{})
	if Pool == nil || followerID == 0 || len(ids) == 0 {
		return out, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).FollowedAmong(ctx, gen.FollowedAmongParams{
		FollowerID: followerID, Ids: ids,
	})
	if err != nil {
		return out, err
	}
	for _, id := range rows {
		out[id] = struct{}{}
	}
	return out, nil
}

// FollowingIDs returns the whole set of account ids userID follows.
//
// It is the connect-time half of the home page's Following section
// (arch/HOME_ACTIVITY_STREAMING.md). The htmx poll it replaced ran
// FollowedAmong once per viewer per five seconds; this runs once per socket, and
// the hub then answers the same question with map lookups against the set it
// returns.
//
// Reading the whole list is the right shape *here* and the wrong shape in the
// poll it replaced. FollowedAmong is bounded by how many people are online,
// which is what you want when the question is asked constantly. This is asked
// once per connection, and what it needs is the durable half of the
// intersection — the half that does not change while the socket is open.
//
// MaxFollowing bounds the result, so an account cannot make its own connect
// expensive. A zero user (an anonymous session) reads nothing.
func FollowingIDs(userID int64) (map[int64]struct{}, error) {
	out := make(map[int64]struct{})
	if Pool == nil || userID == 0 {
		return out, nil
	}
	rows, err := ListFollowing(userID, MaxFollowing, 0)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		out[r.ID] = struct{}{}
	}
	return out, nil
}

// FollowCountsForUser returns the two numbers on a player page. Banned accounts
// are excluded from both, by the same rule that excludes them from the
// home-page panels — and from both together, so the count a profile prints
// always matches the list it opens.
func FollowCountsForUser(userID int64) (FollowCounts, error) {
	if Pool == nil {
		return FollowCounts{}, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).FollowCounts(ctx, userID)
	if err != nil {
		return FollowCounts{}, err
	}
	return FollowCounts{Followers: row.Followers, Following: row.Following}, nil
}
