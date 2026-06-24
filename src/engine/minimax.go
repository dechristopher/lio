package engine

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dechristopher/octad"
	"github.com/pkg/errors"

	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/rng"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// OpeningVarietyMoves is the number of top root moves the engine randomly
// chooses among on the first move of the game, so it doesn't play the same
// opening (e.g. P-c2 as white) every single time. A small top-N pick stays
// robust across search depths, where the eval gap between good first moves
// varies (and oscillates odd/even) far more than the move ordering does.
const OpeningVarietyMoves = 3

// OpeningVarietyMargin caps how far (in eval units, where a pawn is worth 10) a
// candidate opening move may trail the best move, so a varied opening choice is
// never an outright blunder even if fewer than OpeningVarietyMoves are sound.
const OpeningVarietyMargin float64 = 25

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
	time.Sleep(clock.Centisecond * 5 * time.Duration(rng.Intn(25)))

	// add a little opening variety: on the first move of the game the engine
	// otherwise always plays its single best move (e.g. P-c2 as white), which
	// gets repetitive to play against. Pick randomly among the near-best
	// opening moves instead. Later moves always take the single best move.
	if isOpeningPosition(situation) {
		return pickOpeningMove(situation, depth)
	}

	return minimaxABRoot(situation, depth)
}

// minimaxABRoot runs the parallel alpha-beta root search and returns the single
// best move. It is the pure, side-effect-free core of searchMinimaxAB (no
// handicap sleep) so it can be exercised directly by tests.
func minimaxABRoot(situation *octad.Game, depth int) MoveEval {
	isWhite := situation.Position().Turn() == octad.White
	bestMoveEval := WinVal
	if isWhite {
		bestMoveEval = -WinVal
	}

	var bestMove MoveEval
	moves := orderMoves(situation)
	results := evaluateRootMoves(situation, moves, depth)

	for _, evaluation := range results {
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

	util.DebugFlag("engine", str.CEval, "chose best move: %s (%2f) for OFEN: %s",
		bestMove.Move.String(), bestMove.Eval, situation.Position().String())

	return bestMove
}

// evaluateRootMoves runs the parallel alpha-beta search for every legal root
// move and returns each move paired with its absolute white-positive eval. It
// is the shared core of minimaxABRoot (which keeps the single best move) and
// pickOpeningMove (which keeps a random near-best move). Callers must pass the
// orderMoves result for situation so the root validMoves cache is warmed before
// the per-goroutine value-copies fan out (see the concurrency note in CLAUDE.md).
func evaluateRootMoves(situation *octad.Game, moves []octad.Move, depth int) []MoveEval {
	isWhite := situation.Position().Turn() == octad.White

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

	results := make([]MoveEval, 0, len(moves))
	for evaluation := range evaluations {
		results = append(results, evaluation)
	}

	return results
}

// isOpeningPosition reports whether situation is on the first full move of the
// game (white's first move or black's first reply). moveCount is the trailing
// field of the OFEN; it starts at 1 and only increments after black moves.
func isOpeningPosition(situation *octad.Game) bool {
	fields := strings.Fields(situation.Position().String())
	return len(fields) == 6 && fields[5] == "1"
}

// pickOpeningMove searches all root moves and returns a random pick from the
// top OpeningVarietyMoves, dropping any that trail the best move by more than
// OpeningVarietyMargin. The best move always qualifies, so a candidate is
// always returned; positions with a single sensible move simply play it.
func pickOpeningMove(situation *octad.Game, depth int) MoveEval {
	isWhite := situation.Position().Turn() == octad.White
	moves := orderMoves(situation)
	results := evaluateRootMoves(situation, moves, depth)
	if len(results) == 0 {
		// no moves searched (shouldn't happen for a live position); defer to
		// the standard best-move logic and its losing-position fallback
		return minimaxABRoot(situation, depth)
	}

	// sort best-first from the side-to-move's perspective
	sort.SliceStable(results, func(i, j int) bool {
		if isWhite {
			return results[i].Eval > results[j].Eval
		}
		return results[i].Eval < results[j].Eval
	})
	best := results[0].Eval

	// keep the top OpeningVarietyMoves, dropping any beyond the safety margin;
	// results are sorted, so we can stop at the first one that falls short
	candidates := make([]MoveEval, 0, OpeningVarietyMoves)
	for _, r := range results {
		if len(candidates) == OpeningVarietyMoves {
			break
		}
		within := (isWhite && r.Eval >= best-OpeningVarietyMargin) ||
			(!isWhite && r.Eval <= best+OpeningVarietyMargin)
		if !within {
			break
		}
		candidates = append(candidates, r)
	}

	choice := candidates[rng.Intn(len(candidates))]

	util.DebugFlag("engine", str.CEval, "chose opening move: %s (%2f) from %d candidates for OFEN: %s",
		choice.Move.String(), choice.Eval, len(candidates), situation.Position().String())

	return choice
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

	util.DebugFlag("engine", str.CEval, "root eval: %s (%2f)",
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
		return mmABMax(node, move, depth, -WinVal, WinVal)
	}
	return mmABMin(node, move, depth, -WinVal, WinVal)
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
