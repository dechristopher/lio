package engine

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dechristopher/octad/v2"

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
	// stop aborts the search when set: every node returns immediately and the
	// iteration's results are discarded by the caller (see deepeningRoot)
	stop *atomic.Bool
	// repHist is the real game's position occurrence counts (see
	// RepetitionHistory); nil disables repetition scoring. It is shared
	// read-only across the root goroutines — each builds its own repTracker.
	repHist map[string]int
}

// noStop is a shared, never-set stop flag for searches without a deadline.
// It is only ever Load()ed, so sharing it across concurrent searches is safe.
var noStop = new(atomic.Bool)

// searchMinimaxAB is the root for minimax with alpha-beta pruning. A non-zero
// deadline bounds the search: it runs iterative deepening up to depth and
// returns the best move of the last fully completed depth, so the engine
// always answers in time instead of flagging on deep searches. repHist is the
// real game's position occurrence counts (nil = repetition-blind).
func searchMinimaxAB(situation *octad.Game, depth int, deadline time.Time, repHist map[string]int) MoveEval {
	// sleep for a random amount of time to make the engine easier to beat,
	// anywhere from a fraction of a second to 1.25 seconds — but never more
	// than a quarter of the remaining budget, so the handicap can't eat the
	// search time on a low clock
	sleep := clock.Centisecond * 5 * time.Duration(rng.Intn(25))
	if !deadline.IsZero() {
		if maxSleep := time.Until(deadline) / 4; sleep > maxSleep {
			sleep = maxSleep
		}
	}
	if sleep > 0 {
		time.Sleep(sleep)
	}

	// add a little opening variety: on the first move of the game the engine
	// otherwise always plays its single best move (e.g. P-c2 as white), which
	// gets repetitive to play against. Pick randomly among the near-best
	// opening moves instead. Later moves always take the single best move.
	if deadline.IsZero() {
		if isOpeningPosition(situation) {
			return pickOpeningMove(situation, depth, repHist)
		}
		return minimaxABRoot(situation, depth, repHist)
	}

	best, results := deepeningRoot(situation, depth, deadline, repHist)
	if isOpeningPosition(situation) && len(results) > 0 {
		return pickVariety(situation, results)
	}
	return best
}

// deepeningRoot runs the parallel root search with iterative deepening from
// depth 1 up to maxDepth, keeping the best move and full root results of the
// last depth that completed before the deadline. An iteration interrupted by
// the deadline is discarded outright: its aborted subtrees returned bogus
// bounds, so partial results can't be trusted. Depth 1 on a 4x4 board takes
// microseconds, so a move is effectively always found in budget; if even that
// is interrupted, a final depth-1 pass without a deadline guarantees one.
func deepeningRoot(situation *octad.Game, maxDepth int, deadline time.Time, repHist map[string]int) (MoveEval, []MoveEval) {
	isWhite := situation.Position().Turn() == octad.White
	moves := orderMoves(situation)

	var best MoveEval
	var results []MoveEval

	for depth := 1; depth <= maxDepth; depth++ {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		stop := new(atomic.Bool)
		timer := time.AfterFunc(remaining, func() { stop.Store(true) })
		iterStart := time.Now()
		iterResults := evaluateRootMoves(situation, moves, depth, stop, repHist)
		timer.Stop()

		if stop.Load() {
			break
		}

		results = iterResults
		best = bestOf(results, moves, isWhite)

		iterTime := time.Since(iterStart)
		util.DebugFlag("engine", str.CEval, "deepening: depth %d done in %s, best %s (%2f)",
			depth, iterTime, best.Move.String(), best.Eval)

		// each extra ply costs a multiple of the last: if the next iteration
		// can't plausibly finish in the time left, stop now instead of burning
		// the rest of the budget on a search we'd have to abandon anyway
		if time.Until(deadline) < 2*iterTime {
			break
		}
	}

	if len(results) == 0 {
		results = evaluateRootMoves(situation, moves, 1, noStop, repHist)
		best = bestOf(results, moves, isWhite)
	}

	return best, results
}

// minimaxABRoot runs the parallel alpha-beta root search and returns the single
// best move. It is the pure, side-effect-free core of searchMinimaxAB (no
// handicap sleep) so it can be exercised directly by tests. repHist carries the
// real game's position occurrence counts for repetition scoring (nil disables).
func minimaxABRoot(situation *octad.Game, depth int, repHist map[string]int) MoveEval {
	moves := orderMoves(situation)
	results := evaluateRootMoves(situation, moves, depth, noStop, repHist)
	bestMove := bestOf(results, moves, situation.Position().Turn() == octad.White)

	util.DebugFlag("engine", str.CEval, "chose best move: %s (%2f) for OFEN: %s",
		bestMove.Move.String(), bestMove.Eval, situation.Position().String())

	return bestMove
}

