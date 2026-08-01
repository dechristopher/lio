// Package learn is the server side of the /learn tutorial: a hands-on
// beginner's course in Octad.
//
// The browser has no octad rules engine — the same constraint that puts every
// analysis-board move through /api/analysis — so the tutorial board is driven
// the same way. The client renders and collects input; every move a learner
// makes round-trips here to be applied by the octad library, judged against the
// lesson's goal, and answered with the coach's verdict plus the opponent's
// reply.
//
// Everything here is stateless. A request names the lesson, the step, and the
// position the learner is looking at, and the response describes what happened.
// There is no room, no socket, no session and no persistence, so a learner can
// reload, deep-link a lesson, or come back a week later and nothing has to have
// survived on the server. How far somebody got is the client's business
// (localStorage), because the tutorial is for people who do not have an account
// yet — requiring one to learn the game would defeat the point.
//
// Positions are authored two ways (see Step): as a move sequence from the
// standard start, which lets the library compute castle rights, en passant and
// the draw clocks correctly, or as a literal OFEN for the artificial puzzle
// positions no real game reaches. Both are validated at package init and by
// TestCurriculum, which replays every step's stated solution and requires it to
// satisfy that step's goal — so a mistyped position or an unsolvable puzzle
// fails the build, not the learner.
package learn

import (
	"fmt"
	"strings"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/engine"
)

// Kind is how a lesson is played, which is also which UI mode the client puts
// the board into.
type Kind string

const (
	// KindDrill is the common case: a position, a goal, and a coach. The board
	// accepts moves and the server judges each one.
	KindDrill Kind = "drill"
	// KindDeploy teaches the blind arrangement phase. The board accepts swaps
	// within the learner's home rank instead of moves, and committing reveals
	// an opponent arrangement.
	KindDeploy Kind = "deploy"
	// KindPlay is the graduation game: a full, untimed game against the
	// weakest bot persona, still coached.
	KindPlay Kind = "play"
)

// Goal is what a step asks the learner to do. The judge (see judge) reads it
// against the move that was just played and the position it produced.
type Goal string

const (
	// GoalSelect is answered by clicking squares, not moving: the board-literacy
	// step that teaches coordinates. Judged on the client, since no move is made.
	GoalSelect Goal = "select"
	// GoalReach is satisfied when the move lands on a target square.
	GoalReach Goal = "reach"
	// GoalCapture is satisfied by a capture, on a target square if one is given.
	GoalCapture Goal = "capture"
	// GoalEnPassant is satisfied by an en passant capture specifically.
	GoalEnPassant Goal = "enpassant"
	// GoalCastle is satisfied by castling, of the named type if one is given.
	GoalCastle Goal = "castle"
	// GoalPromote is satisfied by promoting a pawn.
	GoalPromote Goal = "promote"
	// GoalCheck is satisfied by a move that gives check.
	GoalCheck Goal = "check"
	// GoalEscape is satisfied by any legal move: it is the "you are in check
	// and the board will only let you fix it" step.
	GoalEscape Goal = "escape"
	// GoalMate is satisfied by checkmate.
	GoalMate Goal = "mate"
	// GoalStalemate is satisfied by stalemating the opponent.
	GoalStalemate Goal = "stalemate"
	// GoalDeploy is satisfied by committing a legal arrangement.
	GoalDeploy Goal = "deploy"
	// GoalWin is satisfied by winning the graduation game.
	GoalWin Goal = "win"
)

