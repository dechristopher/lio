package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dechristopher/octad"
	"github.com/valyala/fastjson"

	"github.com/dechristopher/lioctad/bus"
	"github.com/dechristopher/lioctad/engine"
	"github.com/dechristopher/lioctad/game"
	"github.com/dechristopher/lioctad/store"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/lioctad/variant"
	"github.com/dechristopher/lioctad/www/ws/common"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

var pub = bus.NewPublisher("game", game.Channel)

// HandleMove processes game update messages
func HandleMove(m []byte, meta common.SocketContext) []byte {
	if game.Games[meta.Channel] == nil {
		_, err := game.NewOctadGame(game.OctadGameConfig{
			White:   "123",
			Black:   "456",
			Control: variant.HalfOneBlitzTC,
			Channel: meta.Channel,
		})

		if err != nil {
			panic(err)
		}
	}

	g := game.Games[meta.Channel]

	// quickly return board state on new connection
	if fastjson.GetInt(m, "d", "a") == 0 {
		return current(g, true)
	}

	var msg proto.MessageMove
	err := json.Unmarshal(m, &msg)
	if err != nil {
		util.Error(str.CHMov, str.EMoveUnmarshal, m, err)
		return nil
	}

	move := msg.Data
	ok := false

	// make move if possible and make subsequent computer
	// move some time after if applicable
	for _, mov := range g.Game.ValidMoves() {
		if mov.String() == move.UOI {
			err := g.Game.Move(mov)
			if err != nil {
				// bad if this happens
				return nil
			}

			eval := engine.Evaluate(g.Game)

			// publish move to broadcast channel
			pub.Publish(mov.String(), g.Game.OFEN(), eval)

			util.DebugFlag("eng", str.CHMov, "player move eval: %2f",
				eval)

			ok = true
			go makeComputerMove(g, meta)
			break
		}
	}

	// if no move or illegal move provided, return to
	// current position and wait for another move
	if !ok {
		return current(g, false)
	}

	g.ToMove = g.Game.Position().Turn()

	// check to see if the game is over
	checkGameOver(g, meta)

	// broadcast move to everyone and send response back to player
	common.BroadcastEx(current(g, true), meta)
	return current(g, true)
}

func current(g *game.OctadGame, addLast bool) []byte {
	curr := proto.MovePayload{
		Clock:      currentClock(g),
		OFEN:       g.Game.OFEN(),
		SAN:        getSAN(g, addLast),
		MoveNum:    len(g.Game.Moves()) / 2,
		Check:      g.Game.Position().InCheck(),
		Moves:      g.AllMoves(),
		ValidMoves: g.LegalMoves(),
		Latency:    0,
	}
	return curr.Marshal()
}

func currentClock(g *game.OctadGame) proto.ClockPayload {
	state := g.Clock.State()
	return proto.ClockPayload{
		Black: state.BlackTime.Centi(),
		White: state.WhiteTime.Centi(),
		Lag:   0,
	}
}

func makeComputerMove(g *game.OctadGame, meta common.SocketContext) {
	if g.Game.Outcome() == octad.NoOutcome {
		if len(g.Game.ValidMoves()) > 0 {
			searchMove := engine.Search(g.Game.Position().String(),
				5, engine.MinimaxAB)
			err := g.Game.Move(&searchMove.Move)
			if err != nil {
				// this means there is a bug in either
				// the engine or in the octad lib
				panic(err)
			}

			eval := engine.Evaluate(g.Game)

			// publish move to broadcast channel
			pub.Publish(searchMove.Move.String(), g.Game.OFEN(), eval)

			util.DebugFlag("eng", str.CHMov, "engine eval: %s (%2f)",
				searchMove.Move.String(), searchMove.Eval)
			util.DebugFlag("eng", str.CHMov, "computer move eval: %2f",
				eval)

			// broadcast move to all players
			common.Broadcast(current(g, true), meta)

			// check to see if the game is over
			checkGameOver(g, meta)
		}
	}
}

// getSAN returns the last move in algebraic notation
func getSAN(g *game.OctadGame, calc bool) string {
	if !calc {
		return ""
	}
	if len(g.Game.Positions()) > 1 {
		pos := g.Game.Positions()[len(g.Game.Positions())-2]
		move := g.Game.Moves()[len(g.Game.Moves())-1]
		return octad.AlgebraicNotation{}.Encode(pos, move)
	}

	return ""
}

