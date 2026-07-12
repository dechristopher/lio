package game

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/google/uuid"

	"github.com/dechristopher/lio/bus"
	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/variant"
)

// Channel is the engine monitoring bus channel
const Channel bus.Channel = "lio:game"

// OctadGame wraps octad game and clock
type OctadGame struct {
	octad.Game
	ID      string          `json:"i"` // game id
	Start   time.Time       `json:"t"` // game start time
	White   string          `json:"w"` // white userid
	Black   string          `json:"b"` // black userid
	ToMove  octad.Color     `json:"m"` // color to move
	Variant variant.Variant // octad variant
	Clock   *clock.Clock    // clock instance
}

// NewOctadGame returns a new OctadGame instance from the given configuration
func NewOctadGame(config OctadGameConfig) (*OctadGame, error) {
	game, err := genGame(config.OFEN)
	if err != nil {
		return nil, err
	}

	g := OctadGame{
		ID:      uuid.NewString(),
		Start:   time.Now(),
		Game:    *game,
		ToMove:  game.Position().Turn(),
		Variant: config.Variant,
		Clock:   clock.NewClock(config.Variant.Control),
		White:   config.White,
		Black:   config.Black,
	}

	return &g, nil
}

// RestoreOctadGame rebuilds a persisted game by replaying its move list from
// its starting OFEN (which differs from the standard start for deploy-mode
// games), preserving the original game identity and seat assignment. The clock
// is supplied by the caller (restored separately — see clock.Restore) rather
// than created fresh like NewOctadGame does. Board-derived outcomes
// (checkmate, stalemate, repetition, ...) re-arise from the replay itself;
// declared outcomes (resignation, agreed draw) are the caller's to re-apply.
func RestoreOctadGame(config OctadGameConfig, id string, start time.Time,
	startOFEN string, moves []string, clk *clock.Clock) (*OctadGame, error) {
	game, err := genGame(startOFEN)
	if err != nil {
		return nil, err
	}

	for i, uoi := range moves {
		var match *octad.Move
		for _, m := range game.ValidMoves() {
			if m.String() == uoi {
				match = m
				break
			}
		}
		if match == nil {
			return nil, fmt.Errorf("restore: illegal move %q at ply %d", uoi, i+1)
		}
		if err := game.Move(match); err != nil {
			return nil, fmt.Errorf("restore: move %q at ply %d: %w", uoi, i+1, err)
		}
	}

	g := OctadGame{
		ID:      id,
		Start:   start,
		Game:    *game,
		ToMove:  game.Position().Turn(),
		Variant: config.Variant,
		Clock:   clk,
		White:   config.White,
		Black:   config.Black,
	}

	return &g, nil
}

// LegalMoves returns all legal moves in a map of origin square
// to all legal destination squares
func (g *OctadGame) LegalMoves() map[string][]string {
	legalMoves := make(map[string][]string)
	for _, move := range g.Game.ValidMoves() {
		if legalMoves[move.S1().String()] == nil {
			legalMoves[move.S1().String()] = make([]string, 0)
		}
		legalMoves[move.S1().String()] =
			append(legalMoves[move.S1().String()], move.S2().String())
	}
	return legalMoves
}

// LegalMovesJSON returns a json formatted array of all legal moves
func (g *OctadGame) LegalMovesJSON() string {
	moves := g.LegalMoves()
	j, err := json.Marshal(moves)
	if err != nil {
		return ""
	}
	return string(j)
}

// MoveHistory returns a list of all moves to have
// happened in the game so far
func (g *OctadGame) MoveHistory() []string {
	moves := g.Game.Moves()
	allMoves := make([]string, 0)
	for _, move := range moves {
		allMoves = append(allMoves, move.String())
	}
	return allMoves
}

// SANHistory returns the algebraic notation (SAN) for every move played so far,
// in order and parallel to MoveHistory. Each SAN is encoded against the position
// as it stood before that move, mirroring the room's single-move getSANLocked.
func (g *OctadGame) SANHistory() []string {
	positions := g.Game.Positions()
	moves := g.Game.Moves()
	sans := make([]string, 0, len(moves))
	for i, move := range moves {
		sans = append(sans, octad.AlgebraicNotation{}.Encode(positions[i], move))
	}
	return sans
}

// OFENHistory returns the OFEN of every position the game has passed through:
// index 0 is the starting position and index i is the position after ply i, so
// its length is one greater than the move count. This lets clients render the
// board at any ply without an octad rules engine of their own.
func (g *OctadGame) OFENHistory() []string {
	positions := g.Game.Positions()
	ofens := make([]string, 0, len(positions))
	for _, pos := range positions {
		ofens = append(ofens, pos.String())
	}
	return ofens
}

// genGame creates a new game, optionally from an ofen
func genGame(ofen ...string) (*octad.Game, error) {
	if ofen[0] != "" {
		fromPos, err := octad.OFEN(ofen[0])
		if err != nil {
			return nil, err
		}
		return octad.NewGame(fromPos)
	}

	return octad.NewGame()
}
