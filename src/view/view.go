// Package view holds the server-rendered UI for lioctad, built with templ
// (https://templ.guide). Page components are exported and invoked from the
// www handlers via Render; smaller components are unexported helpers composed
// within this package.
package view

import (
	"context"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/dechristopher/lio/assets"
	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/engine"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/variant"
)

// asset returns the content-hashed public URL for an embedded static asset,
// named by its slash-relative path under static/ (e.g. asset("lio-game.js") ->
// "/lio-game.<hash>.js"). templ components use it for every script/link href so
// the browser cache busts exactly when a file changes. See the assets package.
func asset(name string) string {
	return assets.URL(name)
}

// Meta carries the per-page metadata needed to render the document <head>.
// It replaces the old untyped pageModel: the room-vs-default branching that
// used to live in the head/doc_title partials is now resolved up front by the
// PageMeta / RoomMeta constructors into explicit fields.
type Meta struct {
	Version     string // build version, shown in the footer
	SiteURL     string // absolute site URL (already includes trailing '/')
	Title       string // full <title> contents
	OGURL       string // og:url
	OGTitle     string // og:title
	OGImage     string // og:image — absolute URL of the preview card PNG
	Description string // description + og:description
	// Notice is an optional one-shot banner rendered near the top of the home
	// page (e.g. "that room is gone" after a client is redirected off a room
	// that a server restart dropped). Empty renders nothing.
	Notice string
}

const (
	defaultOGTitle     = "Octad — 4x4 chess with a twist"
	defaultDescription = "Free online octad server. Play octad in a clean interface. " +
		"Play octad with the computer, friends or random players. No ads."
	roomDescription = "Join the challenge or watch the game here."
)

// PageMeta builds metadata for a standard (non-room) page. name is rendered in
// the title as "lioctad.org • {name}".
func PageMeta(name string) Meta {
	return Meta{
		Version:     config.VersionString(),
		SiteURL:     config.SiteURL(),
		Title:       "lioctad.org • " + name,
		OGURL:       "https://lioctad.org",
		OGTitle:     defaultOGTitle,
		OGImage:     "https://lioctad.org/og/default.png",
		Description: defaultDescription,
	}
}

// RoomMeta builds metadata for a room page, mirroring the OpenGraph and title
// treatment the old room/doc_title partials produced.
func RoomMeta(payload message.RoomTemplatePayload) Meta {
	group := cases.Title(language.English).String(payload.Variant.Group.String())
	// untimed games are casual; anything with a real clock is competitive
	mode := "competitive"
	if payload.Variant.Casual {
		mode = "casual"
	}
	challenger := "anonymous player"
	if payload.CreatorName != "" {
		challenger = payload.CreatorName
		if payload.CreatorRating != "" {
			challenger += " (" + payload.CreatorRating + ")"
		}
	}
	challenge := group + " (" + payload.Variant.Name +
		") " + mode + " octad • Challenge from " + challenger
	return Meta{
		Version:     config.VersionString(),
		SiteURL:     config.SiteURL(),
		Title:       challenge + " • lioctad.org",
		OGURL:       "https://lioctad.org/" + payload.RoomID,
		OGTitle:     challenge,
		OGImage:     "https://lioctad.org/og/room/" + payload.RoomID + ".png",
		Description: roomDescription,
	}
}

// Viewer is the render-scoped identity components read via viewer(ctx): who
// is looking at the page. It is the only request-derived value that crosses
// into templ (see Render); every field is a plain value copied out of the
// session-resolved user.Context (DB-owned strings — nothing referencing
// fasthttp's pooled buffers survives into the render).
type Viewer struct {
	UID             string // session uid ("" when no session resolved)
	LoggedIn        bool
	Username        string // display-case username, empty when anonymous
	AccountsEnabled bool   // false only in PG-less local dev
}

// viewerKey keys the Viewer in the render context.
type viewerKey struct{}

// viewerFrom snapshots the request's identity for the render context.
func viewerFrom(c fiber.Ctx) Viewer {
	v := Viewer{AccountsEnabled: auth.Enabled()}
	if uc := user.GetContext(c); uc != nil {
		v.UID = uc.ID
		if uc.Account != nil {
			v.LoggedIn = true
			v.Username = uc.Account.Username
		}
	}
	return v
}

