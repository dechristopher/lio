package handlers

import (
	"regexp"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/engine"
)

// POST /api/analysis — the exploration seam behind the study-style analysis
// board (archive pages + post-game bot-room analysis). The client has no octad
// rules engine, so every explored position round-trips here: given an OFEN
// (and optionally a UOI move to apply to it), respond with the resulting
// position, its legal-move map, terminal state, and a white-positive
// centipawn eval. Evals read through the positions cache (hash lookup) and
// fall back to a tightly budgeted live engine search — exploration must stay
// interactive, and the endpoint is rate-limited because that search is real
// CPU. Nothing here writes to the archive: explored positions are ephemeral
// and never pollute the games/moves/positions analytics index.

const (
	// analysisDepth / analysisBudget bound the fallback engine search per
	// request. Depth matches the background evaluator; the wall-clock budget is
	// much tighter (deepeningRoot returns the best fully-searched depth).
	analysisDepth  = 8
	analysisBudget = 200 * time.Millisecond

	// analysisEvalCap mirrors the evaluator's int16 saturation for mate-ish
	// scores; the engine's material unit is decipawns (×10 → centipawns).
	analysisEvalCap   = 32000
	analysisCentiUnit = 10
)

// uoiPattern validates a UOI move string on the 4x4 board, with an optional
// promotion piece suffix.
var uoiPattern = regexp.MustCompile(`^[a-d][1-4][a-d][1-4][qrbn]?$`)

// analysisRequest is the exploration request: a position, and optionally a
// move to apply to it (absent = just describe/evaluate the position).
type analysisRequest struct {
	OFEN string `json:"ofen"`
	UOI  string `json:"uoi"`
}

// analysisResponse mirrors the live wire's field names where they overlap
// ("o"/"s"/"k"/"v") so the client render paths stay uniform.
type analysisResponse struct {
	OFEN  string              `json:"o"`
	SAN   string              `json:"s,omitempty"`
	Check bool                `json:"k,omitempty"`
	Dests map[string][]string `json:"v"`
	// Over is the resulting position's terminal outcome ("w"/"b"/"d"), empty
	// while the position is playable.
	Over string `json:"over,omitempty"`
	// Reason is the terminal method as a client result-reason key
	// ("checkmate"/"stalemate"/"insufficient"/"repetition"/"moverule"), so an
	// explored line that ends decisively shows the same "… by checkmate"
	// annotation as a real game end. Empty while the position is playable.
	Reason string `json:"rr,omitempty"`
	// CP is the position's white-positive centipawn eval (cache read-through,
	// else a budgeted live search; exact for terminal positions).
	CP int16 `json:"cp"`
}

// AnalysisHandler applies/evaluates one explored position (see package
// comment above).
func AnalysisHandler(c fiber.Ctx) error {
	var req analysisRequest
	if err := c.Bind().Body(&req); err != nil || req.OFEN == "" || len(req.OFEN) > 100 {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "malformed request"})
	}

	fromPos, err := octad.OFEN(req.OFEN)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(fiber.Map{"error": "invalid position"})
	}
	g, err := octad.NewGame(fromPos)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(fiber.Map{"error": "invalid position"})
	}

	resp := analysisResponse{}

	if req.UOI != "" {
		if !uoiPattern.MatchString(req.UOI) {
			return c.Status(fiber.StatusUnprocessableEntity).
				JSON(fiber.Map{"error": "malformed move"})
		}
		var match *octad.Move
		for _, m := range g.ValidMoves() {
			if m.String() == req.UOI {
				match = m
				break
			}
		}
		if match == nil {
			return c.Status(fiber.StatusUnprocessableEntity).
				JSON(fiber.Map{"error": "illegal move"})
		}
		before := g.Position()
		if err := g.Move(match); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).
				JSON(fiber.Map{"error": "illegal move"})
		}
		resp.SAN = octad.AlgebraicNotation{}.Encode(before, match)
	}

	pos := g.Position()
	resp.OFEN = pos.String()
	resp.Check = pos.InCheck()
	resp.Over = winnerFromOutcome(g.Outcome().String())
	if resp.Over != "" {
		resp.Reason = reasonFromMethod(g.Method())
	}

	// legal-move map of the resulting position (empty when terminal)
	resp.Dests = map[string][]string{}
	if resp.Over == "" {
		for _, m := range g.ValidMoves() {
			s1 := m.S1().String()
			resp.Dests[s1] = append(resp.Dests[s1], m.S2().String())
		}
	}

	resp.CP = analysisEval(resp.OFEN, pos, resp.Over)
	return c.JSON(resp)
}

// reasonFromMethod maps octad's terminal Method to the client's result-reason
// key (mirrors resultReasons in lio-game.js), so an explored line that ends
// decisively shows the same annotation as a real game end. Only the
// auto-detectable methods can arise from a bare explored position —
// resignation/draw-offer never do; threefold needs game history the endpoint
// doesn't carry — but the drawn methods are mapped for completeness.
func reasonFromMethod(m octad.Method) string {
	switch m {
	case octad.Checkmate:
		return "checkmate"
	case octad.Stalemate:
		return "stalemate"
	case octad.InsufficientMaterial:
		return "insufficient"
	case octad.ThreefoldRepetition:
		return "repetition"
	case octad.TwentyFiveMoveRule:
		return "moverule"
	}
	return ""
}

// analysisEval scores a position white-positive in centipawns: exact for
// terminal positions, else the cached eval by position hash, else a budgeted
// live engine search (nil history — a bare explored position has no game line
// for repetition scoring).
func analysisEval(ofen string, pos *octad.Position, over string) int16 {
	switch over {
	case "w":
		return analysisEvalCap
	case "b":
		return -analysisEvalCap
	case "d":
		return 0
	}

	hash := pos.Hash()
	if cached := db.CachedEvalByHash(hash[:]); cached != nil {
		return *cached
	}

	me := engine.Search(ofen, nil, analysisDepth, analysisBudget, engine.MinimaxAB)
	cp := me.Eval * analysisCentiUnit
	if cp > analysisEvalCap {
		return analysisEvalCap
	}
	if cp < -analysisEvalCap {
		return -analysisEvalCap
	}
	return int16(cp)
}
