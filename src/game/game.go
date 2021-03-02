package game

import (
	"encoding/json"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lioctad/clock"
)

var (
	// TODO persist in redis or something
	Games map[string]*OctadGame
)

// LiGame wraps octad game and clock
type OctadGame struct {
	ID     string       `json:"i"` // game id
	White  string       `json:"w"` // white userid
	Black  string       `json:"b"` // black userid
	ToMove octad.Color  `json:"m"` // color to move
	Game   *octad.Game  // game instance
	Clock  *clock.Clock // clock instance
}

// NewOctadGame returns a new OctadGame instance from the given configuration
func NewOctadGame(config OctadGameConfig) (*OctadGame, error) {
	game, err := genGame(config.OFEN)
	if err != nil {
		return nil, err
	}

	g := OctadGame{
		ID:    config.Channel,
		Game:  game,
		Clock: clock.NewClock(config.White, config.Black, config.Control),
	}

	// create map if not exists
	if Games == nil {
		Games = make(map[string]*OctadGame)
	}
	Games[g.ID] = &g

	return &g, nil
}

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

func (g *OctadGame) LegalMovesJSON() string {
	moves := g.LegalMoves()
	j, err := json.Marshal(moves)
	if err != nil {
		return ""
	}
	return string(j)
}

func (g *OctadGame) AllMovesJSON() string {
	moves := g.Game.Moves()
	allMoves := make([]string, 0)
	for _, move := range moves {
		allMoves = append(allMoves, move.String())
	}
	j, err := json.Marshal(allMoves)
	if err != nil {
		return ""
	}
	return string(j)
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

//cl := clock.NewClock("Andrew", "Mike", clock.TimeControl{
//	Time:      time.Second * 15,
//	Increment: time.Second * 3,
//	Delay:     time.Second * 5,
//})
//
//cl.Start()
//
//go func() {
//	for {
//		select {
//		case s := <-cl.StateChannel:
//			log.Printf("%v", s)
//		}
//	}
//}()
//
//r.Get("/flip", func(ctx *fiber.Ctx) error {
//	if !cl.Flagged() {
//		cl.ControlChannel <- clock.Flip
//	}
//	return ctx.Status(200).JSON(cl.State())
//})