package handlers

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/view"
	"github.com/dechristopher/lio/www/ws/proto"
)

// The permanent room/game permalinks: once a room's games are archived to
// Postgres, /<room_id> (after the room actor closes), /<room_id>/<n>, and
// /game/<uuid> serve a read-only archive view instead of 404ing. The page
// carries the full match history inline (view.ArchiveData) and never opens a
// websocket. Without Postgres every path here degrades to today's 404s.

// notFound falls through to the site-wide 404 page.
func notFound(c fiber.Ctx) error {
	return c.Status(fiber.StatusNotFound).Next()
}

// ArchiveRoomFallback serves the archived match view for a room whose live
// actor no longer exists (the RoomHandler miss path). Defaults to the match's
// last game. Rooms that never archived a game still 404.
func ArchiveRoomFallback(c fiber.Ctx) error {
	games, ok := loadRoomGames(c.Params("id"))
	if !ok {
		return notFound(c)
	}
	return renderArchive(c, games, len(games), false)
}

// ArchiveGameHandler serves /<room_id>/<n>: the permalink to game n (1-based
// match ordinal) of a room's match — served from the archive even while the
// room is still live (games archive the moment they finish). An out-of-range
// n (including the current unfinished game) redirects to the room itself.
func ArchiveGameHandler(c fiber.Ctx) error {
	id := c.Params("id")
	n, err := strconv.Atoi(c.Params("num"))
	if err != nil || n < 1 {
		return notFound(c)
	}

	// a seated player of the still-live room belongs on the live page, where
	// past games are browsable in place and the rematch window stays intact —
	// never on a socket-less archive view of their own ongoing match
	if liveRoom, lErr := room.Get(id); lErr == nil && liveRoom != nil &&
		liveRoom.IsPlayer(user.GetID(c)) {
		return redirect(c, "/"+id)
	}

	games, ok := loadRoomGames(id)
	if !ok {
		// nothing archived under this room yet: bounce a live room's link back
		// to the room page, else 404
		if liveRoom, lErr := room.Get(id); lErr == nil && liveRoom != nil {
			return redirect(c, "/"+id)
		}
		return notFound(c)
	}
	if n > len(games) {
		return redirect(c, "/"+id)
	}
	return renderArchive(c, games, n, false)
}

// ArchiveGameByUUIDHandler serves /game/<uuid>: the direct permalink by game
// UUID. Games with a room association 301 to their canonical /<room_id>/<n>
// URL; room-less games (the pre-relational backfill) render standalone.
func ArchiveGameByUUIDHandler(c fiber.Ctx) error {
	g, found, err := db.GetGameByUUID(c.Params("uuid"))
	if err != nil {
		util.Error(str.CRoom, "archive game lookup failed: %s", err.Error())
		return notFound(c)
	}
	if !found {
		return notFound(c)
	}

	if g.RoomID != "" {
		// the room/ordinal mapping is immutable once archived
		return c.Redirect().Status(fiber.StatusMovedPermanently).
			To(fmt.Sprintf("/%s/%d", g.RoomID, g.GameIndex))
	}

	return renderArchive(c, []gen.Game{g}, 1, true)
}

// ArchiveGameJSONHandler serves GET /api/room/:id/game/:num — the archived
// game data behind in-room match browsing (see view.ArchiveGameData). The
// payload for a given (room, n) never changes once archived, so it is served
// immutable-cacheable.
func ArchiveGameJSONHandler(c fiber.Ctx) error {
	id := c.Params("id")
	n, err := strconv.Atoi(c.Params("num"))
	if err != nil || n < 1 || !db.Ready() {
		return fiber.ErrNotFound
	}

	g, found, err := db.GetRoomGameByIndex(id, int16(n))
	if err != nil {
		util.Error(str.CRoom, "archive game json lookup failed id=%s n=%d: %s",
			id, n, err.Error())
		return fiber.ErrNotFound
	}
	if !found {
		return fiber.ErrNotFound
	}

	og, err := replayArchivedGame(g)
	if err != nil {
		return fiber.ErrNotFound
	}

	c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	return c.JSON(view.ArchiveGameData{
		RoomID:  g.RoomID,
		GameID:  g.GameID.String(),
		N:       int(g.GameIndex),
		Board:   boardPayload(g, og),
		Winner:  winnerFromOutcome(g.Outcome),
		Reason:  g.Reason,
		Outcome: g.Outcome,
	})
}

// loadRoomGames fetches a room's archived games in match order, reporting
// ok=false when the archive is unavailable, the room is untracked, or it has
// no games.
func loadRoomGames(id string) ([]gen.Game, bool) {
	if !db.Ready() || id == "" {
		return nil, false
	}
	_, found, err := db.GetArchivedRoom(id)
	if err != nil {
		util.Error(str.CRoom, "archive room lookup failed id=%s: %s", id, err.Error())
		return nil, false
	}
	if !found {
		return nil, false
	}
	games, err := db.ListRoomGames(id)
	if err != nil {
		util.Error(str.CRoom, "archive games lookup failed id=%s: %s", id, err.Error())
		return nil, false
	}
	if len(games) == 0 {
		return nil, false
	}
	return games, true
}