// Step is one task inside a lesson: a position, something to do, and the words
// the coach says about it.
type Step struct {
	// Setup is the position as UOI moves from the standard start. Preferred
	// over OFEN wherever the position is reachable in a real game, because the
	// library then derives castle rights, the en passant square and the draw
	// clocks — the three things that are easy to get wrong by hand and
	// invisible until a lesson misbehaves.
	Setup []string
	// OFEN is the position written out, for the constructed puzzles no game
	// reaches. Exactly one of Setup and OFEN is set (init enforces it).
	OFEN string

	// Prompt is what the coach says when the step opens: the teaching, the
	// part that explains why. Success is what it says when the goal is met;
	// Hint is the nudge, shown on request and after a move that does not get
	// there.
	Prompt string
	// Action is the instruction on its own — the one sentence that says what to
	// do right now. It is split out of Prompt rather than left as its last
	// sentence so the coach panel can set it apart (it renders in the accent
	// colour): a learner skimming a paragraph should still be able to see what
	// the board is waiting for. Prompt teaches, Action tells.
	Action  string
	Hint    string
	Success string

	// Goal is what completes the step, with Targets/Castle/Promo narrowing it
	// (see the Goal constants). Empty Targets means "anywhere".
	Goal    Goal
	Targets []string
	// Castle names which castle satisfies a GoalCastle step: "near", "center"
	// or "far". Empty accepts any.
	Castle string

	// Moves bounds how many moves the learner gets before the step is failed
	// and offered a retry. 0 is unlimited, which is right for free practice and
	// wrong for a mate in one — there, a budget of 1 is what turns a wrong move
	// into immediate, useful feedback.
	Moves int

	// Replies scripts the opponent by the learner's move (player UOI ->
	// opponent UOI), so a teaching line stays on the rails. A move with no
	// scripted answer falls through to Engine.
	Replies map[string]string
	// Engine answers any unscripted move with a weak engine search. Without it
	// (and without a scripted reply) the opponent simply does not move.
	Engine bool

	// Solo keeps the move with the learner after every move, by flipping the
	// side to move back. It is how a piece-movement drill lets somebody walk a
	// king across an empty board without an opponent wandering around behind
	// them. Only ever set on positions built for it.
	Solo bool

	// Solution is a move sequence that completes the step. It is what the
	// "Show me" button plays back, what TestCurriculum replays to prove the step
	// is solvable at all, and what the board's move marks are derived from — the
	// circled piece and the arrow come from Solution[n], never from per-step
	// annotations, so they cannot drift from the move the step actually wants.
	//
	// Leaving it empty makes the step unaided — no marks, no "Show me", and no
	// answer anywhere in the page for a curious learner to read. That is the
	// right choice for a puzzle whose whole value is working the move out (the
	// checkmate and stalemate steps); the hint still arrives after a wrong try,
	// so nobody is stuck. A step with no Solution must be solvable in a single
	// move, which TestCurriculum proves by trying every legal one — the build
	// still refuses to ship a puzzle that cannot be solved.
	Solution []string

	// PriorMove is the opponent move that produced this step's position, drawn
	// as a blue arrow while the learner has yet to move. It exists for the
	// positions whose whole point is what just happened — en passant is only
	// comprehensible if you can see the two squares the black pawn skipped
	// through. Deliberately narrow: it annotates history, not the answer (the
	// green marks do that), so it cannot become a way to hand-draw hints.
	PriorMove string

	// start is the resolved starting OFEN, computed once at init from Setup or
	// OFEN so no request pays for the replay.
	start string
	// startDests is the starting position's legal-move map, also computed at
	// init. It is shipped in the page so a step's board accepts input the
	// instant the step opens: arming the board off a round trip left a window,
	// short but real, in which a learner's first click on a piece did nothing.
	startDests map[string][]string
}

// Start is the step's starting position.
func (s Step) Start() string { return s.start }

// StartDests is the starting position's legal-move map.
func (s Step) StartDests() map[string][]string { return s.startDests }

// Lesson is one entry in the course: a titled group of steps sharing a theme.
type Lesson struct {
	// Slug is the lesson's stable id: the URL segment (/learn/castling), the
	// API's lesson field, and the localStorage progress key.
	Slug    string
	Title   string
	Chapter string
	// Blurb is the one-line description under the title in the lesson rail.
	Blurb string
	// Icon is a single glyph shown beside the lesson in the rail.
	Icon  string
	Kind  Kind
	Steps []Step
}

// BySlug resolves a lesson by its slug, reporting whether it exists.
func BySlug(slug string) (*Lesson, bool) {
	for i := range Lessons {
		if Lessons[i].Slug == slug {
			return &Lessons[i], true
		}
	}
	return nil, false
}

// Index returns a lesson's position in the course, or -1.
func Index(slug string) int {
	for i := range Lessons {
		if Lessons[i].Slug == slug {
			return i
		}
	}
	return -1
}

// standardStart is the classic Octad starting position, the base every Setup
// sequence is replayed from.
const standardStart = "ppkn/4/4/NKPP w NCFncf - 0 1"

