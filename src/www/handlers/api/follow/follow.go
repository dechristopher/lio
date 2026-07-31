// Package follow holds the follow graph's HTTP surface (arch/FOLLOWING.md):
// following an account, and unfollowing it.
//
// The writes are scoped to the session, which is the rule that makes this group
// safe without a role check: the follower is always the account that made the
// request, and no handler accepts a follower id from a client. What a caller
// names is the *other* end of the edge, by username — the thing they can
// actually see on the page they are looking at.
//
// The list reads beside them are public — anybody may see who follows whom, in
// the same posture the rest of the profile takes. They sit in this package
// because a public read of the graph is still the graph; what a viewer's
// identity adds there is per-row state, not access.
package follow

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/notify"
	"github.com/dechristopher/lio/presence"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
)

type errBody struct {
	Error string `json:"error"`
}

// stateResponse is what both writes answer with: the resulting state of the
// button that was pressed, and the target's follower count beside it.
//
// Both are returned so the control settles without a second request. The count
// is re-read rather than adjusted by one in the client: a follow that was
// already in place changes nothing, and a client that assumed otherwise would
// drift by one for as long as the page stayed open.
type stateResponse struct {
	Following bool  `json:"following"`
	Followers int64 `json:"followers"`
}

// Wire attaches the follow endpoints to the given group.
//
// Any static path segment added here must be registered *before* the
// ":username" routes it would otherwise be captured by — the same trap
// /@/<username> documents against the room wildcards in www.go. There is no
// bare GET /:username, so the two list sub-paths below are unambiguous even for
// an account whose name collides with a future static segment.
func Wire(g fiber.Router) {
	// static first: a GET on this path must reach MineHandler, not be captured
	// as a username. There is no bare GET /:username, so this is the only place
	// the two could collide.
	g.Get("/mine", MineHandler)
	g.Post("/:username", Handler)
	g.Delete("/:username", Handler)
	g.Get("/:username/followers", ListHandler)
	g.Get("/:username/following", ListHandler)
}

// mineLimit bounds the header popover's list. It is a menu of the people this
// viewer chose to watch, not a directory: somebody who follows four hundred
// accounts is served the first fifty and reads the rest on their profile.
const mineLimit = 50

// pageSize bounds one page of a follow list. The modal is a directory that
// people scroll, not an archive they page through deliberately, so this is
// sized to fill its scroll area rather than to be counted.
const pageSize = 25

// memberRow is one listed account, resolved for the viewer who asked.
//
// Following and Self are per-viewer, which is why these responses are
// no-store: the same list means something different to two people.
type memberRow struct {
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
	// Online marks an account holding a live socket right now. It comes from
	// the process, not the database — presence already knows.
	Online bool `json:"online,omitempty"`
	// Playing and Busy say what an online member is doing, and are set only by
	// the header popover's endpoint: they come from the room registry, which
	// the list endpoints have no reason to consult. Busy is the wider of the
	// two and is what decides whether a challenge can be offered.
	Playing bool `json:"playing,omitempty"`
	Busy    bool `json:"busy,omitempty"`
	// Following is the asking viewer's own edge to this row, which is what lets
	// the list offer a follow button per person.
	Following bool `json:"following,omitempty"`
	// Self marks the viewer's own row, which gets no button: the page refuses a
	// self-follow, and offering the control would be a promise it will not keep.
	Self bool `json:"self,omitempty"`
}

// listResponse is one page. More reports whether another page exists, resolved
// by reading one row past the page and never rendering it — the same trick the
// profile's recent-games list uses to know if its window is complete.
type listResponse struct {
	Members []memberRow `json:"members"`
	Page    int         `json:"page"`
	More    bool        `json:"more"`
}

