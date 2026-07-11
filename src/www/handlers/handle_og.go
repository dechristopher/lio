package handlers

import (
	"strings"

	"github.com/dechristopher/octad/v2"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/dechristopher/lio/og"
	"github.com/dechristopher/lio/room"
)

// sendPNG writes an encoded card with the given Cache-Control header.
func sendPNG(c fiber.Ctx, img []byte, cacheControl string) error {
	c.Set(fiber.HeaderCacheControl, cacheControl)
	c.Type("png")
	return c.Send(img)
}

// OGDefaultHandler serves the site-wide OpenGraph preview card (starting
// position + tagline) referenced by every non-room page's og:image tag. The
// card is rendered once per process, so it can be cached hard.
func OGDefaultHandler(c fiber.Ctx) error {
	img, err := og.Default()
	if err != nil {
		return fiber.ErrInternalServerError
	}
	return sendPNG(c, img, "public, max-age=86400")
}

// OGRoomHandler serves a room's OpenGraph preview card: the current board
// position (with the last move highlighted) beside the same challenge text
// the room page's og:title carries. Unknown or torn-down rooms fall back to
// the default card — scrapers of dead links still get branding, never a 404
// image. The route is read-only: fetching a preview can never join, spectate,
// or otherwise touch the room.
func OGRoomHandler(c fiber.Ctx) error {
	id := strings.TrimSuffix(c.Params("id"), ".png")

	roomInstance, err := room.Get(id)
	if err != nil || roomInstance == nil {
		return OGDefaultHandler(c)
	}

	payload := roomInstance.GenTemplatePayload("")
	group := cases.Title(language.English).String(payload.Variant.Group.String())
	title := group + " (" + payload.Variant.Name + ") casual octad"

	var subtitle string
	switch roomInstance.State() {
	case room.StateWaitingForPlayers:
		subtitle = "Challenge from anonymous player — join the game."
	case room.StateGameOver, room.StateRoomOver:
		subtitle = "Game finished — see how it ended."
	default:
		subtitle = "Game in progress — watch it live."
	}

	card := og.Card{Title: title, Subtitle: subtitle}
	if ofen, s1, s2, hasLast := roomInstance.PositionSnapshot(); ofen != "" {
		card.OFEN = ofen
		if hasLast {
			card.Marks = []octad.Square{s1, s2}
		}
	}

	img, err := og.Render(card)
	if err != nil {
		// a mid-deploy or otherwise unparseable OFEN falls back to branding
		return OGDefaultHandler(c)
	}

	// the same URL's content changes every move, so ask scrapers/proxies to
	// revalidate rather than pin the first fetch
	return sendPNG(c, img, "no-cache")
}
