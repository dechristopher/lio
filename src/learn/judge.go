package learn

import (
	"errors"
	"strings"

	"github.com/dechristopher/octad/v2"
)

// Request is one interaction with a lesson step: the position the learner is
// looking at, and optionally the move they just made. A request with no move
// is the "describe this position" call the client makes on entering a step, so
// the board knows its legal destinations — the same describe-only shape
// /api/analysis uses, for the same reason.
//
// Played is how many moves the learner has already spent on this step. It is
// client state by design: the server keeps nothing between requests, and the
// only thing a learner gains by lying about it is a longer tutorial.
type Request struct {
	Lesson string `json:"lesson"`
	Step   int    `json:"step"`
	OFEN   string `json:"ofen"`
	UOI    string `json:"uoi"`
	Played int    `json:"played"`
	// Deploy is the learner's home-rank arrangement on a KindDeploy step, as
	// four piece letters (k/n/p) from their own left to right.
	Deploy string `json:"deploy"`
}

// Move describes one move's effect on the board — the shape the client needs to
// render it. Field names match the live wire and /api/analysis ("o"/"s"/"k")
// so the client's render paths stay uniform.
type Move struct {
	OFEN  string `json:"o"`
	SAN   string `json:"s,omitempty"`
	Check bool   `json:"k,omitempty"`
	UOI   string `json:"uoi,omitempty"`
	// LastMove is the from/to pair octadground highlights.
	LastMove []string `json:"lm,omitempty"`
	// Capture marks a move that took a piece, so the client plays the capture
	// sound instead of the move sound.
	Capture bool `json:"x,omitempty"`
}

// Response is the coach's answer: what the board looks like now, whether the
// step is done, and what to say about it.
type Response struct {
	// Move is the learner's move as applied. Absent on a describe-only call.
	Move *Move `json:"mv,omitempty"`
	// Reply is the opponent's answer, when there is one.
	Reply *Move `json:"rp,omitempty"`
	// OFEN and Dests describe the position the learner now has to act on —
	// after the reply, if there was one.
	OFEN  string              `json:"o"`
	Dests map[string][]string `json:"v"`
	// Turn is whose move it now is ("white"/"black"), which the client needs
	// because a Solo step hands the move back to the learner.
	Turn string `json:"t"`
	// Check flags the side to move being in check, for the board's highlight.
	Check bool `json:"k,omitempty"`

	// Done is true when the step's goal has been met.
	Done bool `json:"done,omitempty"`
	// Failed is true when the step can no longer be completed — the move budget
	// is spent, or the game ended the wrong way. The client offers a retry.
	Failed bool `json:"failed,omitempty"`
	// Say is the coach's line for what just happened.
	Say string `json:"say,omitempty"`
	// Over and Reason report a finished game ("w"/"b"/"d" and the result-reason
	// key), so the graduation game can show the same annotation a real game does.
	Over   string `json:"over,omitempty"`
	Reason string `json:"rr,omitempty"`
}

// ErrBadRequest marks a request the caller should reject with a 4xx.
var ErrBadRequest = errors.New("learn: bad request")