// ListHandler serves one page of an account's followers or the accounts it
// follows. Which of the two is decided by the route, so the two lists cannot
// drift apart: they differ by one db call and nothing else.
//
// The lists are public — signed in or not, anybody may read them, which is the
// same posture the rest of the profile takes (arch/FOLLOWING.md). What the
// viewer's identity adds is per-row state, not access.
func ListHandler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(errBody{Error: "following is unavailable in this environment"})
	}

	target, found, err := db.GetUserByUsername(strings.TrimSpace(c.Params("username")))
	if err != nil {
		util.Error(str.CDB, "follow list lookup failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not read that list"})
	}
	if !found {
		return c.Status(fiber.StatusNotFound).JSON(errBody{Error: "no such account"})
	}
	// A closed account publishes nothing, the graph included. Its own edges are
	// untouched underneath — a ban is not a delete — but this page makes no
	// claim about them, exactly as the profile makes none about its record.
	if target.Ban.Banned {
		return c.JSON(listResponse{Members: []memberRow{}, Page: 1})
	}

	page := pageParam(c)
	// one past the page: enough to know whether another exists, never rendered
	limit, offset := int32(pageSize+1), int32((page-1)*pageSize)

	var members []db.FollowMember
	if strings.HasSuffix(c.Path(), "/followers") {
		members, err = db.ListFollowers(target.ID, limit, offset)
	} else {
		members, err = db.ListFollowing(target.ID, limit, offset)
	}
	if err != nil {
		util.Error(str.CDB, "follow list failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not read that list"})
	}

	more := len(members) > pageSize
	if more {
		members = members[:pageSize]
	}

	// Per-viewer state, so this must never sit in a shared cache.
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(listResponse{
		Members: rowsFor(user.GetAccount(c), members),
		Page:    page,
		More:    more,
	})
}

// MineHandler serves the header popover: the accounts this viewer follows,
// the ones who are here now first (arch/FOLLOWING.md Phase 5).
//
// Session-scoped like the writes — it answers about the caller and takes no
// account from them — so it needs no gate beyond being signed in.
//
// It lists offline follows too, muted, because the control is also the viewer's
// "who do I follow" entry point. That is why it is not named /online: the
// online ones lead, but they are not all of it.
func MineHandler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(errBody{Error: "following is unavailable in this environment"})
	}
	acct := user.GetAccount(c)
	if acct == nil {
		return c.Status(fiber.StatusUnauthorized).
			JSON(errBody{Error: "log in to see who you follow"})
	}

	members, err := db.ListFollowing(acct.ID, mineLimit, 0)
	if err != nil {
		util.Error(str.CDB, "following list failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not read your list"})
	}

	// The presence snapshot, with the room registry's seats overlaid, so a row
	// can say whether somebody is merely here or actually at a board. This is
	// the same pair of reads the home page's roster makes, and it is why the
	// popover can offer a challenge to exactly the people who could accept one.
	_, _, _, seated := room.HomeListing()
	online := presence.Online(seated, 0).Accounts

	rows := make([]memberRow, 0, len(members))
	for _, m := range members {
		row := memberRow{Name: m.Username, Title: m.Title.Code, Following: true}
		if p, here := online[m.ID]; here {
			row.Online, row.Playing, row.Busy = true, p.Playing, p.Busy
		}
		rows = append(rows, row)
	}
	// Available first, then the rest of the online ones, then everybody who is
	// away — the order the question "who can I play right now" is asked in.
	// Ties break alphabetically so reopening the panel does not reshuffle it.
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Online != b.Online {
			return a.Online
		}
		if a.Online && a.Busy != b.Busy {
			return !a.Busy
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(listResponse{Members: rows, Page: 1})
}

// notifyFollowed tells an account it has a new follower (arch/FOLLOWING.md
// Phase 3). Called only for a follow that actually created an edge, so
// re-pressing a button nobody moved announces nothing.
//
// Nothing is pushed on an unfollow. Being unfollowed is not news anybody needs,
// and a site that reported it would be a site people are careful on.
//
// A failure is logged and swallowed, like every other producer here: the follow
// already happened, and reporting failure would invite the caller to repeat a
// write that succeeded.
func notifyFollowed(targetID int64, follower *user.Account) {
	// A second announcement about the same person inside a day is suppressed;
	// see db.RecentFollowNotice for the loop it exists to stop.
	if db.RecentFollowNotice(targetID, follower.ID) {
		return
	}
	if err := notify.Push(db.NewNotification{
		UserID:  targetID,
		ActorID: &follower.ID,
		Kind:    db.KindFollow,
		// The follower is named by actor_id, not by this sentence. The panel
		// resolves that id to a *current* username on every read, so a rename
		// does not leave an old name sitting in somebody's list — which is the
		// rule 00021 states and the reason this copy has no subject in it.
		Body: "You have a new follower",
		// The link is a copy of the name, and the one part of this a rename can
		// make stale. That is the accepted trade: a notification worth clicking
		// beats one that is always correct and goes nowhere, and the name shown
		// beside it is resolved live either way.
		Link: "/@/" + follower.Username,
	}, follower.Username); err != nil {
		util.Error(str.CNotif, "follow notify failed target=%d error=%s",
			targetID, err.Error())
	}
}

