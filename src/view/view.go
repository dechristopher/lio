// Package view holds the server-rendered UI for lioctad, built with templ
// (https://templ.guide). Page components are exported and invoked from the
// www handlers via Render; smaller components are unexported helpers composed
// within this package.
package view

import (
	"context"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/variant"
)

// Meta carries the per-page metadata needed to render the document <head>.
// It replaces the old untyped pageModel: the room-vs-default branching that
// used to live in the head/doc_title partials is now resolved up front by the
// PageMeta / RoomMeta constructors into explicit fields.
type Meta struct {
	Version     string // build version, shown in the footer
	CacheKey    string // static-asset cache-buster (already includes leading '.')
	SiteURL     string // absolute site URL (already includes trailing '/')
	Title       string // full <title> contents
	OGURL       string // og:url
	OGTitle     string // og:title
	Description string // description + og:description
}

const (
	defaultOGTitle     = "The best free, adless octad server"
	defaultDescription = "Free online octad server. Play octad in a clean interface. " +
		"No registration, no ads. Play octad with the computer, friends or random players."
	roomDescription = "Join the challenge or watch the game here."
)

// PageMeta builds metadata for a standard (non-room) page. name is rendered in
// the title as "lioctad.org • {name}".
func PageMeta(name string) Meta {
	return Meta{
		Version:     config.Version,
		CacheKey:    config.CacheKey,
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
		Version:     config.Version,
		CacheKey:    config.CacheKey,
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
// We deliberately render with context.Background() rather than c.UserContext():
// user.ContextMiddleware overwrites the fiber user-context with a *user.Context
// whose embedded context.Context is nil for returning users, so templ's
// ctx.Err() check would nil-panic. The view components use no request-scoped
// context values, so a background context is both correct and safe here.
func Render(c *fiber.Ctx, status int, component templ.Component) error {
	c.Status(status).Type("html", "utf-8")
	return component.Render(context.Background(), c.Response().BodyWriter())
}

// IsHTMXFragment reports whether a request should be answered with a bare HTMX
// fragment instead of a full page. True for htmx-initiated requests (HX-Request)
// except history-restore requests, which need the full document to rebuild the
// page from a cache miss. Handlers use this to serve the same URL as either a
// swap-in fragment or a directly-navigable full page.
func IsHTMXFragment(c *fiber.Ctx) bool {
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
	if payload.PlayerColor == "black" {
		color = "b"
	}
	return "/new/computer?tc=" + payload.Variant.HTMLName + "&color=" + color
}