func checkGameOver(g *game.OctadGame, meta common.SocketContext) {
	// restart game if over
	if g.Game.Outcome() != octad.NoOutcome {
		// record game result
		gcp := *g
		go recordGame(gcp)
		// broadcast game over message
		common.Broadcast(gameOverMessage(g), meta)
		// set up the board and broadcast state
		go func() {
			t := time.NewTimer(time.Second * 2)
			<-t.C
			g, _ = game.NewOctadGame(game.OctadGameConfig{
				White:   "123",
				Black:   "456",
				Control: variant.HalfOneBlitzTC,
				OFEN:    "",
				Channel: meta.Channel,
			})
			common.Broadcast(current(g, false), meta)
		}()
	}
}

func gameOverMessage(g *game.OctadGame) []byte {
	id, status := genGameState(g)
	gameOver := proto.GameOverPayload{
		Winner:   getWinnerString(id),
		StatusID: id,
		Status:   status,
		Clock:    currentClock(g),
	}
	return gameOver.Marshal()
}

func genGameState(g *game.OctadGame) (int, string) {
	switch g.Game.Outcome() {
	case octad.NoOutcome:
		return 0, "FREE, ONLINE OCTAD COMING SOON!"
	case octad.Draw:
		return genDrawState(g)
	case octad.WhiteWon:
		return genWhiteWinState(g)
	default:
		return genBlackWinState(g)
	}
}

func genDrawState(g *game.OctadGame) (int, string) {
	switch g.Game.Method() {
	case octad.InsufficientMaterial:
		return 3, "DRAWN DUE TO INSUFFICIENT MATERIAL."
	case octad.Stalemate:
		return 4, "DRAWN BY STALEMATE."
	case octad.DrawOffer:
		return 5, "DRAWN BY AGREEMENT"
	case octad.ThreefoldRepetition:
		return 6, "DRAWN BY REPETITION"
	case octad.TwentyFiveMoveRule:
		return 11, "DRAWN DUE TO 25 MOVE RULE"
	default:
		return -1, ""
	}
}

func genWhiteWinState(g *game.OctadGame) (int, string) {
	switch g.Game.Method() {
	case octad.Checkmate:
		return 1, "WHITE WINS BY CHECKMATE"
	case octad.Resignation:
		return 7, "BLACK RESIGNED, WHITE IS VICTORIOUS"
	}
	return -1, ""
}

func genBlackWinState(g *game.OctadGame) (int, string) {
	switch g.Game.Method() {
	case octad.Checkmate:
		return 2, "BLACK WINS BY CHECKMATE"
	case octad.Resignation:
		return 8, "WHITE RESIGNED, BLACK IS VICTORIOUS"
	}
	return -1, ""
}

func getWinnerString(statusId int) string {
	switch statusId {
	case 1, 7:
		return "w"
	case 2, 8:
		return "b"
	}
	return "d"
}

func recordGame(g game.OctadGame) {
	pgn := g.Game.String()
	parts := strings.Split(pgn, " ")

	full := "[Event \"Lioctad Test Match\"]\n" +
		"[Site \"https://lioctad.org\"]\n" +
		"[Date \"" + g.Start.Format("2006.01.02") + "\"]\n" +
		"[Round \"1\"]\n" +
		"[White \"Lioctad Test Players\"]\n" +
		"[Black \"Lioctad Random Computer\"]\n" +
		"[Result \"" + parts[len(parts)-1] + "\"]\n" +
		"[Time \"" + g.Start.Format("15:04:05") + "\"]" + pgn

	util.Debug("PGN", full)

	// year/month/day/HH:MM:SSTZ-(inserted-time-unix).pgn
	key := fmt.Sprintf("%s/%s/%s/%s-%d.pgn",
		g.Start.Format("2006"),
		g.Start.Format("01"),
		g.Start.Format("02"),
		g.Start.Format("15:04:05Z07:00"),
		time.Now().UnixNano())

	err := store.PutObject(store.PGNBucket, key, []byte(full))
	if err != nil {
		util.Error(str.CHMov, str.ERecord, err.Error())
	}
}
