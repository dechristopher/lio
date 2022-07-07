package engine

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/dechristopher/lioctad/clock"
	"github.com/dechristopher/octad"
	"github.com/pkg/errors"

	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

type minimaxABParams struct {
	situation octad.Game
	move      octad.Move
	isWhite   bool
	depth     int
	evalChan  chan MoveEval
	wg        *sync.WaitGroup
}

// searchMinimaxAB is the root for minimax with alpha-beta pruning
func searchMinimaxAB(situation *octad.Game, depth int) MoveEval {
	// sleep for a random amount of time to make the engine easier to beat,
	// anywhere from a fraction of a second to 1.25 seconds
	time.Sleep(clock.Centi * 5 * time.Duration(rand.Intn(25)))

	isWhite := situation.Position().Turn() == octad.White
	bestMoveEval := math.Inf(1)
	if isWhite {
		bestMoveEval = math.Inf(-1)
	}

	var bestMove MoveEval
	moves := orderMoves(situation)

	evaluations := make(chan MoveEval, len(moves))
	wg := &sync.WaitGroup{}

	// run search for each legal move in parallel
	for _, move := range moves {
		wg.Add(1)
		go minimaxABAsync(minimaxABParams{
			situation: *situation,
			move:      move,
			isWhite:   isWhite,
			depth:     depth,
			evalChan:  evaluations,
			wg:        wg,
		})
	}

	// wait for evaluation routines to finish
	go func() {
		wg.Wait()
		close(evaluations)
	}()

	for evaluation := range evaluations {
		if (isWhite && evaluation.Eval > bestMoveEval) ||
			(!isWhite && evaluation.Eval < bestMoveEval) {
			bestMoveEval = evaluation.Eval
			bestMove = evaluation
		}
	}

	// pick first legal move if no move found better than
	// the completely losing default evaluation
	if bestMove.Move.String() == "a1a1" {
		bestMove.Move = moves[0]
		bestMove.Eval = bestMoveEval
	}

	util.DebugFlag("eng", str.CEval, "chose best move: %s (%2f) for OFEN: %s",
		bestMove.Move.String(), bestMove.Eval, situation.Position().String())

	return bestMove
}

// minimaxABAsync is a parallel wrapper for minimaxAB
func minimaxABAsync(params minimaxABParams) {
	defer params.wg.Done()

	err := params.situation.Move(&params.move)
	if err != nil {
		panic(errors.WithMessagef(err,
			"pos: %+v, move: %+v", params.situation, params.move))
	}

	eval := minimaxAB(&params.situation, &params.move, !params.isWhite, params.depth)

	util.DebugFlag("eng", str.CEval, "root eval: %s (%2f)",
		params.move.String(), eval)

	params.evalChan <- MoveEval{
		Eval: eval,
		Move: params.move,
	}
}

// minimaxAB is a recursive minimax search implementation that
// uses alpha-beta pruning to perform search tree cutting and
// subsequently improve the maximum depth we can search in a
// reasonable amount of time
func minimaxAB(
	node *octad.Game,
	move *octad.Move,
	isMaxi bool,
	depth int,
) float64 {
	if isMaxi {
		return mmABMax(node, move, depth, math.Inf(-1), math.Inf(1))
	}
	return mmABMin(node, move, depth, math.Inf(-1), math.Inf(1))
}

// mmABMax is the maximizing routine for minimax with alpha-beta pruning
func mmABMax(node *octad.Game, lastMove *octad.Move, depth int, alpha, beta float64) float64 {
	moves := node.ValidMoves()

	if depth == 0 || len(moves) == 0 {
		eval := Evaluate(node)
		util.DebugFlag("eng-v", str.CEval, "minimax: d0: MAX move=%s eval=%2f",
			lastMove.String(), eval)
		return eval
	}

	// perform calculations as white (maximizing player)
	for _, move := range moves {
		err := node.Move(move)
		if err != nil {
			panic(errors.WithMessagef(err,
				"pos: %+v, move: %+v", node, move))
		}

		eval := mmABMin(node, move, depth-1, alpha, beta)
		node.UndoMove()

		util.DebugFlag("eng-v", str.CEval, "minimax: d%d: MAX move=%s eval=%2f",
			depth, move.String(), eval)

		if eval >= beta {
			return beta
		}
		if eval > alpha {
			alpha = eval
		}
	}

	util.DebugFlag("eng-v", str.CEval, "minimax: d%d: MAX best eval=%2f",
		depth, alpha)

	return alpha
}

// mmABMax is the minimizing routine for minimax with alpha-beta pruning
func mmABMin(node *octad.Game, lastMove *octad.Move, depth int, alpha, beta float64) float64 {
	moves := node.ValidMoves()

	if depth == 0 || len(moves) == 0 {
		eval := -Evaluate(node)
		util.DebugFlag("eng-v", str.CEval, "minimax: d0: MIN move=%s eval=%2f",
			lastMove.String(), eval)
		return eval
	}

	// perform calculations as black (minimizing player)
	for _, move := range moves {
		err := node.Move(move)
		if err != nil {
			panic(errors.WithMessagef(err,
				"pos: %+v, move: %+v", node, move))
		}

		eval := mmABMax(node, move, depth-1, alpha, beta)
		node.UndoMove()

		util.DebugFlag("eng-v", str.CEval, "minimax: d%d: MIN move=%s eval=%2f",
			depth, move.String(), eval)

		if eval <= alpha {
			return alpha
		}
		if eval < beta {
			beta = eval
		}
	}

	util.DebugFlag("eng-v", str.CEval, "minimax: d%d: MIN best eval=%2f",
		depth, beta)

	return beta
}
