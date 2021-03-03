package handlers

import (
	"math/rand"
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lioctad/clock"
	"github.com/dechristopher/lioctad/game"
	"github.com/dechristopher/lioctad/www/ws/common"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

// HandleGame processes game update messages
func HandleGame(m proto.Message) proto.Message {
	response := common.GenResponse(m)

	if game.Games[m.Channel] == nil {
		_, err := game.NewOctadGame(game.OctadGameConfig{
			White: "123",
			Black: "456",
			Control: clock.TimeControl{
				Time:      2,
				Increment: 1,
			},
			OFEN:    "",
			Channel: m.Channel,
		})

		if err != nil {
			panic(err)
		}
	}

	g := game.Games[m.Channel]

	if m.Body[0] == "0" {
		response.Body = genGameUpdate(g)
	} else if m.Body[0] == "1" {
		for _, move := range g.Game.ValidMoves() {
			if move.String() == m.Body[1] {
				err := g.Game.Move(move)
				if err != nil {
					response.Error = "001"
					break
				}
				if g.Game.Outcome() == octad.NoOutcome {
					bm := g.Game.ValidMoves()
					if len(bm) > 0 {
						r := rand.New(rand.NewSource(time.Now().UnixNano()))
						err = g.Game.Move(bm[r.Int31n(int32(len(bm)))])
						if err != nil {
							response.Error = "002"
						}
					}
				}
				break
			}
		}
		g.ToMove = g.Game.Position().Turn()
		response.Body = genGameUpdate(g)
		if genGameState(g) != "0" {
			go func() {
				t := time.NewTimer(time.Second * 1)
				<-t.C
				g, _ = game.NewOctadGame(game.OctadGameConfig{
					White: "123",
					Black: "456",
					Control: clock.TimeControl{
						Time:      2,
						Increment: 1,
					},
					OFEN:    "",
					Channel: m.Channel,
				})
			}()
		}
	}

	return response
}

func genGameUpdate(g *game.OctadGame) []string {
	g.Game.Position()
	return []string{
		genGameState(g),
		g.Game.Position().String(),
		g.Game.Position().Turn().String(),
		g.AllMovesJSON(),
		g.LegalMovesJSON(),
		isChecked(g),
	}
}

func isChecked(g *game.OctadGame) string {
	if g.Game.Position().InCheck() {
		return "1"
	}
	return "0"
}

func genGameState(g *game.OctadGame) string {
	switch g.Game.Outcome() {
	case octad.NoOutcome:
		return "0"
	case octad.Draw:
		return genDrawState(g)
	case octad.WhiteWon:
		return genWhiteWinState(g)
	default:
		return genBlackWinState(g)
	}
}

func genDrawState(g *game.OctadGame) string {
	switch g.Game.Method() {
	case octad.InsufficientMaterial:
		return "3"
	case octad.Stalemate:
		return "4"
	case octad.DrawOffer:
		return "5"
	case octad.ThreefoldRepetition:
		return "6"
	case octad.FiftyMoveRule:
		return "11"
	default:
		return ""
	}
}

func genWhiteWinState(g *game.OctadGame) string {
	switch g.Game.Method() {
	case octad.Checkmate:
		return "1"
	case octad.Resignation:
		return "7"
	}
	return ""
}

func genBlackWinState(g *game.OctadGame) string {
	switch g.Game.Method() {
	case octad.Checkmate:
		return "2"
	case octad.Resignation:
		return "8"
	}
	return ""
}
