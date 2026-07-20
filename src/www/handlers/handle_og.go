package handlers

import (
	"strings"

	"github.com/dechristopher/octad/v2"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/dechristopher/lio/game"
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
		// closed rooms live on as archive permalinks; give their shared links
		// a real preview of the final position before falling back to branding
		return ogArchivedRoom(c, id)
	}

	payload := roomInstance.GenTemplatePayload("")
	group := cases.Title(language.English).String(payload.Variant.Group.String())
	title := group + " (" + payload.Variant.Name + ") casual octad"

	var subtitle string
	switch roomInstance.State() {
	case room.StateWaitingForPlayers:
		challenger := "an anonymous player"
		if payload.CreatorName != "" {
			challenger = payload.CreatorName
		}
		subtitle = "Challenge from " + challenger + " — join the game."
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

// ogArchivedRoom renders the preview card for an archived (closed) room: the
// final position of the match's last game with its last move highlighted.
// Unknown rooms still get the default branding card.
func ogArchivedRoom(c fiber.Ctx, id string) error {
	games, ok := loadRoomGames(id)
	if !ok {
		return OGDefaultHandler(c)
	}
	last := games[len(games)-1]

	g, err := game.ReplayArchive(last.StartingOfen, last.Moves)
	if err != nil {
		return OGDefaultHandler(c)
	}

	group := cases.Title(language.English).String(last.VariantGroup)
	mode := "competitive"
	if last.Casual {
		mode = "casual"
	}
	card := og.Card{
		Title:    group + " (" + last.VariantName + ") " + mode + " octad",
		Subtitle: "Archived match — see how it ended.",
		OFEN:     g.Position().String(),
	}
	if moves := g.Moves(); len(moves) > 0 {
		lastMove := moves[len(moves)-1]
		card.Marks = []octad.Square{lastMove.S1(), lastMove.S2()}
	}

	img, err := og.Render(card)
	if err != nil {
		return OGDefaultHandler(c)
	}

	// the archived position never changes; let scrapers cache it for a while
	return sendPNG(c, img, "public, max-age=3600")
}
