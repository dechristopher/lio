package engine

import (
	"math/rand"
	"time"

	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/octad"
)

// randomMove is the root for the random move generator
func randomMove(situation *octad.Game) MoveEval {
	time.Sleep(clock.Centisecond * 5 * time.Duration(rand.Intn(50)))

	var bestMove MoveEval
	moves := orderMoves(situation)

	moveIndex := rand.Intn(len(moves))

	bestMove = MoveEval{
		Eval: 0,
		Move: moves[moveIndex],
	}
	util.DebugFlag("engine", str.CEval, "chose random move: %s (%2f) for OFEN: %s",
		bestMove.Move.String(), bestMove.Eval, situation.Position().String())

	return bestMove
}
