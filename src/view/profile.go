package view

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/pools"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/title"
)

// The public player page (arch/ADMIN_MODERATION.md): the site's first surface
// that links a name to a history. Everything a moderator can do to an account
// hangs off this same page as a bar only moderators see, so there is no
// separate per-player console to keep in sync with it.
//
// The model is fully resolved by the handler — no component reaches for the DB
// — matching how ArchiveModel feeds the archive page.

// ProfileModel is everything the player page renders.
type ProfileModel struct {
	// UserID is the account's row id, the target of every mod action on the
	// page. Rendered only into the mod bar (which is itself mod-gated).
	UserID   int64
	Username string
	Title    title.Title
	Joined   string // "March 2026"
	// RenderedAt is when the server built this page, as epoch milliseconds. The
	// page is a snapshot of a history that keeps moving, so it reports its own
	// age rather than letting a tab left open all afternoon look current.
	RenderedAt string

	// Closed marks a banned account. The public page then shows a neutral
	// closed card in place of the stats and suppresses the ratings — no reason,
	// no duration, which are moderator-only (and are told to the account holder
	// at the login refusal instead).
	Closed bool

	// Online marks an account holding a live socket right now, and Busy one
	// already seated in a room — playing, or waiting in a challenge of its own.
	// Between them they decide whether the Challenge button renders, which is
	// the same rule the home roster's sword follows (arch/NOTIFICATIONS.md
	// Phase 2).
	//
	// Online matters most here. A profile is the one challenge surface a visitor
	// reaches without any presence context at all — from a game, a leaderboard,
	// a search — so before this the button was routinely offered against
	// somebody who had not been seen in weeks.
	Online bool
	Busy   bool

	// Ratings, records and games are omitted entirely for a closed account.
	Ratings  []RatingView
	Total    RecordView
	Lifetime LifetimeView
	Variants []VariantRecordView
	Bots     []BotRecordView
	Games    []ProfileGameView

	// Charts are the per-category rating curves. HasCharts reports whether any
	// of them is actually plottable, which is what decides if the rating tiles
	// render as curve selectors or as plain tiles — a tile that switches to
	// nothing is worse than a tile that does not invite a click.
	Charts    []RatingChartView
	HasCharts bool

	// Core record analytics (Phase 3). Form is derived from Games rather than
	// queried, so it is always consistent with the list rendered below it.
	Colors  []ColorSplitView
	Endings []EndingView
	Lengths LengthsView
	// Formations is the octad-specific section (Phase 4): deploy choices, what
	// they face, and per-matchup performance.
	Formations FormationsView

	// Activity & social (Phase 5). BotLadder is derived from Bots rather than
	// queried, so it can render rungs the account has never played.
	Activity  ActivityView
	Opponents []OpponentView
	BotLadder []BotRungView
	Form      []FormGroup
	Streaks   StreakView

	// H2H is the viewer's own record against this account, shown only when the
	// viewer is logged in, is not this account, and the two have actually met.
	H2H     string
	H2HShow bool

	// Follow is the social block: the two public counts, and the follow control
	// for a viewer who may press it (arch/FOLLOWING.md).
	Follow FollowView

	// ShowReport offers the report control (arch/ADMIN_MODERATION.md Phase 4)
	// to a logged-in visitor looking at somebody else's open account. Mutually
	// exclusive with ShowMod: a moderator who can act on this account has the
	// tools right there, and asking themselves to look is not a workflow.
	ShowReport bool

	// Mod is the moderation bar's state, populated only when the viewing
	// account may moderate this one. Rendering is gated on ShowMod; every
	// action it offers is independently re-authorized server-side.
	ShowMod bool
	Mod     ModBarView
}

// RatingView is one time control's rating on the profile.
type RatingView struct {
	// Category is the raw Glicko-2 key (a variant HTMLName, e.g.
	// "one-two-rapid-deploy"). It is the tile's identity — later phases key the
	// rating curve off it — and is never displayed.
	Category string
	// Label / Speed / Mode are the display resolution of Category through
	// pools.LookupRatingCategory: "1 + 2", "rapid", and "" for the default
	// deploy mode (surfaced as Octad) or e.g. "Classic".
	Label string
	Speed string
	Mode  string
	// order is the canonical time-control ordering (bullet < blitz < 1+2 < 3+5)
	// from the same lookup, so tiles are not at the mercy of the SQL's
	// alphabetical category sort.
	order  int
	known  bool
	Rating string // "1653" / "1500?"
	Games  int
}

