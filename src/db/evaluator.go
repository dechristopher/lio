package db

import (
	"time"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/engine"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// Lazy position evaluator. Because positions are deduped by Position.Hash(),
// each distinct position is evaluated exactly once — off the game path — filling
// the positions eval cache with a white-positive centipawn score and a best
// move. It is opt-in (the lio_pg_evaluator secret/env = "1") so a box under game
// load never pays for background search unless asked: correctness of the archive
// never depends on it, and the eval columns simply stay NULL until it runs.
const (
	evalTick   = 30 * time.Second       // batch cadence
	evalBatch  = 8                      // positions per tick (bounds background CPU)
	evalDepth  = 8                      // fixed search depth
	evalBudget = 750 * time.Millisecond // per-position wall-clock cap
	// engine material is decipawns (pawn=10); ×10 converts an eval to centipawns.
	evalCentiUnit = 10
	// int16 saturation for mate-ish scores (WinVal=10000 → far past the column).
	evalCap = 32000
)

// UpEvaluator starts the background evaluator loop when Postgres is configured
// and the evaluator is enabled. No-op otherwise.
func UpEvaluator() {
	if Pool == nil || config.ReadSecretFallback("lio_pg_evaluator") != "1" {
		return
	}
	go func() {
		ticker := time.NewTicker(evalTick)
		defer ticker.Stop()
		for range ticker.C {
			safeEvalBatch()
		}
	}()
	util.Debug(str.CDB, "position evaluator online")
}

// safeEvalBatch runs one evaluator batch, converting any panic into an error
// log. The evaluator is an opt-in background cache filler whose correctness
// nothing depends on — an unrecovered panic on its goroutine would kill the
// whole process (an engine edge case on terminal positions did exactly that),
// which is never the right trade for it.
func safeEvalBatch() {
	defer func() {
		if r := recover(); r != nil {
			util.Error(str.CDB, "evaluator batch panicked: %v", r)
		}
	}()
	evalBatchOnce()
}

// evalBatchOnce evaluates up to evalBatch unevaluated positions.
func evalBatchOnce() {
	ctx, cancel := Ctx()
	rows, err := gen.New(Pool).ListPositionsNeedingEval(ctx, evalBatch)
	cancel()
	if err != nil {
		util.Error(str.CDB, "evaluator list failed error=%s", err.Error())
		return
	}

	for _, row := range rows {
		// Search rebuilds a fresh game from the bare OFEN, so passing the stored
		// position string is safe for the parallel root (nil history disables
		// repetition scoring — a standalone position has no game line).
		me := engine.Search(row.Ofen, nil, evalDepth, evalBudget, engine.MinimaxAB)
		cp := clampEval(me.Eval * evalCentiUnit)
		depth := int16(evalDepth)

		// a terminal position (mate/stalemate) has no best move: Search returns
		// the zero move (the "a1a1" idiom, see engine.bestOf) — store the exact
		// terminal eval and leave best_move NULL rather than packing a non-move
		var bestPtr *int16
		if me.Move.String() != "a1a1" {
			best := game.PackMove(&me.Move)
			bestPtr = &best
		}

		setCtx, setCancel := Ctx()
		err := gen.New(Pool).SetPositionEval(setCtx, gen.SetPositionEvalParams{
			ID:        row.ID,
			EvalCp:    &cp,
			EvalDepth: &depth,
			BestMove:  bestPtr,
		})
		setCancel()
		if err != nil {
			util.Error(str.CDB, "evaluator set failed id=%d error=%s", row.ID, err.Error())
		}
	}
}

// clampEval converts an engine eval into saturated int16 centipawns: mate-ish
// scores clamp to ±evalCap so they fit the column.
func clampEval(cp float64) int16 {
	if cp > evalCap {
		return evalCap
	}
	if cp < -evalCap {
		return -evalCap
	}
	return int16(cp)
}