// renderArchive builds the archive view model for game n of the given match
// (games in ordinal order) and renders the page. standalone marks a room-less
// single-game view (no timeline).
func renderArchive(c fiber.Ctx, games []gen.Game, n int, standalone bool) error {
	selected := games[n-1]

	og, err := replayArchivedGame(selected)
	if err != nil {
		return notFound(c)
	}

	// orient the board (and the You/Opponent labels) to a returning
	// participant; everyone else views from white's side
	uid := user.GetID(c)
	orientation := "w"
	if uid != "" && uid == selected.BlackUid {
		orientation = "b"
	}
	viewerPlayed := uid != "" && (uid == selected.WhiteUid || uid == selected.BlackUid)

	bottomUID := selected.WhiteUid
	topUID := selected.BlackUid
	if orientation == "b" {
		bottomUID, topUID = topUID, bottomUID
	}

	model := view.ArchiveModel{
		RoomID:       selected.RoomID,
		VariantName:  selected.VariantName,
		VariantGroup: selected.VariantGroup,
		Casual:       selected.Casual,
		RaceTo:       int(selected.RaceTo),
		N:            n,
		Count:        len(games),
		Standalone:   standalone,
		Orientation:  orientation,
		TopName:      seatLabel(topUID, viewerPlayed, uid),
		BottomName:   seatLabel(bottomUID, viewerPlayed, uid),
		EndedDate:    selected.EndTs.Time.Format("Jan 2, 2006"),
		Data:         buildArchiveData(games, selected, og),
	}

	return view.Render(c, fiber.StatusOK, view.RoomArchive(view.ArchiveMeta(model), model))
}

// seatLabel names a timeline row's seat: the engine is "BOT" (bot seats hold
// an empty uid — they never join), a returning participant sees themselves as
// "You" and their opponent as "Opponent", and everyone else is "PLAYER".
func seatLabel(seatUID string, viewerPlayed bool, viewerUID string) string {
	if seatUID == "" {
		return "BOT"
	}
	if viewerPlayed {
		if seatUID == viewerUID {
			return "You"
		}
		return "Opponent"
	}
	return "PLAYER"
}

// replayArchivedGame rebuilds a finished game from its archived row, logging
// the (corrupt-row) failure case once for every caller.
func replayArchivedGame(g gen.Game) (*game.OctadGame, error) {
	replayed, err := game.ReplayArchive(g.StartingOfen, g.Moves)
	if err != nil {
		// an unreplayable archive row is corrupt; log loudly and let the
		// caller 404 rather than render a broken board
		util.Error(str.CRoom, "archive replay failed game=%s: %s",
			g.GameID.String(), err.Error())
		return nil, err
	}
	return &game.OctadGame{Game: *replayed}, nil
}

// boardPayload builds the MovePayload-shaped board block (final OFEN, per-ply
// histories, seat uids) for an archived game.
func boardPayload(g gen.Game, og *game.OctadGame) proto.MovePayload {
	ofens := og.OFENHistory()
	return proto.MovePayload{
		OFEN:  ofens[len(ofens)-1],
		Moves: og.MoveHistory(),
		SANs:  og.SANHistory(),
		OFENs: ofens,
		White: g.WhiteUid,
		Black: g.BlackUid,
	}
}

// buildArchiveData assembles the client hydration payload for the selected
// game of a match, reusing the live proto payload shapes (see
// view.ArchiveData). Score and history are keyed by the selected game's seats.
func buildArchiveData(games []gen.Game, selected gen.Game, og *game.OctadGame) view.ArchiveData {
	data := view.ArchiveData{
		RoomID:  selected.RoomID,
		GameID:  selected.GameID.String(),
		N:       int(selected.GameIndex),
		Count:   len(games),
		Board:   boardPayload(selected, og),
		Winner:  winnerFromOutcome(selected.Outcome),
		Reason:  selected.Reason,
		Outcome: selected.Outcome,
	}

	if selected.RoomID == "" {
		// standalone game: no match context to score
		data.N = 1
		return data
	}

	// cumulative match score after the last game, mapped onto the selected
	// game's seats (players swap colors between games of a match)
	last := games[len(games)-1]
	lastWhiteFinal, lastBlackFinal := float64(last.WhiteScore), float64(last.BlackScore)
	if last.WhiteUid == selected.WhiteUid {
		data.Score = proto.ScorePayload{"w": lastWhiteFinal, "b": lastBlackFinal}
	} else {
		data.Score = proto.ScorePayload{"w": lastBlackFinal, "b": lastWhiteFinal}
	}

	// per-game history entries, keyed by the selected game's seats: wp is the
	// color the selected game's white player held in each game (the client's
	// renderTimeline convention, with the selected game standing in for the
	// live "current" game)
	history := make(proto.MatchHistoryPayload, 0, len(games))
	for _, gm := range games {
		wPts, bPts := pointsFromOutcome(gm.Outcome)
		entry := proto.GameHistoryEntry{Reason: gm.Reason}
		if gm.WhiteUid == selected.WhiteUid {
			entry.WhitePlayed = "w"
			entry.White, entry.Black = wPts, bPts
		} else {
			entry.WhitePlayed = "b"
			entry.White, entry.Black = bPts, wPts
		}
		history = append(history, entry)
	}
	data.History = history
	return data
}

// winnerFromOutcome maps a PGN result token to the client's winner code.
func winnerFromOutcome(outcome string) string {
	switch outcome {
	case "1-0":
		return "w"
	case "0-1":
		return "b"
	case "1/2-1/2":
		return "d"
	}
	return ""
}

// pointsFromOutcome maps a PGN result token to (white, black) game points.
func pointsFromOutcome(outcome string) (float64, float64) {
	switch outcome {
	case "1-0":
		return 1, 0
	case "0-1":
		return 0, 1
	case "1/2-1/2":
		return 0.5, 0.5
	}
	return 0, 0
}
