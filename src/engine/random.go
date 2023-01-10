package engine

import (
	"math/rand"
	"time"

	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/octad"
)

// randomMove is the root for the random move generator
func randomMove(situation *octad.Game) MoveEval {
	// sleep for a random amount of time to make the engine easier to beat,
	// anywhere from .25 seconds to 3 seconds
	min := time.Millisecond * 250
	max := time.Millisecond * 3000
	randomDuration := rand.Intn(int(max.Nanoseconds()-min.Nanoseconds()+1)) + int(min.Nanoseconds())
	time.Sleep(time.Duration(randomDuration))

	var bestMove MoveEval
	moves := orderMoves(situation)

	moveIndex := rand.Intn(len(moves))

	bestMove = MoveEval{
		Eval: 0,
		Move: moves[moveIndex],
	}
	util.DebugFlag("engine", str.CEval, "chose randomDuration move: %s (%2f) for OFEN: %s",
		bestMove.Move.String(), bestMove.Eval, situation.Position().String())

	return bestMove
}