// viewer returns the rendering request's Viewer; the zero Viewer outside a
// Render call (component smoke tests).
func viewer(ctx context.Context) Viewer {
	if v, ok := ctx.Value(viewerKey{}).(Viewer); ok {
		return v
	}
	return Viewer{}
}

// Render writes a templ component to the fiber response as UTF-8 HTML.
// It is the templ replacement for the old util.HandleTemplate helper.
//
// We deliberately render on a fresh background-derived context rather than
// c.Context(): the session middleware overwrites the fiber user-context with a
// *user.Context, and templ only needs the Viewer value — request-scoped
// deadlines/values have no business in the render. The Viewer is injected
// explicitly; components must never reach for the fiber ctx.
func Render(c fiber.Ctx, status int, component templ.Component) error {
	c.Status(status).Type("html", "utf-8")
	ctx := context.WithValue(context.Background(), viewerKey{}, viewerFrom(c))
	return component.Render(ctx, c.Response().BodyWriter())
}

// IsHTMXFragment reports whether a request should be answered with a bare HTMX
// fragment instead of a full page. True for htmx-initiated requests (HX-Request)
// except history-restore requests, which need the full document to rebuild the
// page from a cache miss. Handlers use this to serve the same URL as either a
// swap-in fragment or a directly-navigable full page.
func IsHTMXFragment(c fiber.Ctx) bool {
	return c.Get("HX-Request") == "true" &&
		c.Get("HX-History-Restore-Request") != "true"
}

// groupTitle title-cases a variant speed group ("blitz" → "Blitz") for display
// in the pre-game summary.
func groupTitle(g variant.Group) string {
	return cases.Title(language.English).String(g.String())
}

// humanClock renders a variant's clock in plain language for the pre-game
// summary ("30 seconds each + 1 second per move") so newcomers don't have to
// decode the "½ + 1" notation to know what they're signing up for. Casual
// variants read "Unlimited time"; the delay-only ulti control reads
// "5 second delay per move".
func humanClock(v variant.Variant) string {
	if v.Casual {
		return "Unlimited time"
	}
	var parts []string
	if base := v.Control.Time.Milli() / 1000; base > 0 {
		parts = append(parts, humanSeconds(base)+" each")
	}
	if inc := v.Control.Increment.Milli() / 1000; inc > 0 {
		parts = append(parts, humanSeconds(inc)+" per move")
	}
	if delay := v.Control.Delay.Milli() / 1000; delay > 0 {
		parts = append(parts, humanSeconds(delay)+" delay per move")
	}
	return strings.Join(parts, " + ")
}

// humanSeconds renders a second count as prose, preferring whole minutes
// ("30 seconds", "1 minute", "3 minutes").
func humanSeconds(n int64) string {
	if n >= 60 && n%60 == 0 {
		m := n / 60
		if m == 1 {
			return "1 minute"
		}
		return strconv.FormatInt(m, 10) + " minutes"
	}
	if n == 1 {
		return "1 second"
	}
	return strconv.FormatInt(n, 10) + " seconds"
}

// challengerInitial is the single-letter avatar chip for the joiner's
// challenger card ("?" for an anonymous creator).
func challengerInitial(name string) string {
	if name == "" {
		return "?"
	}
	return strings.ToUpper(string([]rune(name)[0]))
}

// challengerColorName names the side the challenger will play in the joiner's
// pre-game card. The joiner's PlayerColor is the open seat, so the challenger
// holds the other color; a blind-color (random) room hides both until the
// board reveals them.
func challengerColorName(payload message.RoomTemplatePayload) string {
	if payload.BlindColor {
		return "a random color"
	}
	if payload.PlayerColor == "w" {
		return "Black"
	}
	return "White"
}