// init resolves every step's starting position and fails loudly on a malformed
// lesson. The curriculum is compiled-in data, so a bad position is a programmer
// error that should never reach a learner — and a panic here surfaces it on the
// first run rather than as a mysteriously dead board.
func init() {
	for li := range Lessons {
		l := &Lessons[li]
		for si := range l.Steps {
			s := &l.Steps[si]
			start, err := resolveStart(*s)
			if err != nil {
				panic(fmt.Sprintf("learn: lesson %q step %d: %v", l.Slug, si, err))
			}
			s.start = start
			g, err := newGame(start)
			if err != nil {
				panic(fmt.Sprintf("learn: lesson %q step %d: %v", l.Slug, si, err))
			}
			s.startDests = legalDests(g)
			if s.PriorMove != "" && !validSquarePair(s.PriorMove) {
				panic(fmt.Sprintf("learn: lesson %q step %d: malformed PriorMove %q",
					l.Slug, si, s.PriorMove))
			}
		}
	}
}

// validSquarePair reports whether a string is two 4x4 board squares — the shape
// PriorMove needs to draw an arrow. It is not checked for legality: PriorMove
// describes the move that led to an authored position, which by construction is
// not in any game history the package can replay.
func validSquarePair(s string) bool {
	if len(s) != 4 {
		return false
	}
	for i := 0; i < 4; i += 2 {
		if s[i] < 'a' || s[i] > 'd' || s[i+1] < '1' || s[i+1] > '4' {
			return false
		}
	}
	return true
}

// resolveStart turns a step's Setup or OFEN into the position it starts from.
func resolveStart(s Step) (string, error) {
	if (len(s.Setup) == 0) == (s.OFEN == "") {
		return "", fmt.Errorf("exactly one of Setup and OFEN must be set")
	}
	if s.OFEN != "" {
		if _, err := octad.OFEN(s.OFEN); err != nil {
			return "", fmt.Errorf("invalid ofen %q: %w", s.OFEN, err)
		}
		return s.OFEN, nil
	}
	g, err := newGame(standardStart)
	if err != nil {
		return "", err
	}
	for _, uoi := range s.Setup {
		if err := playUOI(g, uoi); err != nil {
			return "", fmt.Errorf("setup move %q: %w", uoi, err)
		}
	}
	return g.Position().String(), nil
}

// newGame builds a game from a bare OFEN.
func newGame(ofen string) (*octad.Game, error) {
	pos, err := octad.OFEN(ofen)
	if err != nil {
		return nil, err
	}
	return octad.NewGame(pos)
}

// findMove resolves a UOI string to a legal move in the position, or nil.
func findMove(g *octad.Game, uoi string) *octad.Move {
	for _, m := range g.ValidMoves() {
		if m.String() == uoi {
			return m
		}
	}
	return nil
}

// playUOI applies a UOI move to the game, erroring when it is not legal.
func playUOI(g *octad.Game, uoi string) error {
	m := findMove(g, uoi)
	if m == nil {
		return fmt.Errorf("illegal move %q in %s", uoi, g.Position().String())
	}
	return g.Move(m)
}

// flipTurn rewrites a position's side to move and resets its draw clock, for
// Solo practice steps where the learner keeps moving the same piece. The
// resulting position is validated, because handing the move back can be
// illegal (it would leave the other side in check); when it is, the caller
// keeps the real position and the drill simply ends there.
func flipTurn(ofen string) (string, bool) {
	parts := strings.Split(ofen, " ")
	if len(parts) != 6 {
		return "", false
	}
	if parts[1] == "w" {
		parts[1] = "b"
	} else {
		parts[1] = "w"
	}
	// a Solo drill shuffles one piece around for as long as the learner likes;
	// left running, the 25-move rule would eventually call a draw mid-lesson
	parts[4] = "0"
	flipped := strings.Join(parts, " ")
	if _, err := octad.OFEN(flipped); err != nil {
		return "", false
	}
	return flipped, true
}

// botDepth / botBudget bound the opponent's search for unscripted replies. The
// tutorial's opponent is the weakest persona and its positions are trivial, so
// this only has to stay far below anything a learner would notice as a pause.
const (
	botDepth  = 4
	botBudget = 150 * time.Millisecond
)

// tutorBot is the persona behind every unscripted reply: the gentlest rung of
// the ladder, the same one the difficulty modal recommends to a new player.
var tutorBot = engine.PersonaByKey("pawn")

// reply picks the opponent's answer to the learner's move: the step's scripted
// answer when it has one, else a weak engine move when the step asks for one.
// An empty string means the opponent does not move.
func reply(s Step, g *octad.Game, uoi string) string {
	if mv, ok := s.Replies[uoi]; ok {
		return mv
	}
	if !s.Engine {
		return ""
	}
	if len(g.ValidMoves()) == 0 {
		return ""
	}
	me := engine.SearchPersona(g.Position().String(), nil, botDepth, botBudget, tutorBot)
	return me.Move.String()
}
