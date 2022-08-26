package engine

import (
	"fmt"
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/bus"
	"github.com/dechristopher/lio/util"
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

// pub is the engine publisher
var pub = bus.NewPublisher("engine", Channel)

// Search returns the best move after running a search algorithm
// on the given position to the given depth
func Search(ofen string, depth int, alg SearchAlg) MoveEval {
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
		eval = searchMinimaxAB(situation, depth)
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
func TestEngine(ofen string, depth int) {
	//ofen := "K3/2kq/4/4 b - - 15 7"
	//ofen := "4/k1KP/4/4 w - - 0 2"
	o, _ := octad.OFEN(ofen)
	game, _ := octad.NewGame(o)

	util.Debug("", game.Position().String())
	fmt.Print(game.Position().Board().Draw())

	for game.Outcome() == octad.NoOutcome {
		move := Search(game.Position().String(), depth, MinimaxAB)
		_ = game.Move(&move.Move)

		util.Debug("", move.Move.String())
		util.Debug("", game.Position().String())
		fmt.Print(game.Position().Board().Draw())
	}

	util.Debug("", game.Outcome().String())
}