// seatColorLabel resolves one seat's clock/timeline label from the payload:
// the difficulty persona's name for the engine ("Queen" — the piece glyph is
// rendered separately as the clock's bot avatar, see BotSeatGlyph), the account
// username when the seat is logged in, "You" for the anonymous viewer's own
// seat, and "Anonymous" for any other anonymous human. color is "w"/"b"; isBot
// is that seat's bot flag.
func seatColorLabel(payload message.RoomTemplatePayload, color string, isBot bool) string {
	if isBot {
		return BotSeatLabel(payload.BotPersona)
	}
	name := ""
	switch color {
	case "w":
		name = payload.WhiteName
	case "b":
		name = payload.BlackName
	}
	if name != "" {
		return name
	}
	// anonymous human: the viewer's own seat reads "You", everyone else
	// "Anonymous". A spectator has no own seat, so both seats read "Anonymous".
	if !payload.IsSpectator && color == payload.PlayerColor {
		return "You"
	}
	return "Anonymous"
}

// ratingDeltaClass / ratingDeltaText render a per-game rating change beside the
// archive clock's rating: green (win) for a gain, red (loss) for a loss, and
// the signed number ("+8" / "-8"). Zero is never rendered (the clock omits the
// delta span), so it needs no class.
func ratingDeltaClass(d int) string {
	if d > 0 {
		return "win"
	}
	if d < 0 {
		return "loss"
	}
	return ""
}

func ratingDeltaText(d int) string {
	if d > 0 {
		return "+" + strconv.Itoa(d)
	}
	return strconv.Itoa(d) // negatives carry their own '-'
}

// seatColorRating resolves one seat's clock rating display (by "w"/"b" color),
// or "" when the seat has none (anonymous/bot, or an unrated game).
func seatColorRating(payload message.RoomTemplatePayload, color string) string {
	switch color {
	case "w":
		return payload.WhiteRating
	case "b":
		return payload.BlackRating
	}
	return ""
}

// topClockRating / bottomClockRating label each clock with the seat's rating,
// resolved by the same color/anchor rules as the names.
func topClockRating(payload message.RoomTemplatePayload) string {
	return seatColorRating(payload, topClockColor(payload))
}

func bottomClockRating(payload message.RoomTemplatePayload) string {
	return seatColorRating(payload, bottomClockColor(payload))
}

// h2hText formats a head-to-head score for display, matching the timeline
// totals' ½ convention (2 → "2", 1.5 → "1½", 0.5 → "½"). Scores are always
// non-negative multiples of ½ (win = 1, draw = ½).
func h2hText(score float64) string {
	whole := int(score) // floor for the non-negative scores here
	if score-float64(whole) >= 0.5 {
		if whole == 0 {
			return "½"
		}
		return strconv.Itoa(whole) + "½"
	}
	return strconv.Itoa(whole)
}

// seatH2H resolves one seat's head-to-head score by "w"/"b" color from the
// payload's color-keyed values.
func seatH2H(payload message.RoomTemplatePayload, color string) float64 {
	if color == "w" {
		return payload.H2HWhite
	}
	return payload.H2HBlack
}

// topH2HScore / bottomH2HScore map the head-to-head score onto the two timeline
// rows by the same color/anchor rules as the names and ratings.
func topH2HScore(payload message.RoomTemplatePayload) float64 {
	return seatH2H(payload, topClockColor(payload))
}

func bottomH2HScore(payload message.RoomTemplatePayload) float64 {
	return seatH2H(payload, bottomClockColor(payload))
}

// otherColorStr flips a "w"/"b" color string (passthrough for anything else).
func otherColorStr(c string) string {
	switch c {
	case "w":
		return "b"
	case "b":
		return "w"
	default:
		return c
	}
}

// topClockColor / bottomClockColor resolve which seat (by color) each clock
// row shows. A player is always on the bottom (their opponent on top); a
// spectator's board is oriented to the anchored player, who holds the bottom
// row across the color flips between games of a match.
func bottomClockColor(payload message.RoomTemplatePayload) string {
	if payload.IsSpectator {
		return payload.AnchorColor
	}
	return payload.PlayerColor
}

func topClockColor(payload message.RoomTemplatePayload) string {
	if payload.IsSpectator {
		return otherColorStr(payload.AnchorColor)
	}
	return payload.OpponentColor
}