// NewRatingView resolves a raw rating category into a displayable tile.
// pools.LookupRatingCategory is the single source of that mapping — it already
// backs the account popover, and the profile rendering the raw HTMLName
// ("one-two-rapid-deploy") was simply a place that never adopted it.
//
// An unknown category (a legacy row whose variant is no longer curated) keeps
// its raw key as the label and sorts last, rather than vanishing: an earned
// rating should still be visible after its pool is retired.
func NewRatingView(category, display string, games int) RatingView {
	v := RatingView{Category: category, Rating: display, Games: games}
	info, ok := pools.LookupRatingCategory(category)
	if !ok {
		v.Label, v.order = category, math.MaxInt
		return v
	}
	v.Label, v.Speed, v.Mode, v.order, v.known = info.TimeControl, info.Speed, info.Mode, info.Order, true
	return v
}

// SortRatings puts rating tiles in canonical time-control order, unknown
// categories last, ties broken by category so the order is stable.
func SortRatings(rs []RatingView) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].order != rs[j].order {
			return rs[i].order < rs[j].order
		}
		return rs[i].Category < rs[j].Category
	})
}

// LifetimeView is the hero card's pair of figures: how many games, and how long
// has been spent at the board. Values and labels are separate so each renders as
// a figure tile rather than a sentence.
//
// Show is false for an account with no games, which then gets no figures at all
// rather than two zeroes — "0 games / 0m played" is a worse greeting for a new
// player than silence.
type LifetimeView struct {
	Games  string // "1,204"
	Played string // "31h" / "2d 3h" / "45m"
	Show   bool
}

// NewLifetimeView builds the hero figures. Durations stay coarse — matching the
// page's posture that a public profile publishes rounded facts, not exact
// timestamps — and compact, since these are figures rather than prose.
func NewLifetimeView(r db.Record, l db.Lifetime) LifetimeView {
	if r.Games == 0 {
		return LifetimeView{}
	}
	return LifetimeView{
		Games:  commas(r.Games),
		Played: compactPlayed(l.Played),
		Show:   true,
	}
}

// compactPlayed renders time at the board as a figure: "45m", "3h", "2d 3h".
// Below a minute it is "—": an account can have finished games whose durations
// round to nothing, and a blank tile beside a filled one reads as broken.
//
// Note this is wall-clock game duration summed across the account's games, so it
// counts both players' thinking and any idle time in an untimed game — time
// spent at the board, not time this account's clock ran.
func compactPlayed(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		days := int64(d / (24 * time.Hour))
		hours := int64((d % (24 * time.Hour)) / time.Hour)
		out := strconv.FormatInt(days, 10) + "d"
		if hours > 0 {
			out += " " + strconv.FormatInt(hours, 10) + "h"
		}
		return out
	case d >= time.Hour:
		return strconv.FormatInt(int64(d.Hours()), 10) + "h"
	case d >= time.Minute:
		return strconv.FormatInt(int64(d.Minutes()), 10) + "m"
	}
	return "—"
}

// plural renders a count with a thousands separator and the right noun.
func plural(n int64, one, many string) string {
	noun := many
	if n == 1 {
		noun = one
	}
	return commas(n) + " " + noun
}

