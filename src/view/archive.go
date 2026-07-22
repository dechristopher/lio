package view

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/www/ws/proto"
)

// ArchiveData is the inline JSON payload (templ.JSONScript, #archive-data) the
// archived room/game page hands to lio-game.js. Its presence on the page is
// what switches the client into archive mode: no socket, board + move list +
// timeline hydrated entirely from this data. The board/score/history blocks
// reuse the live wire payload types so the JSON key names ("m"/"sm"/"om",
// "sc", "h", ...) can never drift from what the client's render paths expect.
type ArchiveData struct {
	// RoomID is empty for a standalone game view (a room-less backfilled game).
	RoomID string `json:"roomId,omitempty"`
	GameID string `json:"gameId"`
	// N is the selected game's 1-based match ordinal; Count the match's total.
	N     int `json:"n"`
	Count int `json:"count"`
	// Board mirrors a live MovePayload: final OFEN (o), per-ply UOI/SAN/OFEN
	// histories (m/sm/om), and seat uids (w/b) for the selected game.
	Board proto.MovePayload `json:"board"`
	// Score is the final cumulative match score and History the per-game match
	// results, both keyed by the SELECTED game's seats (the client maps rows
	// via its playerWhite orientation, exactly like the live convention of
	// keying by current seats).
	Score   proto.ScorePayload        `json:"sc,omitempty"`
	History proto.MatchHistoryPayload `json:"h,omitempty"`
	// Winner ("w"/"b"/"d") and Reason (short method code) mirror the
	// GameOverPayload fields the client's result rendering reads.
	Winner string `json:"w"`
	Reason string `json:"r"`
	// Outcome is the PGN result token ("1-0", "0-1", "1/2-1/2") for copy-PGN.
	Outcome string `json:"outcome"`
	// PGN is the game's canonical PGN, rebuilt server-side from the archived row
	// (never fetched from the object store). The copy button copies it verbatim,
	// so a copied PGN is byte-for-byte what was archived.
	PGN string `json:"pgn,omitempty"`
}

// ArchiveGameData is the response of the archived-game JSON endpoint
// (GET /api/room/:id/game/:num) that powers in-room game browsing: while a
// room is live and in a post-game lull, clicking a finished game in the match
// timeline fetches this and swaps the client's review state in place — no
// navigation, so the socket and rematch window survive. Deliberately
// viewer-independent (orientation is resolved client-side from the seat uids)
// and free of match-cumulative fields (score/history/count), so the response
// for a given game is immutable and publicly cacheable forever.
type ArchiveGameData struct {
	RoomID string `json:"roomId"`
	GameID string `json:"gameId"`
	N      int    `json:"n"`
	// Board mirrors a live MovePayload: final OFEN (o), per-ply UOI/SAN/OFEN
	// histories (m/sm/om), and seat uids (w/b).
	Board   proto.MovePayload `json:"board"`
	Winner  string            `json:"w"`
	Reason  string            `json:"r"`
	Outcome string            `json:"outcome"`
	// PGN is the browsed game's canonical PGN, rebuilt from its archived row, so
	// the copy button copies what was archived even for a non-live game of the
	// match. Immutable like the rest of this cacheable response.
	PGN string `json:"pgn,omitempty"`
}

// ArchiveModel is everything the archived room/game page renders server-side:
// the static labels and chrome around the board, plus the ArchiveData blob the
// client hydrates from.
type ArchiveModel struct {
	RoomID      string
	VariantName string
	// VariantGroup is the variant's speed group name ("blitz", ...).
	VariantGroup string
	Casual       bool
	RaceTo       int
	N            int
	Count        int
	// Standalone marks a room-less single-game view (backfilled archives):
	// no match timeline, no room permalink context.
	Standalone bool
	// Orientation is the board-orientation class ("w"/"b"): the viewer's own
	// color when they played in this game, else white.
	Orientation string
	// Top/Bottom label the timeline rows (bottom is the seat the board is
	// oriented to): "You"/"Opponent" for a returning participant, "BOT" for
	// the engine, "PLAYER" otherwise.
	TopName    string
	BottomName string
	// Top/BottomTitle are each account seat's optional display title ("GM", …),
	// rendered to the left of the name in the theme accent color. Empty for
	// anon/bot seats and untitled accounts.
	TopTitle    string
	BottomTitle string
	// Top/BottomRating are each seat's rating "at the time of this game" (the
	// display it held going into the game, "1650"/"1500?"), and Top/BottomRatingDelta
	// the signed change that game applied (+8/-8). Empty/zero for casual/anon/bot
	// games and rows archived before ratings were tracked — the clock then shows
	// no rating (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5).
	TopRating         string
	BottomRating      string
	TopRatingDelta    int
	BottomRatingDelta int
	// Top/BottomIsBot mark the engine's seat (no uid and no account) so the
	// archive clock cards can show the bot avatar exactly like the live page.
	TopIsBot    bool
	BottomIsBot bool
	// Top/BottomGlyph are the bot seat's difficulty-persona piece glyph, the
	// clock's bot avatar (BotSeatGlyph). Empty for a human seat.
	TopGlyph    string
	BottomGlyph string
	// Top/BottomH2H are the two seats' all-time head-to-head score (win = 1,
	// draw = ½) against each other, shown beside the timeline names with the
	// leader greened. H2HShow gates it: set only when both seats are distinct
	// accounts with at least one game on record together.
	TopH2H    float64
	BottomH2H float64
	H2HShow   bool
	// TCCenti is the game's full starting clock budget in centiseconds,
	// resolved from the variant registry (the archived row stores only the
	// variant's display name). It renders into the board's data-tc — the
	// authoritative "full budget" for the archive clock cards; the per-ply
	// clock history alone cannot recover it when a deploy pre-start expiry
	// charged the first move. 0 when the variant no longer resolves (the
	// client then falls back to the ply-1 clock value).
	TCCenti int64
	// EndedDate is the selected game's end date for the info line.
	EndedDate string
	// Matchup is this game's opening name (the White-vs-Black formation clash,
	// e.g. "Tidal Siege"); Bottom/TopFormation are the two sides' formation
	// names ("The Standard", ...) oriented to the board's bottom/top seats. All
	// three are empty when the starting position doesn't resolve to a named
	// deploy (defensive; a real game always resolves).
	Matchup         string
	BottomFormation string
	TopFormation    string
	Data            ArchiveData
}

// archiveModeLabel mirrors the live rail's Casual/Competitive tag.
func archiveModeLabel(m ArchiveModel) string {
	if m.Casual {
		return "Casual"
	}
	return "Competitive"
}

// ArchiveMeta builds page metadata for an archived room/game permalink. The
// OG card reuses the room-card route (which falls back to the archive too) so
// shared links preview the final position.
func ArchiveMeta(m ArchiveModel) Meta {
	group := cases.Title(language.English).String(m.VariantGroup)
	mode := "competitive"
	if m.Casual {
		mode = "casual"
	}
	title := group + " (" + m.VariantName + ") " + mode + " octad • Archived match"

	meta := Meta{
		Version:     config.VersionString(),
		SiteURL:     config.SiteURL(),
		Title:       title + " • lioctad.org",
		OGTitle:     title,
		OGURL:       "https://lioctad.org/game/" + m.Data.GameID,
		OGImage:     "https://lioctad.org/og/default.png",
		Description: "Finished octad game — replay every move.",
	}
	if !m.Standalone {
		meta.OGURL = "https://lioctad.org/" + m.RoomID
		meta.OGImage = "https://lioctad.org/og/room/" + m.RoomID + ".png"
		meta.Description = "Finished octad match — replay every move."
	}
	return meta
}