// topClockName / bottomClockName label the two clocks (and the matching match-
// timeline rows) by the seat each shows — usernames for logged-in players,
// You/Anonymous for anonymous humans, "BOT" for the engine. The rows are
// anchored to the *players*, not the colors, which swap between games of a
// match; the anchored player (the human in a bot game) always holds the bottom
// row and each clock's left-edge stripe (app.css) shows its current color.
func topClockName(payload message.RoomTemplatePayload) string {
	return seatColorLabel(payload, topClockColor(payload), topClockIsBot(payload))
}

func bottomClockName(payload message.RoomTemplatePayload) string {
	return seatColorLabel(payload, bottomClockColor(payload), bottomClockIsBot(payload))
}

// topClockIsBot / bottomClockIsBot drive the data-bot attribute that the
// client's engine "thinking" indicator keys off. For a player the top clock is
// the opponent and the bottom (their own) is never a bot; for a spectator the
// anchor pins the human to the bottom, so a bot seat is always the top clock.
func topClockIsBot(payload message.RoomTemplatePayload) bool {
	if payload.IsSpectator {
		return payload.WhiteIsBot || payload.BlackIsBot
	}
	return payload.OpponentIsBot
}

func bottomClockIsBot(message.RoomTemplatePayload) bool {
	return false
}

// controlTitle returns a game-control button's tooltip: the action for a
// player, or a "watching only" explanation for a spectator, whose controls
// render permanently disabled.
func controlTitle(payload message.RoomTemplatePayload, action string) string {
	if payload.IsSpectator {
		return "Watching as a spectator"
	}
	return action
}

// boardOrientation returns the #gcon-xx orientation class: the player's own
// color, or the anchored player's current color for spectators (whose
// PlayerColor is NoColor / "-") — see RoomTemplatePayload.AnchorColor.
func boardOrientation(payload message.RoomTemplatePayload) string {
	if payload.IsSpectator {
		if payload.AnchorColor == "b" {
			return "b"
		}
		return "w"
	}
	return payload.PlayerColor
}

// botRematchURL builds the "same settings" rematch link for a bot game: a fresh
// vs-computer room with the same variant/time-control and the player's side. Bot
// rematch does not reuse the finished room (which is torn down after the analysis
// window), so the client navigates here instead — see NewRoomVsComputer. Returns
// "" for a human game, where rematch stays the in-room agreement flow.
func botRematchURL(payload message.RoomTemplatePayload) string {
	if !payload.OpponentIsBot {
		return ""
	}
	color := "w"
	// octad.Color.String() renders as "w"/"b" (not the full color name)
	if payload.PlayerColor == "b" {
		color = "b"
	}
	return "/new/computer?tc=" + payload.Variant.HTMLName + "&color=" + color +
		"&bot=" + engine.PersonaByKey(payload.BotPersona).Key
}

// BotSeatLabel names a bot seat by its difficulty persona name ("Queen"). The
// persona's piece glyph is rendered separately (BotSeatGlyph) as the clock's
// bot avatar so it aligns in its own slot rather than sitting on the text
// baseline. An empty/unknown key (every pre-persona room and archived game)
// resolves to the full-strength Queen, which is exactly what those bots played
// as.
func BotSeatLabel(personaKey string) string {
	return engine.PersonaByKey(personaKey).Name
}

// BotSeatGlyph is a bot seat's difficulty-persona piece glyph ("♛︎"), the
// clock's bot avatar (rendered in the .clockBot slot in place of the generic
// CPU icon). Empty/unknown keys resolve to the full-strength Queen.
func BotSeatGlyph(personaKey string) string {
	return engine.PersonaByKey(personaKey).Glyph
}

// topClockBotGlyph / bottomClockBotGlyph give the persona piece glyph for a
// clock whose seat is the engine, else "" (the clock then renders the generic
// CPU icon, kept hidden by CSS for a human seat). The bottom clock is never a
// bot (the human is always anchored there), so it is always "".
func topClockBotGlyph(payload message.RoomTemplatePayload) string {
	if topClockIsBot(payload) {
		return BotSeatGlyph(payload.BotPersona)
	}
	return ""
}

func bottomClockBotGlyph(payload message.RoomTemplatePayload) string {
	if bottomClockIsBot(payload) {
		return BotSeatGlyph(payload.BotPersona)
	}
	return ""
}