// bestOf returns the best root evaluation for the side to move, falling back
// to the first legal move if no move beat the completely losing default
// evaluation. An empty move list (a terminal position that slipped past
// Search's guard) yields the zero MoveEval rather than a panic.
func bestOf(results []MoveEval, moves []octad.Move, isWhite bool) MoveEval {
	if len(moves) == 0 {
		return MoveEval{}
	}
	bestMoveEval := WinVal
	if isWhite {
		bestMoveEval = -WinVal
	}

	var bestMove MoveEval
	for _, evaluation := range results {
		if (isWhite && evaluation.Eval > bestMoveEval) ||
			(!isWhite && evaluation.Eval < bestMoveEval) {
			bestMoveEval = evaluation.Eval
			bestMove = evaluation
		}
	}

	if bestMove.Move.String() == "a1a1" {
		bestMove.Move = moves[0]
		bestMove.Eval = bestMoveEval
	}

	return bestMove
}

// evaluateRootMoves runs the parallel alpha-beta search for every legal root
// move and returns each move paired with its absolute white-positive eval. It
// is the shared core of minimaxABRoot (which keeps the single best move) and
// pickOpeningMove (which keeps a random near-best move). Callers must pass the
// orderMoves result for situation so the root validMoves cache is warmed before
// the per-goroutine value-copies fan out (see the concurrency note in CLAUDE.md).
// Setting stop aborts the in-flight search; the returned results are then
// partial/unreliable and must be discarded (pass noStop for an unbounded search).
// repHist enables repetition scoring against the real game's positions (nil
// disables); it is read-only here and in every goroutine it fans out to.
func evaluateRootMoves(situation *octad.Game, moves []octad.Move, depth int, stop *atomic.Bool, repHist map[string]int) []MoveEval {
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
			stop:      stop,
			repHist:   repHist,
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
func pickOpeningMove(situation *octad.Game, depth int, repHist map[string]int) MoveEval {
	moves := orderMoves(situation)
	results := evaluateRootMoves(situation, moves, depth, noStop, repHist)
	if len(results) == 0 {
		// no moves searched (shouldn't happen for a live position); defer to
		// the standard best-move logic and its losing-position fallback
		return minimaxABRoot(situation, depth, repHist)
	}

	return pickVariety(situation, results)
}

// pickVariety applies the opening-variety selection to a completed set of root
// evaluations: a random pick from the top OpeningVarietyMoves within
// OpeningVarietyMargin of the best (see pickOpeningMove).
func pickVariety(situation *octad.Game, results []MoveEval) MoveEval {
	isWhite := situation.Position().Turn() == octad.White

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
		panic(fmt.Errorf("pos: %+v, move: %+v: %w", params.situation, params.move, err))
	}

	// each root goroutine gets its own path tracker over the shared history
	eval := minimaxAB(&params.situation, &params.move, !params.isWhite, params.depth,
		params.stop, newRepTracker(params.repHist))

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
	stop *atomic.Bool,
	rep *repTracker,
) float64 {
	if isMaxi {
		return mmABMax(node, move, depth, -WinVal, WinVal, stop, rep)
	}
	return mmABMin(node, move, depth, -WinVal, WinVal, stop, rep)
}

// mmABMax is the maximizing routine for minimax with alpha-beta pruning
func mmABMax(node *octad.Game, lastMove *octad.Move, depth int, alpha, beta float64, stop *atomic.Bool, rep *repTracker) float64 {
	// search aborted: unwind immediately; the caller discards the result
	if stop.Load() {
		return alpha
	}

	// a node that revisits a real-game position (or completes a threefold
	// within this line) scores as the draw octad will eventually rule it:
	// 0 in the search's absolute white-positive space
	if rep != nil {
		key := repetitionKey(node.Position().String())
		if rep.isDrawnRepetition(key) {
			return 0
		}
		rep.enter(key)
		defer rep.leave(key)
	}

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
			panic(fmt.Errorf("pos: %+v, move: %+v: %w", node, move, err))
		}

		eval := mmABMin(node, move, depth-1, alpha, beta, stop, rep)
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

// mmABMin is the minimizing routine for minimax with alpha-beta pruning
func mmABMin(node *octad.Game, lastMove *octad.Move, depth int, alpha, beta float64, stop *atomic.Bool, rep *repTracker) float64 {
	// search aborted: unwind immediately; the caller discards the result
	if stop.Load() {
		return beta
	}

	// repetition draw: see the matching check in mmABMax (0 is a draw in both
	// the relative and absolute conventions, so no sign flip is needed here)
	if rep != nil {
		key := repetitionKey(node.Position().String())
		if rep.isDrawnRepetition(key) {
			return 0
		}
		rep.enter(key)
		defer rep.leave(key)
	}

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
			panic(fmt.Errorf("pos: %+v, move: %+v: %w", node, move, err))
		}

		eval := mmABMax(node, move, depth-1, alpha, beta, stop, rep)
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
