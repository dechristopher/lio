// Package view holds the server-rendered UI for lioctad, built with templ
// (https://templ.guide). Page components are exported and invoked from the
// www handlers via Render; smaller components are unexported helpers composed
// within this package.
package view

import (
	"context"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/dechristopher/lio/assets"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/message"
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
	Description string // description + og:description
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
		Description: defaultDescription,
	}
}

// RoomMeta builds metadata for a room page, mirroring the OpenGraph and title
// treatment the old room/doc_title partials produced.
func RoomMeta(payload message.RoomTemplatePayload) Meta {
	group := cases.Title(language.English).String(payload.Variant.Group.String())
	challenge := group + " (" + payload.Variant.Name +
		") casual octad • Challenge from anonymous player"
	return Meta{
		Version:     config.VersionString(),
		SiteURL:     config.SiteURL(),
		Title:       challenge + " • lioctad.org",
		OGURL:       "https://lioctad.org/" + payload.RoomID,
		OGTitle:     challenge,
		Description: roomDescription,
	}
}

// Render writes a templ component to the fiber response as UTF-8 HTML.
// It is the templ replacement for the old util.HandleTemplate helper.
//
// We deliberately render with context.Background() rather than c.Context():
// user.ContextMiddleware overwrites the fiber user-context with a *user.Context
// whose embedded context.Context is nil for returning users, so templ's
// ctx.Err() check would nil-panic. The view components use no request-scoped
// context values, so a background context is both correct and safe here.
func Render(c fiber.Ctx, status int, component templ.Component) error {
	c.Status(status).Type("html", "utf-8")
	return component.Render(context.Background(), c.Response().BodyWriter())
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

// opponentName is the label shown on the opponent's clock: "BOT" for a game
// against the built-in engine, otherwise the generic "Opponent" (there are no
// human accounts/usernames yet).
func opponentName(payload message.RoomTemplatePayload) string {
	if payload.OpponentIsBot {
		return "BOT"
	}
	return "Opponent"
}

// topClockName / bottomClockName label the two clocks (and the matching match-
// timeline rows). Players see the relative You/Opponent labels; spectators see
// seats by identity — "BOT" for the engine, "PLAYER" for a human (no usernames
// yet) — because the rows are anchored to the *players*, not to the colors,
// which swap between games of a match. The anchored player (the human in a bot
// game) always holds the bottom row; each clock's left-edge stripe (app.css)
// shows the color that seat currently holds.
func topClockName(payload message.RoomTemplatePayload) string {
	if payload.IsSpectator {
		if payload.WhiteIsBot || payload.BlackIsBot {
			return "BOT"
		}
		return "PLAYER"
	}
	return opponentName(payload)
}

func bottomClockName(payload message.RoomTemplatePayload) string {
	if payload.IsSpectator {
		return "PLAYER"
	}
	return "You"
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
	return "/new/computer?tc=" + payload.Variant.HTMLName + "&color=" + color
}
