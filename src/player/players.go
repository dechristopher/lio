package player

import (
	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// Players map for use anywhere two players compete
type Players map[octad.Color]*Player

// FlipColor flips which color the players are playing
func (p Players) FlipColor() {
	white := p[octad.White]
	p[octad.White] = p[octad.Black]
	p[octad.Black] = white
}

// HasTwoPlayers returns true if both players are configured, and
// the color of the missing player if only one player is missing
func (p Players) HasTwoPlayers() (hasTwo bool, missing octad.Color) {
	hasTwo = util.BothColors(func(color octad.Color) bool {
		return p[color].ID != ""
	})

	if p[octad.White].ID == "" && !p[octad.White].IsBot {
		missing = octad.White
	} else if p[octad.Black].ID == "" && !p[octad.Black].IsBot {
		missing = octad.Black
	} else {
		missing = octad.NoColor
	}

	return
}

// IsPlayer returns true if the given ID belongs to a player in this match
func (p Players) IsPlayer(id string) bool {
	return p[octad.White].ID == id || p[octad.Black].ID == id
}

// Lookup player by id and return the player instance and the color
func (p Players) Lookup(id string) (*Player, octad.Color) {
	if p[octad.White].ID == id {
		return p[octad.White], octad.White
	}
	if p[octad.Black].ID == id {
		return p[octad.Black], octad.Black
	}
	return nil, octad.NoColor
}

// ScoreWin tracks a win for the given color and records the finished game in
// both players' match histories with the given method code
func (p Players) ScoreWin(color octad.Color, reason string) {
	p[color].scorePoints++
	p[color].results = append(p[color].results,
		GameResult{Points: 1, Color: color, Reason: reason})
	loser := color.Other()
	p[loser].results = append(p[loser].results,
		GameResult{Points: 0, Color: loser, Reason: reason})
}

// ScoreDraw tracks a draw (1/2 point) for both players and records the
// finished game in both players' match histories with the given method code
func (p Players) ScoreDraw(reason string) {
	util.DoBothColors(func(c octad.Color) {
		p[c].scoreHalf++
		p[c].results = append(p[c].results,
			GameResult{Points: 0.5, Color: c, Reason: reason})
	})
}

// ScoreMap returns a compatible ScorePayload map of the current player scores
func (p Players) ScoreMap() proto.ScorePayload {
	return proto.ScorePayload{
		octad.White.String(): p[octad.White].Score(),
		octad.Black.String(): p[octad.Black].Score(),
	}
}

// MatchHistory returns the per-game history of the room's match keyed by the
// players' current seats, following the ScorePayload convention: entry i's
// White/Black are the points earned in game i+1 by the players *now* seated
// white/black, however many color swaps ago that game was played. WhitePlayed
// carries the color the currently-white player actually held in that game so
// clients can show the side alternation.
func (p Players) MatchHistory() proto.MatchHistoryPayload {
	w, b := p[octad.White].Results(), p[octad.Black].Results()

	// the slices are appended in lockstep by ScoreWin/ScoreDraw; guard anyway
	n := min(len(w), len(b))

	hist := make(proto.MatchHistoryPayload, 0, n)
	for i := 0; i < n; i++ {
		hist = append(hist, proto.GameHistoryEntry{
			White:       w[i].Points,
			Black:       b[i].Points,
			Reason:      w[i].Reason,
			WhitePlayed: w[i].Color.String(),
		})
	}
	return hist
}

// AnchorColor returns a stable color to orient a spectator/TV board to, so that
// a given player keeps the same side of the board across the color flips that
// happen between games of a match. Because the score and identity travel with
// the *Player through FlipColor (which only swaps the map pointers), anchoring
// on a color-independent identity pins that player — and their score — to one
// side while the board itself flips to reveal who now has white.
//
// The human anchors (at the bottom) in bot games; otherwise the player whose ID
// sorts first anchors, which is deterministic and survives flips.
func (p Players) AnchorColor() octad.Color {
	if bot := p.GetBotColor(); bot != octad.NoColor {
		return bot.Other()
	}
	if p[octad.White] != nil && p[octad.Black] != nil &&
		p[octad.Black].ID != "" && p[octad.Black].ID < p[octad.White].ID {
		return octad.Black
	}
	return octad.White
}

// GetBotColor returns the current color the bot is playing
func (p Players) GetBotColor() octad.Color {
	if p[octad.White] != nil && p[octad.White].IsBot {
		return octad.White
	}
	if p[octad.Black] != nil && p[octad.Black].IsBot {
		return octad.Black
	}
	return octad.NoColor
}

// HasBot returns true if either player is configured to be a bot
func (p Players) HasBot() bool {
	return p.GetBotColor() != octad.NoColor
}
