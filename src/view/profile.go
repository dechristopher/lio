package view

import (
	"strconv"
	"time"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/db"
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

	// Closed marks a banned account. The public page then shows a neutral
	// closed card in place of the stats and suppresses the ratings — no reason,
	// no duration, which are moderator-only (and are told to the account holder
	// at the login refusal instead).
	Closed bool

	// Ratings, records and games are omitted entirely for a closed account.
	Ratings  []RatingView
	Total    RecordView
	Variants []VariantRecordView
	Bots     []BotRecordView
	Games    []ProfileGameView

	// H2H is the viewer's own record against this account, shown only when the
	// viewer is logged in, is not this account, and the two have actually met.
	H2H     string
	H2HShow bool

	// Mod is the moderation bar's state, populated only when the viewing
	// account may moderate this one. Rendering is gated on ShowMod; every
	// action it offers is independently re-authorized server-side.
	ShowMod bool
	Mod     ModBarView
}

// RatingView is one time control's rating on the profile.
type RatingView struct {
	Category string
	Rating   string // "1653" / "1500?"
	Games    int
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
}

// ProfileGameView is one row of the recent-games list.
type ProfileGameView struct {
	URL      string // archive permalink
	When     string // "2 days ago"
	Variant  string // "½ + 1 blitz"
	Mode     string // "Rated" / "Casual"
	Result   string // "Won" / "Lost" / "Drew"
	Class    string // result-tinting class: win / loss / draw
	Opponent string // "cdpplayer" / "BOT ♛ Queen" / "Anonymous"
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
	who := m.Username
	if m.Title.Set() {
		who = m.Title.Code + " " + m.Username
	}
	desc := who + " on " + config.SiteName() + " — octad games, ratings and record."
	if m.Closed {
		desc = "This " + config.SiteName() + " account is closed."
	}
	return Meta{
		Version:     config.VersionString(),
		SiteURL:     config.SiteURL(),
		Title:       who + " • " + config.SiteName(),
		OGURL:       config.SiteOrigin() + "/@/" + m.Username,
		OGTitle:     who,
		OGImage:     config.SiteOrigin() + "/og/default.png",
		Description: desc,
	}
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
	if n == 1 {
		return "1 game"
	}
	return strconv.Itoa(n) + " games"
}

// botRecordLabel names a bot-persona record row using the same glyph+name pair
// the clocks and archive show ("♛ Queen"), so one bot reads identically
// everywhere.
func botRecordLabel(b BotRecordView) string {
	if b.Glyph == "" {
		return b.Persona
	}
	return b.Glyph + " " + b.Persona
}

// openReportsLabel phrases the unresolved-report count on a player page.
func openReportsLabel(n int64) string {
	if n == 1 {
		return "1 open report against this account →"
	}
	return strconv.FormatInt(n, 10) + " open reports against this account →"
}