// rowsFor renders one page for the viewer who asked, resolving the two facts
// the database rows do not carry: who is online, and which of them this viewer
// already follows.
//
// Both are one lookup for the whole page rather than one per row. Presence is a
// process-local set, and the follow states come back from a single indexed
// probe bounded by the page size (db.FollowedAmong).
func rowsFor(viewer *user.Account, members []db.FollowMember) []memberRow {
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.ID)
	}

	online := make(map[int64]struct{})
	for _, id := range presence.OnlineIDs() {
		online[id] = struct{}{}
	}

	// An anonymous viewer follows nobody, and FollowedAmong answers that without
	// a query — so a signed-out reader costs this endpoint nothing extra.
	var viewerID int64
	if viewer != nil {
		viewerID = viewer.ID
	}
	followed, err := db.FollowedAmong(viewerID, ids)
	if err != nil {
		// The list is still worth serving without the per-row buttons; a reader
		// came here to see who these people are.
		util.Error(str.CDB, "follow state failed error=%s", err.Error())
	}

	rows := make([]memberRow, 0, len(members))
	for _, m := range members {
		_, isOnline := online[m.ID]
		_, isFollowed := followed[m.ID]
		rows = append(rows, memberRow{
			Name:      m.Username,
			Title:     m.Title.Code,
			Online:    isOnline,
			Following: isFollowed,
			Self:      viewerID != 0 && m.ID == viewerID,
		})
	}
	return rows
}

// pageParam reads the 1-based page number, clamping anything unreadable to the
// first page. A bad page is not worth an error: the caller gets the list they
// would have got by not asking.
func pageParam(c fiber.Ctx) int {
	page, err := strconv.Atoi(c.Query("page"))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

// Handler follows or unfollows an account, by HTTP method: POST creates the
// edge, DELETE removes it. One handler rather than two, because everything
// except the single db call — the gate, the lookups and every refusal — is
// shared, and two copies of that list is two places to forget the same check.
//
// Both directions are idempotent. Following somebody you already follow, and
// unfollowing somebody you never followed, both answer with the state the
// caller asked for rather than an error: the caller wanted a state, and that
// state holds.
//
// None of the refusals here is the only line of defence. The CHECK constraint
// refuses a self-follow, the foreign keys refuse an unknown account, and the
// cap lives in db.Follow so that every caller inherits it.
func Handler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(errBody{Error: "following is unavailable in this environment"})
	}
	acct := user.GetAccount(c)
	if acct == nil {
		// Following requires an account: the edge has to start somewhere, and an
		// anonymous session is not a somewhere that survives a browser restart.
		return c.Status(fiber.StatusUnauthorized).
			JSON(errBody{Error: "log in to follow players"})
	}

	target, found, err := db.GetUserByUsername(strings.TrimSpace(c.Params("username")))
	if err != nil {
		util.Error(str.CDB, "follow lookup failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not reach that account"})
	}
	if !found {
		return c.Status(fiber.StatusNotFound).JSON(errBody{Error: "no such account"})
	}
	if target.ID == acct.ID {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "you cannot follow yourself"})
	}

	// A closed account publishes nothing, so there is nothing to follow. Unfollow
	// is deliberately still allowed: somebody who followed an account that was
	// later banned must be able to undo that, and refusing would strand the row.
	if target.Ban.Banned && c.Method() == fiber.MethodPost {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "that account is closed"})
	}

	// One account's own social graph, and it changes on this request. It must
	// never be served from a shared cache or replayed from the browser's.
	c.Set(fiber.HeaderCacheControl, "no-store")

	following := c.Method() == fiber.MethodPost
	var created bool
	if following {
		created, err = db.Follow(acct.ID, target.ID)
	} else {
		_, err = db.Unfollow(acct.ID, target.ID)
	}
	switch {
	case errors.Is(err, db.ErrFollowLimit):
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "you are following as many players as one account can"})
	case err != nil:
		util.Error(str.CDB, "follow write failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not save that"})
	}

	if created {
		notifyFollowed(target.ID, acct)
	}

	// The count is read after the write, from the same place the profile page
	// reads it, so the number the button lands on is the number a reload shows.
	counts, err := db.FollowCountsForUser(target.ID)
	if err != nil {
		// The write is done and the button's own state is known. Answer with it
		// and let the count correct itself on the next render rather than report
		// a failure for something that succeeded.
		util.Error(str.CDB, "follow count failed error=%s", err.Error())
		return c.JSON(stateResponse{Following: following})
	}
	return c.JSON(stateResponse{Following: following, Followers: counts.Followers})
}
