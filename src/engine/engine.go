package engine

import (
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/bus"
)

// MoveEval contains the best move and the evaluation of the best sequence
// of moves to the given depth
type MoveEval struct {
	Eval float64
	Move octad.Move
}

// SearchAlg selector for Search function
type SearchAlg int

// MinimaxAB selects minimax with alpha-beta pruning
const MinimaxAB SearchAlg = 0

// NegamaxAB selects negamax with alpha-beta pruning
const NegamaxAB SearchAlg = 1

// Random move engine
const Random SearchAlg = 2

// Channel is the engine monitoring bus channel
const Channel bus.Channel = "lio:engine"

// DrawEvalMargin is the maximum absolute (white-positive) search evaluation,
// in the engine's material units (a pawn is 10), within which the bot will
// accept a human's draw offer: the bot agrees only when neither side is winning
// by more than this margin, and otherwise plays on. It is a strength/behavior
// knob, not a correctness value.
const DrawEvalMargin float64 = 20

// pub is the engine publisher
var pub = bus.NewPublisher("engine", Channel)

// Search returns the best move after running a search algorithm on the given
// position to the given depth. A positive budget bounds how long the search
// may run: MinimaxAB then iteratively deepens toward depth and returns the
// best move found when the budget expires, so the caller can size the budget
// off the bot's clock and never flag. A zero budget searches the full depth
// unconditionally.
//
// history is the game's position history as OFENs (oldest first, including
// the current position — game.OctadGame.OFENHistory's shape). Search rebuilds
// the game from the bare OFEN, so without it the search cannot see draws by
// repetition coming and will happily shuffle a won endgame into a threefold;
// with it, MinimaxAB scores any revisited position as the draw it leads to.
// nil/empty history disables repetition scoring (Negamax and Random ignore it).
func Search(ofen string, history []string, depth int, budget time.Duration, alg SearchAlg) MoveEval {
	// establish the deadline before any parsing/setup so all engine-side
	// overhead counts against the caller's budget
	var deadline time.Time
	if budget > 0 {
		deadline = time.Now().Add(budget)
	}

	o, err := octad.OFEN(ofen)
	if err != nil {
		panic(err)
	}

	// build out game state from ofen
	situation, err := octad.NewGame(o)
	if err != nil {
		panic(err)
	}

	var eval MoveEval

	// publish search starting message
	pub.Publish(ofen, alg)

	start := time.Now()

	// run selected search algorithm
	if alg == MinimaxAB {
		eval = searchMinimaxAB(situation, depth, deadline, RepetitionHistory(history))
	} else if alg == NegamaxAB {
		eval = searchNegamaxAB(situation, depth)
	} else if alg == Random {
		eval = randomMove(situation)
	} else {
		panic("invalid search algorithm")
	}

	// publish time taken, ofen, alg, and eval to engine channel
	pub.Publish(time.Since(start).Seconds(), ofen, alg, eval)

	return eval
}

// TestEngine runs a quick test of the engine for a given ofen
// at the given depth and prints all moves and positions
//func TestEngine(ofen string, depth int) {
//	//ofen := "K3/2kq/4/4 b - - 15 7"
//	//ofen := "4/k1KP/4/4 w - - 0 2"
//	o, _ := octad.OFEN(ofen)
//	game, _ := octad.NewGame(o)
//
//	util.Debug("TestEngine", game.Position().String())
//	fmt.Print(game.Position().Board().Draw())
//
//	for game.Outcome() == octad.NoOutcome {
//		move := Search(game.Position().String(), depth, MinimaxAB)
//		_ = game.Move(&move.Move)
//
//		util.Debug("TestEngine", move.Move.String())
//		util.Debug("TestEngine", game.Position().String())
//		fmt.Print(game.Position().Board().Draw())
//	}
//
//	util.Debug("TestEngine", game.Outcome().String())
//}
