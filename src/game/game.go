package game

import (
	"encoding/json"
	wsv1 "github.com/dechristopher/lio/proto"
	"time"

	"github.com/dechristopher/octad"
	"github.com/google/uuid"

	"github.com/dechristopher/lio/bus"
	"github.com/dechristopher/lio/clock"
)

// Channel is the engine monitoring bus channel
const Channel bus.Channel = "lio:game"

// OctadGame wraps octad game and clock
type OctadGame struct {
	octad.Game
	ID      string        `json:"i"` // game id
	Start   time.Time     `json:"t"` // game start time
	White   string        `json:"w"` // white userid
	Black   string        `json:"b"` // black userid
	ToMove  octad.Color   `json:"m"` // color to move
	Variant *wsv1.Variant // octad variant
	Clock   *clock.Clock  // clock instance
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

// LegalMoves returns all legal moves in a map of origin square
// to all legal destination squares
func (g *OctadGame) LegalMoves() map[string]*wsv1.Moves {
	legalMoves := make(map[string]*wsv1.Moves)
	for _, move := range g.Game.ValidMoves() {
		if legalMoves[move.S1().String()] == nil {
			legalMoves[move.S1().String()] = &wsv1.Moves{
				Moves: make([]string, 0),
			}
		}
		legalMoves[move.S1().String()].Moves =
			append(legalMoves[move.S1().String()].Moves, move.S2().String())
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