// commas groups an integer with thousands separators ("1,204"). Counts on this
// page can reach five figures for an active account, where a bare run of digits
// is measurably harder to read at a glance.
func commas(n int64) string {
	s := strconv.FormatInt(n, 10)
	if n < 0 || len(s) <= 3 {
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

// FollowView is the profile's social block (arch/FOLLOWING.md): who follows
// this account, who it follows, and — for a viewer who may act — the control
// that changes the first of those.
type FollowView struct {
	// Followers / Following are finished phrases ("128 followers",
	// "34 following"), not bare numbers: they read as a sentence under the
	// name, where two unlabelled figures beside the hero's games/played tiles
	// would be four numbers competing to be understood.
	Followers string
	Following string
	// HasFollowers / HasFollowing report whether each count has a list behind
	// it. A count of zero renders its button *disabled* rather than as plain
	// text: the control keeps its place and its shape, so a first follower does
	// not make the line change form under the person who just arrived. It is
	// the same fade-in-place rule the create-game dialog follows.
	HasFollowers bool
	HasFollowing bool
	// Show renders the counts line for a visitor who cannot act on it. False
	// when nobody follows this account and it follows nobody — the same rule
	// LifetimeView follows, for the same reason: "0 followers · 0 following" is
	// a worse greeting for a new player than silence.
	//
	// The line renders regardless when Control does. A viewer holding the button
	// is owed the number it changes, including while that number is zero.
	Show bool
	// Control offers the Follow button: a logged-in visitor looking at somebody
	// else's open account.
	//
	// Unlike the challenge sword this is not withheld from a busy player.
	// Following somebody who is mid-game is exactly when a person would want to,
	// and unlike a challenge it asks nothing of them.
	Control bool
	// IsFollowing is the button's initial state. It is only the initial one:
	// both writes are idempotent, so a button that rendered from a stale read
	// still reaches the state its label promises.
	IsFollowing bool
}

// followingOnlineLabel is the accessible name on the header control's badge. It
// states the count, like the bell's: a bare dot says somebody is here without
// saying whether that is one person or nine, and the badge is the only place a
// screen reader can learn either.
func followingOnlineLabel(n int64) string {
	if n == 1 {
		return "1 player you follow is online"
	}
	return strconv.FormatInt(n, 10) + " players you follow are online"
}

// NewFollowView renders the counts. The zero FollowCounts yields a block that
// shows nothing, which is what an account nobody has met yet should render.
func NewFollowView(c db.FollowCounts) FollowView {
	return FollowView{
		Followers: plural(c.Followers, "follower", "followers"),
		// "following" does not inflect, so it is not run through plural: "1
		// following" is correct and "1 followings" is not a word.
		Following:    commas(c.Following) + " following",
		HasFollowers: c.Followers > 0,
		HasFollowing: c.Following > 0,
		Show:         c.Followers > 0 || c.Following > 0,
	}
}

// RecordView is a win/draw/loss tally rendered as strings.
type RecordView struct {
	Games  string
	Wins   string
	Draws  string
	Losses string
	// Score reads "12½" — the same half-point notation the match scoreboard
	// uses, so a player sees one vocabulary across the site.
	Score string
	// Empty marks a tally with no games behind it, so the section can show a
	// placeholder instead of a row of zeroes. Carried as a field rather than
	// re-derived by string-comparing Games against "0" in the template.
	Empty bool
	// Bar is the same tally as a proportional mark. Every record row draws one,
	// which is what lets the Record card fill a wide card with something worth
	// reading rather than stranding a label a screen from its numbers.
	Bar WDLBar
	// Rate is the average scoring rate ("0.65") with its tint — the same figure
	// and the same vocabulary the social sections use, so one page does not
	// report the same quantity two different ways.
	Rate      string
	RateClass string
}

// VariantRecordView is a RecordView for one time control.
type VariantRecordView struct {
	Name  string
	Group string
	RecordView
}

// BotRecordView is a RecordView against one bot persona, already resolved to
// its display label and piece glyph.
type BotRecordView struct {
	Persona string
	Glyph   string
	RecordView
	// Bar carries the raw counts the string fields have already formatted away,
	// which the ladder needs to draw a proportional mark and to ask "has this
	// rung ever been beaten".
	Bar WDLBar
}

// ProfileGameView is one row of the recent-games list.
type ProfileGameView struct {
	// RoomID groups games into matches for the recent-form strip. Empty for a
	// room-less backfilled game, which then stands alone.
	RoomID  string
	URL     string // archive permalink
	When    string // "2 days ago"
	Variant string // "½ + 1 blitz"
	Mode    string // "Rated" / "Casual"
	Result  string // "Won" / "Lost" / "Drew"
	// Reason is the DB-canonical method token ("checkmate", "time", …), rendered
	// into the readout as the phrase that follows the result.
	Reason   string
	Class    string // result-tinting class: win / loss / draw
	Opponent string // "cdpplayer" / "BOT Queen" / "Anonymous"
	// OppRating is the opponent's rating going into the game, shown only when
	// the game was rated — an unrated game's ratings say nothing about it.
	OppRating string
	// Ending phrases how the game finished ("by checkmate") and Delta the
	// rating change with its tint. Each is empty when the archive has nothing
	// to say, so a row never carries a placeholder.
	//
	// The game's length in plies used to sit here too. It was dropped: a row
	// already carries the result, the method, the opponent, the rating change
	// and the date, and "23 plies" is the one of those that tells a reader
	// nothing they would act on.
	Ending     string
	Delta      string
	DeltaClass string
	// OppGlyph is the bot persona's piece glyph, kept out of Opponent so the
	// template can wrap it: these glyphs come from a fallback font whose
	// baseline sits low, and correcting that needs an element to hang the rule
	// on. Empty for human and anonymous seats.
	OppGlyph string
	OppTitle string // opponent's title code, "" when untitled
}

// ModBarView is the moderator-only control state for this account.
type ModBarView struct {
	// CanSetRole gates the role control on the viewer being an admin: mods
	// appoint nobody. Also false on your own page — no self-demotion.
	CanSetRole bool
	// CanBan hides the ban control on your own account. An admin may
	// administer themselves (title, rename) but not lock themselves out.
	CanBan bool
	// IsSelf marks the viewer looking at their own page, so the bar can say so
	// rather than silently offering a shorter set of controls.
	IsSelf bool
	// Username is the account being acted on, named in the confirmation so a
	// moderator with several tabs open cannot sanction the wrong person.
	Username string
	// CurrentRole / CurrentTitleID prime the pickers with what is set now.
	CurrentRole    string
	CurrentTitleID string
	// Titles are every assignable title, for the title picker.
	Titles []TitleOptionView
	// Roles are the assignable roles (empty unless CanSetRole).
	Roles []string
	// Banned mirrors ProfileModel.Closed but drives the ban/unban control;
	// BanReason and BanUntil are the moderator-only detail.
	Banned    bool
	BanReason string
	BanUntil  string
	// Actions is this account's moderation history, newest first, capped at
	// what fits on a player page. Shares the /system feed's entry type so both
	// views of the log render through one component.
	Actions []ModFeedEntry
	// OpenReports is how many unresolved reports name this account, so a
	// moderator sees the context before acting rather than after.
	OpenReports int64
	// ActionsTotal is how many entries exist in total; when it exceeds the
	// listed ones, HistoryURL leads to the full, paginated record on /system
	// rather than this page growing a second pager of its own.
	ActionsTotal int64
	HistoryURL   string
}

// TitleOptionView is one entry of the mod bar's title picker.
type TitleOptionView struct {
	ID   string
	Code string
	Name string
}

// NewRecordView renders a db.Record for display.
func NewRecordView(r db.Record) RecordView {
	return RecordView{
		Games:  strconv.FormatInt(r.Games, 10),
		Wins:   strconv.FormatInt(r.Wins, 10),
		Draws:  strconv.FormatInt(r.Draws, 10),
		Losses: strconv.FormatInt(r.Losses, 10),
		Score:  FormatPoints(r.Points()),
		Empty:  r.Games == 0,
		Bar:    NewWDLBar(r),

		Rate:      ScoreRate(r),
		RateClass: RateClass(r),
	}
}

// FormatPoints renders a half-point score the way the scoreboard does: "12",
// "12½", "0".
func FormatPoints(p float64) string {
	whole := int(p)
	if p-float64(whole) >= 0.5 {
		if whole == 0 {
			return "½"
		}
		return strconv.Itoa(whole) + "½"
	}
	return strconv.Itoa(whole)
}

// ProfileMeta builds page metadata for a player page. A closed account gets the
// same neutral treatment in the title as on the page itself.
func ProfileMeta(m ProfileModel) Meta {
	// The title is bracketed here but not on the page itself. On the page it is
	// a tinted badge, visibly a separate element from the name; in a browser tab,
	// a search result or a shared link it is bare text, where "GM drewtest" reads
	// as one two-word name. Brackets restore the separation that the styling
	// carries everywhere else.
	who := m.Username
	if m.Title.Set() {
		who = "[" + m.Title.Code + "] " + m.Username
	}
	desc := who + " on " + config.SiteName() + " — octad games, ratings and record."
	if m.Closed {
		desc = "This " + config.SiteName() + " account is closed."
	}
	// the headline rating rides along in the bare-text contexts, where there is
	// room for one number and no page to read it off
	headline := who
	if r := HeadlineRating(m.Ratings); r != "" {
		headline += " · " + r
	}
	return Meta{
		Version:     config.VersionString(),
		SiteURL:     config.SiteURL(),
		Title:       headline + " • " + config.SiteName(),
		OGURL:       config.SiteOrigin() + "/@/" + m.Username,
		OGTitle:     headline,
		OGImage:     config.SiteOrigin() + "/og/default.png",
		Description: desc,
	}
}

// HeadlineRating picks the one rating worth putting in a page title, as
// "1712 rapid". Empty when the account has nothing worth quoting.
//
// The rule is **most games played, among settled ratings**. The two obvious
// alternatives are both worse:
//
//   - An average across categories is not a number. Glicko ratings come from
//     separate pools with their own populations and deviations; the mean of a
//     bullet and a rapid rating describes nobody.
//   - The highest rating rewards the fluke — four provisional games at 1800 in a
//     pool they never play would outrank four hundred settled games at 1500, and
//     it is also the number most likely to move next week.
//
// Most-played is the rating with the most evidence behind it (most games means
// the lowest deviation) and the one a player would name if asked. Provisional
// ratings are skipped entirely rather than quoted with their "?": a title is
// read without context, and an unsettled rating asserted there is a claim the
// account has not earned. An account whose ratings are all provisional gets no
// number, which is the honest outcome.
//
// The speed group names the category rather than the exact time control: "1712
// rapid" is what a reader parses at a glance, and the precise control is on the
// page for anyone who wants it.
func HeadlineRating(ratings []RatingView) string {
	best := -1
	for i, r := range ratings {
		if strings.HasSuffix(r.Rating, "?") {
			continue
		}
		if best == -1 || r.Games > ratings[best].Games {
			best = i
		}
	}
	if best == -1 {
		return ""
	}
	r := ratings[best]
	if r.Speed == "" {
		return r.Rating
	}
	return r.Rating + " " + r.Speed
}

// JoinedMonth renders an account's creation date as "March 2026" — coarse on
// purpose: a public page has no business publishing an exact signup timestamp.
func JoinedMonth(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("January 2006")
}

// AssignableRoles lists the roles an admin may set, in ladder order.
func AssignableRoles() []string {
	return []string{
		role.Player.String(),
		role.Mod.String(),
		role.Admin.String(),
	}
}

// ResultClass maps a game score to its label and the win/loss/draw tinting
// class the archive and clocks already use.
func ResultClass(score float32) (label, class string) {
	switch {
	case score == 1:
		return "Won", "win"
	case score == 0:
		return "Lost", "loss"
	default:
		return "Drew", "draw"
	}
}

// modHistoryLabel says whether the listed entries are the whole record.
func modHistoryLabel(m ModBarView) string {
	total := strconv.FormatInt(m.ActionsTotal, 10)
	if m.ActionsTotal > int64(len(m.Actions)) {
		return "latest " + strconv.Itoa(len(m.Actions)) + " of " + total
	}
	if m.ActionsTotal == 1 {
		return "1 entry"
	}
	return total + " entries"
}

// pluralGames renders a game count for a rating tile ("1 game" / "12 games").
func pluralGames(n int) string {
	return plural(int64(n), "game", "games")
}

// StatPlaceholder is what a statistics section shows before it has the data to
// draw. Every section on this page renders its heading and frame unconditionally
// and swaps in one of these — a section that silently vanishes reads as a broken
// page to the very players (new ones) who see it most, while a placeholder that
// names what is coming is the page telling them what playing another game earns.
//
// Later phases supply a ghosted skeleton of the real chart as the component's
// children, so the placeholder previews its own shape.
type StatPlaceholder struct {
	// Copy names what will appear here, in the page's plain voice.
	Copy string
	// Have / Need drive an optional progress meter. Need == 0 hides it, for
	// sections gated on something other than a countable threshold.
	Have int64
	Need int64
	// Unit names what is being counted, plural ("rated games", "deploy games").
	Unit string
}

// Meter reports whether the placeholder shows progress toward a threshold.
func (p StatPlaceholder) Meter() bool { return p.Need > 0 && p.Have < p.Need }

// Progress phrases the distance to the threshold ("2 of 5 rated games").
func (p StatPlaceholder) Progress() string {
	return commas(p.Have) + " of " + commas(p.Need) + " " + p.Unit
}

// Width is the meter's fill as a CSS percentage.
func (p StatPlaceholder) Width() string {
	if p.Need <= 0 {
		return "0%"
	}
	pct := p.Have * 100 / p.Need
	if pct > 100 {
		pct = 100
	}
	// a sliver of fill at zero reads as "this meter is real and empty" rather
	// than as a missing element
	if pct < 2 {
		pct = 2
	}
	return strconv.FormatInt(pct, 10) + "%"
}

// openReportsLabel phrases the unresolved-report count on a player page.
func openReportsLabel(n int64) string {
	if n == 1 {
		return "1 open report against this account →"
	}
	return strconv.FormatInt(n, 10) + " open reports against this account →"
}
