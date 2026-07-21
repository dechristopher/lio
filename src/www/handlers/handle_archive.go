package handlers

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/pools"
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

// ArchiveGameEvalsHandler serves GET /api/room/:id/game/:num/evals — the
// cached per-ply engine evals for a finished game of a live room, fetched by
// lio-game.js when the player enters analysis after a bot game (the archive
// write and the background evaluator fill the cache out-of-band). Unlike the
// immutable game JSON endpoint this is served uncacheable: the evaluator fills
// evals lazily, so the response legitimately changes over time. ev is omitted
// entirely while no ply has an eval yet.
func ArchiveGameEvalsHandler(c fiber.Ctx) error {
	id := c.Params("id")
	n, err := strconv.Atoi(c.Params("num"))
	if err != nil || n < 1 || !db.Ready() {
		return fiber.ErrNotFound
	}
	g, found, err := db.GetRoomGameByIndex(id, int16(n))
	if err != nil || !found {
		return fiber.ErrNotFound
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(struct {
		Evals []*int16 `json:"ev,omitempty"`
	}{Evals: db.ListGameMoveEvals(g.ID)})
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

	// resolve each seat's account username (empty for anon/bot); the archive
	// has no live player records, so it reads them off the games row's user-id
	// FKs (arch/ACCOUNTS_AUTH_RATINGS.md Phase 2)
	whiteName := db.UsernameForID(selected.WhiteUserID)
	blackName := db.UsernameForID(selected.BlackUserID)

	bottomUID, bottomName, bottomUserID := selected.WhiteUid, whiteName, selected.WhiteUserID
	topUID, topName, topUserID := selected.BlackUid, blackName, selected.BlackUserID
	// rating "at the time of the game" + that game's +/- change, per seat
	// (empty/zero for casual/anon/bot and pre-tracking rows)
	bottomRating, bottomDelta := derefStr(selected.WhiteRating), derefInt16(selected.WhiteRatingDelta)
	topRating, topDelta := derefStr(selected.BlackRating), derefInt16(selected.BlackRatingDelta)
	if orientation == "b" {
		bottomUID, topUID = topUID, bottomUID
		bottomName, topName = topName, bottomName
		bottomUserID, topUserID = topUserID, bottomUserID
		bottomRating, topRating = topRating, bottomRating
		bottomDelta, topDelta = topDelta, bottomDelta
	}

	// all-time head-to-head score between the two seats' accounts (shown beside
	// the timeline names). Keyed A=bottom, B=top to match the oriented rows;
	// zero-Games means nothing to show (a bot/anonymous seat or a first meeting).
	var topH2H, bottomH2H float64
	h2hShow := false
	if !standalone {
		if h := db.HeadToHead(bottomUserID, topUserID); h.Games > 0 {
			bottomH2H, topH2H = h.AScore, h.BScore
			h2hShow = true
		}
	}

	model := view.ArchiveModel{
		RoomID:            selected.RoomID,
		VariantName:       selected.VariantName,
		VariantGroup:      selected.VariantGroup,
		Casual:            selected.Casual,
		RaceTo:            int(selected.RaceTo),
		N:                 n,
		Count:             len(games),
		Standalone:        standalone,
		Orientation:       orientation,
		TopName:           seatLabel(topUID, topName, topUserID, uid, derefStr(selected.BotPersona)),
		BottomName:        seatLabel(bottomUID, bottomName, bottomUserID, uid, derefStr(selected.BotPersona)),
		TopRating:         topRating,
		BottomRating:      bottomRating,
		TopRatingDelta:    topDelta,
		BottomRatingDelta: bottomDelta,
		TopIsBot:          isBotSeat(topUID, topUserID),
		BottomIsBot:       isBotSeat(bottomUID, bottomUserID),
		TopGlyph:          seatGlyph(isBotSeat(topUID, topUserID), derefStr(selected.BotPersona)),
		BottomGlyph:       seatGlyph(isBotSeat(bottomUID, bottomUserID), derefStr(selected.BotPersona)),
		TopH2H:            topH2H,
		BottomH2H:         bottomH2H,
		H2HShow:           h2hShow,
		TCCenti:           archiveTimeCenti(selected.VariantName, selected.VariantGroup),
		EndedDate:         selected.EndTs.Time.Format("Jan 2, 2006"),
		Data:              buildArchiveData(games, selected, og),
	}

	return view.Render(c, fiber.StatusOK, view.RoomArchive(view.ArchiveMeta(model), model))
}

// archiveTimeCenti resolves an archived game's full starting clock budget
// (centiseconds) from the variant registry — the games row stores only the
// variant's display name and group, not its numbers. A same-name variant in a
// different group (the deploy pairing shares its base control's name) is an
// acceptable fallback: the paired variants differ only in pre-start, never in
// budget. Returns 0 when the name no longer resolves at all (a retired
// variant); the client then derives the budget from the ply-1 clock.
func archiveTimeCenti(name, group string) int64 {
	var byName int64
	for _, v := range pools.Map {
		if v.Name != name {
			continue
		}
		if string(v.Group) == group {
			return v.Control.Time.Centi()
		}
		byName = v.Control.Time.Centi()
	}
	return byName
}

// isBotSeat reports whether an archived seat was the engine. A bot seat holds
// no identity at all — no session uid (bots never join over a socket) AND no
// account. This is deliberately stricter than the old "empty uid" test: a real
// logged-in player whose session uid was lost to the deploy-rebuild bug (fixed
// in Join) still carries an account FK, so keying bot-ness off the uid alone
// mislabeled such humans as "BOT" in the archive. The account check rescues
// those rows without a data migration.
func isBotSeat(seatUID string, seatUserID *int64) bool {
	return seatUID == "" && seatUserID == nil
}

// seatLabel names a timeline row's seat, mirroring the live-room rules: the
// engine shows its difficulty persona name ("Queen" — botPersona is the row's
// games.bot_persona stamp, NULL/empty resolving to the full-strength Queen
// every pre-persona bot played as; the piece glyph is the clock avatar, see
// seatGlyph), a seat with an account shows its username to everyone (including
// that player), the anonymous viewer's own seat reads "You", and any other
// anonymous human is "Anonymous". A seat with an account but a lost session uid
// (the deploy-rebuild bug) still resolves by its username, never as a bot.
func seatLabel(seatUID, seatUsername string, seatUserID *int64, viewerUID, botPersona string) string {
	if isBotSeat(seatUID, seatUserID) {
		return view.BotSeatLabel(botPersona)
	}
	if seatUsername != "" {
		return seatUsername
	}
	if viewerUID != "" && seatUID != "" && seatUID == viewerUID {
		return "You"
	}
	return "Anonymous"
}

// seatGlyph is a bot seat's difficulty-persona piece glyph (the archive clock's
// bot avatar), or "" for a human seat.
func seatGlyph(isBot bool, botPersona string) string {
	if !isBot {
		return ""
	}
	return view.BotSeatGlyph(botPersona)
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
// histories, seat uids) for an archived game. Per-ply timing comes from the
// moves table (the packed games.moves blob carries none); games archived
// before timing was recorded simply omit the arrays.
func boardPayload(g gen.Game, og *game.OctadGame) proto.MovePayload {
	ofens := og.OFENHistory()
	payload := proto.MovePayload{
		OFEN:  ofens[len(ofens)-1],
		Moves: og.MoveHistory(),
		SANs:  og.SANHistory(),
		OFENs: ofens,
		White: g.WhiteUid,
		Black: g.BlackUid,
	}
	times, err := db.ListGameMoveTimes(g.ID)
	if err != nil {
		// degrade to an untimed payload; the archive view works without timing
		util.Error(str.CRoom, "archive move times lookup failed game=%s: %s",
			g.GameID.String(), err.Error())
	} else if len(times) == len(payload.Moves) {
		payload.MoveTimes, payload.ClockTimes = game.TimingArrays(times)
	}
	return payload
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

	// cached per-ply engine evals drive the archive eval bar. Only on this
	// server-rendered page payload (always fresh as the evaluator fills the
	// cache) — never on the immutable-cacheable game JSON endpoint. Nil when
	// nothing is evaluated yet; the client renders no bar then.
	if evals := db.ListGameMoveEvals(selected.ID); len(evals) == len(data.Board.Moves) {
		data.Board.Evals = evals
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

// derefStr / derefInt16 unwrap nullable archive columns to their zero value.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt16(p *int16) int {
	if p == nil {
		return 0
	}
	return int(*p)
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
