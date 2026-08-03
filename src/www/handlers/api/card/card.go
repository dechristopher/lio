// Package card serves the username hover card (arch/PLAYER_CARD.md): the brief
// readout that appears over any username on the site, and the live mini board
// of the game that player is in.
//
// It is a strict subset of what /@/<username> already publishes, assembled for
// a glance rather than a visit — a few ratings, the overall record, whether the
// player is here, and the game they are playing. Nothing here is privileged:
// the card is public exactly as the profile page is, and it makes the same
// promise about a closed account, which is that it says nothing at all.
//
// The response carries a snapshot of the live game rather than only naming the
// room, so the card paints a complete board from this one request. Keeping it
// moving is the socket's job (proto.WatchTag) — the card asks its page's
// existing connection to watch the room it just learned about.
package card

import (
	"sort"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/home"
	"github.com/dechristopher/lio/presence"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/view"
	"github.com/dechristopher/lio/www/ws/proto"
)

// ratingsShown bounds the rating tiles on the card. The profile page lists
// every category a player has ever been rated in; a hover card has room for the
// ones they actually play, and three is where the row stops fitting beside the
// record.
const ratingsShown = 3

// Wire attaches the card endpoint to the given group.
func Wire(g fiber.Router) {
	g.Get("/:username", Handler)
}

// ratingRow is one rating tile: the time control it belongs to and the display
// rating, provisional marker included ("1658" / "1500?").
type ratingRow struct {
	Label  string `json:"label"`
	Speed  string `json:"speed,omitempty"`
	Rating string `json:"rating"`
	Games  int    `json:"games"`
}

// recordRow is the overall win/draw/loss tally across every game the account
// has played.
type recordRow struct {
	Games  int64 `json:"games"`
	Wins   int64 `json:"wins"`
	Draws  int64 `json:"draws"`
	Losses int64 `json:"losses"`
}

// response is the whole card. Everything a client would otherwise have to
// decide is resolved here — the status line's words, the relative dates — so
// the two surfaces that render a name (this card and the profile page it links
// to) cannot come to different conclusions about the same account.
type response struct {
	Username  string `json:"username"`
	Title     string `json:"title,omitempty"`
	TitleName string `json:"titleName,omitempty"`
	URL       string `json:"url"`
	// Closed marks a banned account. Everything below is omitted for one.
	Closed bool `json:"closed,omitempty"`
	// Status is the one-line state: playing, here, or the last time they were.
	Status  string `json:"status"`
	Online  bool   `json:"online,omitempty"`
	Playing bool   `json:"playing,omitempty"`
	Joined  string `json:"joined,omitempty"`

	Ratings []ratingRow `json:"ratings,omitempty"`
	Record  *recordRow  `json:"record,omitempty"`

	// RoomID and Game describe the live game, when there is one. Game is the
	// same struct the home grid streams, so one client component renders both.
	RoomID string        `json:"room,omitempty"`
	Game   *proto.TVGame `json:"game,omitempty"`
}

// Handler answers the hover card for one username.
//
// Every read degrades to absent rather than to an error, exactly as the profile
// page's do: a card missing its ratings is worth more than a card that failed.
// An unknown name is a 404, which the client renders as nothing at all — the
// username stays a plain link.
func Handler(c fiber.Ctx) error {
	username := strings.TrimSpace(c.Params("username"))
	if username == "" || !auth.Enabled() {
		return c.SendStatus(fiber.StatusNotFound)
	}

	rec, found, err := db.GetUserByUsername(username)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if !found {
		return c.SendStatus(fiber.StatusNotFound)
	}

	out := response{
		Username:  rec.Username,
		Title:     rec.Title.Code,
		TitleName: rec.Title.Name,
		URL:       "/@/" + rec.Username,
		Closed:    rec.Ban.Banned,
	}

	// A closed account publishes nothing, the same silence the profile page
	// keeps: no ratings, no record, no game. The card still resolves — the name
	// is real — it just makes no claim about the player behind it.
	if out.Closed {
		out.Status = "Account closed"
		return c.JSON(out)
	}

	out.Joined = view.JoinedMonth(rec.CreatedAt)
	out.Online = presence.AccountOnline(rec.ID)

	// The live game comes from the home hub, which already holds every live
	// room's display state — so the board the card opens with is the identical
	// struct the watch stream will keep updating.
	if game, ok := home.LiveGameFor(rec.Username); ok {
		out.Playing = true
		out.RoomID = game.RoomID
		out.Game = &game
	}

	out.Status = statusLine(out.Playing, out.Online, rec.ID, room.AccountBusy(rec.ID))
	fillRatings(&out, rec.ID)
	fillRecord(&out, rec.ID)

	return c.JSON(out)
}

// statusLine phrases what this player is doing, in the order a reader cares
// about: at a board, waiting at one, here, or last seen.
//
// "Offline" alone is a dead end — it answers the question with nothing — so an
// absent player is described by when they were last here, and failing that (the
// stamps are short-lived by design) simply as away.
func statusLine(playing, online bool, userID int64, busy bool) string {
	switch {
	case playing:
		return "Playing now"
	case online && busy:
		// holds a seat without a live game: sitting on a challenge of their own
		return "Waiting for a game"
	case online:
		return "Online"
	}
	if at, ok := presence.LastSeen(userID); ok {
		return "Last seen " + view.RelativeDay(at)
	}
	return "Offline"
}

// fillRatings adds the most-played rating categories. Selection is by games
// played — the card should lead with what somebody actually plays — but the
// tiles are then put back in canonical time-control order, so the row reads
// bullet-to-rapid rather than in popularity order, which would reshuffle
// itself as they played.
func fillRatings(out *response, userID int64) {
	ratings, err := db.ListRatingsForUser(userID)
	if err != nil || len(ratings) == 0 {
		return
	}
	views := make([]view.RatingView, 0, len(ratings))
	for _, r := range ratings {
		views = append(views, view.NewRatingView(
			r.Category, r.Rating.Display(), r.Rating.Games))
	}
	out.Ratings = pickRatings(views)
}

// pickRatings is the selection rule on its own: take the most-played
// categories, then put them back in canonical order. Split out from the read
// above so the rule can be tested without a database.
func pickRatings(views []view.RatingView) []ratingRow {
	sort.SliceStable(views, func(i, j int) bool {
		return views[i].Games > views[j].Games
	})
	if len(views) > ratingsShown {
		views = views[:ratingsShown]
	}
	view.SortRatings(views)
	rows := make([]ratingRow, 0, len(views))
	for _, v := range views {
		rows = append(rows, ratingRow{
			Label:  v.Label,
			Speed:  v.Speed,
			Rating: v.Rating,
			Games:  v.Games,
		})
	}
	return rows
}

// fillRecord adds the lifetime win/draw/loss tally. An account that has never
// finished a game gets none — three zeroes describe nothing, and the card says
// when they joined instead.
func fillRecord(out *response, userID int64) {
	total, _, err := db.TotalsForUser(userID)
	if err != nil || total.Games == 0 {
		return
	}
	out.Record = &recordRow{
		Games:  total.Games,
		Wins:   total.Wins,
		Draws:  total.Draws,
		Losses: total.Losses,
	}
}