// Do runs one interaction against the curriculum. It is the whole API surface:
// resolve the lesson and step, apply the learner's move if there is one, judge
// it, let the opponent answer, and describe what the learner now faces.
func Do(req Request) (*Response, error) {
	lesson, ok := BySlug(req.Lesson)
	if !ok {
		return nil, ErrBadRequest
	}
	if req.Step < 0 || req.Step >= len(lesson.Steps) {
		return nil, ErrBadRequest
	}
	step := lesson.Steps[req.Step]

	// A deploy step commits an arrangement rather than playing a move: it
	// builds the position the rest of the lesson plays from, so it resolves
	// before the ordinary move path.
	if step.Goal == GoalDeploy {
		return doDeploy(step, req)
	}

	ofen := req.OFEN
	if ofen == "" {
		ofen = step.Start()
	}
	g, err := newGame(ofen)
	if err != nil {
		return nil, ErrBadRequest
	}

	resp := &Response{}

	// describe-only: the client entering a step (or resyncing) just needs the
	// legal moves for the position it is showing
	if req.UOI == "" {
		describe(resp, g)
		return resp, nil
	}

	mv := findMove(g, req.UOI)
	if mv == nil {
		return nil, ErrBadRequest
	}
	before := g.Position()
	if err := g.Move(mv); err != nil {
		return nil, ErrBadRequest
	}
	resp.Move = &Move{
		OFEN:     g.Position().String(),
		SAN:      octad.AlgebraicNotation{}.Encode(before, mv),
		Check:    g.Position().InCheck(),
		UOI:      req.UOI,
		LastMove: []string{mv.S1().String(), mv.S2().String()},
		Capture:  mv.HasTag(octad.Capture),
	}

	// judge the move that was just played, in the position it produced
	met := judge(step, mv, g)

	if met {
		resp.Done = true
		resp.Say = successLine(step, mv)
		describe(resp, g)
		finishState(resp, g)
		return resp, nil
	}

	// the learner's move ended the game without meeting the goal — a stalemate
	// where mate was asked for, say. Nothing is recoverable from there.
	if g.Outcome() != octad.NoOutcome {
		resp.Failed = true
		resp.Say = missSay(step, g)
		describe(resp, g)
		finishState(resp, g)
		return resp, nil
	}

	// the opponent answers
	if answer := reply(step, g, req.UOI); answer != "" {
		rmv := findMove(g, answer)
		if rmv != nil {
			rbefore := g.Position()
			if err := g.Move(rmv); err == nil {
				resp.Reply = &Move{
					OFEN:     g.Position().String(),
					SAN:      octad.AlgebraicNotation{}.Encode(rbefore, rmv),
					Check:    g.Position().InCheck(),
					UOI:      answer,
					LastMove: []string{rmv.S1().String(), rmv.S2().String()},
					Capture:  rmv.HasTag(octad.Capture),
				}
			}
		}
	}

	// a Solo step hands the move straight back so one piece can be practised
	// without an opponent; it is skipped once the position would be illegal
	// with the turn flipped (see flipTurn).
	if step.Solo && resp.Reply == nil {
		if flipped, ok := flipTurn(g.Position().String()); ok {
			if fg, err := newGame(flipped); err == nil {
				g = fg
			}
		}
	}

	describe(resp, g)
	finishState(resp, g)

	// the graduation game is judged only by its outcome, so a move that neither
	// won nor lost is simply the game continuing
	if resp.Over != "" && !resp.Done {
		resp.Failed = true
		resp.Say = missSay(step, g)
		return resp, nil
	}

	// out of moves: the step is failed and the learner is offered a reset
	if step.Moves > 0 && req.Played+1 >= step.Moves {
		resp.Failed = true
		resp.Say = missSay(step, g)
		return resp, nil
	}

	resp.Say = step.Hint
	return resp, nil
}

// describe fills the response's view of the position the learner now faces.
func describe(resp *Response, g *octad.Game) {
	pos := g.Position()
	resp.OFEN = pos.String()
	resp.Check = pos.InCheck()
	resp.Turn = "white"
	if pos.Turn() == octad.Black {
		resp.Turn = "black"
	}
	resp.Dests = legalDests(g)
}

// legalDests is the position's legal-move map in the shape octadground wants.
// A promotion push generates one move per piece choice, all to the same square;
// the board wants each destination once, and the piece is chosen in the
// promotion picker after the drag.
func legalDests(g *octad.Game) map[string][]string {
	dests := map[string][]string{}
	if g.Outcome() != octad.NoOutcome {
		return dests
	}
	seen := map[string]bool{}
	for _, m := range g.ValidMoves() {
		s1, s2 := m.S1().String(), m.S2().String()
		if seen[s1+s2] {
			continue
		}
		seen[s1+s2] = true
		dests[s1] = append(dests[s1], s2)
	}
	return dests
}

// finishState records a finished game's result on the response.
func finishState(resp *Response, g *octad.Game) {
	switch g.Outcome() {
	case octad.WhiteWon:
		resp.Over = "w"
	case octad.BlackWon:
		resp.Over = "b"
	case octad.Draw:
		resp.Over = "d"
	default:
		return
	}
	resp.Reason = reasonKey(g.Method())
}

// reasonKey maps octad's terminal Method to the client's result-reason key,
// the same vocabulary lio-game.js and /api/analysis already speak.
func reasonKey(m octad.Method) string {
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

// successLine is the coach's completion line with the one substitution a step
// can ask for: "{piece}" becomes the piece a promotion actually produced. A
// learner who deliberately underpromotes should be congratulated on the knight
// they chose, not told they made a queen.
func successLine(step Step, mv *octad.Move) string {
	if !strings.Contains(step.Success, piecePlaceholder) {
		return step.Success
	}
	return strings.ReplaceAll(step.Success, piecePlaceholder, promoName(mv.Promo()))
}

// piecePlaceholder is the token a Success line uses to name the promoted piece.
const piecePlaceholder = "{piece}"

// promoName is a promotion choice as the coach says it out loud.
func promoName(pt octad.PieceType) string {
	switch pt {
	case octad.Queen:
		return "queen"
	case octad.Rook:
		return "rook"
	case octad.Bishop:
		return "bishop"
	case octad.Knight:
		return "knight"
	}
	return "piece"
}

// missSay is what the coach says when a step is failed: the step's own hint
// where it has one, else a generic nudge. Keeping the hint as the failure line
// means the retry starts with the most useful sentence already on screen.
func missSay(step Step, _ *octad.Game) string {
	if step.Hint != "" {
		return step.Hint
	}
	return "Not this time — reset and try again."
}

// judge reports whether the move just played satisfied the step's goal. It is
// given the move and the game after that move, so it can read both the move's
// own tags (capture, castle, promotion) and the position it produced (check,
// checkmate, stalemate).
func judge(step Step, mv *octad.Move, g *octad.Game) bool {
	pos := g.Position()
	switch step.Goal {
	case GoalReach:
		return targeted(step, mv.S2().String())
	case GoalCapture:
		if !mv.HasTag(octad.Capture) {
			return false
		}
		return targeted(step, mv.S2().String())
	case GoalEnPassant:
		return mv.HasTag(octad.EnPassant)
	case GoalCastle:
		switch strings.ToLower(step.Castle) {
		case "near":
			return mv.HasTag(octad.NearCastle)
		case "center":
			return mv.HasTag(octad.CenterCastle)
		case "far":
			return mv.HasTag(octad.FarCastle)
		default:
			return mv.HasTag(octad.NearCastle) ||
				mv.HasTag(octad.CenterCastle) ||
				mv.HasTag(octad.FarCastle)
		}
	case GoalPromote:
		return mv.Promo() != octad.NoPieceType
	case GoalCheck:
		// checkmate is a check too, and a step that asks only for check should
		// be delighted by one
		return pos.InCheck()
	case GoalEscape:
		// the board only ever offers legal moves, so any move made from a
		// position in check got out of it — which is exactly the lesson
		return true
	case GoalMate:
		return g.Outcome() != octad.NoOutcome && g.Method() == octad.Checkmate
	case GoalStalemate:
		return g.Outcome() == octad.Draw && g.Method() == octad.Stalemate
	case GoalWin:
		// the learner plays White in the graduation game
		return g.Outcome() == octad.WhiteWon
	}
	return false
}

// targeted reports whether a square satisfies the step's target list. An empty
// list means any square will do.
func targeted(step Step, square string) bool {
	if len(step.Targets) == 0 {
		return true
	}
	for _, t := range step.Targets {
		if t == square {
			return true
		}
	}
	return false
}
