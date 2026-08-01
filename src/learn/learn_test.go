package learn

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dechristopher/octad/v2"
)

// TestCurriculum is the guard that makes hand-authored lesson positions safe to
// write. For every step it replays the stated solution through the real judge
// and requires the step to come out completed. A mistyped OFEN, a solution that
// is not legal in its position, or a puzzle that does not actually work then
// fails here rather than presenting a learner with a dead board.
func TestCurriculum(t *testing.T) {
	for _, lesson := range Lessons {
		for si, step := range lesson.Steps {
			name := lesson.Slug + "/" + itoa(si)
			t.Run(name, func(t *testing.T) {
				if step.Start() == "" {
					t.Fatal("step has no resolved starting position")
				}
				if _, err := octad.OFEN(step.Start()); err != nil {
					t.Fatalf("starting position %q does not parse: %v", step.Start(), err)
				}

				// steps that are not solved by a move sequence: a select step is
				// judged in the browser (no move is made), and the deploy and
				// graduation steps have no single scripted answer
				switch step.Goal {
				case GoalSelect:
					if len(step.Solution) != len(step.Targets) {
						t.Fatalf("select step: %d solutions for %d targets",
							len(step.Solution), len(step.Targets))
					}
					return
				case GoalDeploy, GoalWin:
					return
				}

				// A step may deliberately state no solution: the checkmate and
				// stalemate puzzles withhold it so the learner has to work the
				// move out, and an answer that is nowhere in the curriculum is
				// also nowhere in the page. The build still refuses to ship an
				// unsolvable puzzle — it just has to find the move itself.
				if len(step.Solution) == 0 {
					assertSolvableInOneMove(t, lesson.Slug, si, step)
					return
				}

				ofen := step.Start()
				for mi, uoi := range step.Solution {
					resp, err := Do(Request{
						Lesson: lesson.Slug,
						Step:   si,
						OFEN:   ofen,
						UOI:    uoi,
						Played: mi,
					})
					if err != nil {
						t.Fatalf("solution move %d (%s): %v", mi, uoi, err)
					}
					last := mi == len(step.Solution)-1
					if resp.Failed && !last {
						t.Fatalf("solution move %d (%s) failed the step early: %s",
							mi, uoi, resp.Say)
					}
					if last {
						if !resp.Done {
							t.Fatalf("solution did not complete the step (last move %s, say %q)",
								uoi, resp.Say)
						}
						return
					}
					if resp.Done {
						t.Fatalf("step completed early at move %d (%s)", mi, uoi)
					}
					ofen = resp.OFEN
				}
			})
		}
	}
}

// assertSolvableInOneMove plays every legal move in a step's position through
// the real judge and requires at least one of them to complete it. It is what
// keeps a solution-less puzzle honest: the answer is withheld from the learner,
// not unknown to the build.
func assertSolvableInOneMove(t *testing.T, slug string, si int, step Step) {
	t.Helper()
	g, err := newGame(step.Start())
	if err != nil {
		t.Fatalf("%s step %d: %v", slug, si, err)
	}
	moves := g.ValidMoves()
	if len(moves) == 0 {
		t.Fatalf("%s step %d: position has no legal moves at all", slug, si)
	}

	var solving []string
	for _, m := range moves {
		resp, err := Do(Request{Lesson: slug, Step: si, OFEN: step.Start(), UOI: m.String()})
		if err != nil {
			t.Fatalf("%s step %d: move %s: %v", slug, si, m.String(), err)
		}
		if resp.Done {
			solving = append(solving, m.String())
		}
	}
	if len(solving) == 0 {
		t.Fatalf("%s step %d: no legal move completes it — the puzzle is unsolvable",
			slug, si)
	}
	// a bounded puzzle wants exactly one answer; several would make the "in one
	// move" framing a lie and let a guess pass for understanding
	if step.Moves == 1 && len(solving) > 1 {
		t.Errorf("%s step %d: %d moves complete a one-move puzzle (%v); want a single answer",
			slug, si, len(solving), solving)
	}
}

// TestLessonsWellFormed checks the course-level invariants the view and the
// client rely on: unique slugs, and every lesson carrying the text the rail and
// the coach panel render.
func TestLessonsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range Lessons {
		if l.Slug == "" || seen[l.Slug] {
			t.Fatalf("lesson slug %q is empty or duplicated", l.Slug)
		}
		seen[l.Slug] = true
		if l.Title == "" || l.Blurb == "" || l.Chapter == "" || l.Icon == "" {
			t.Errorf("lesson %q is missing rail text", l.Slug)
		}
		if len(l.Steps) == 0 {
			t.Errorf("lesson %q has no steps", l.Slug)
		}
		for si, s := range l.Steps {
			if s.Prompt == "" {
				t.Errorf("lesson %q step %d has no prompt", l.Slug, si)
			}
			// the accented instruction: every step has to say what to do, and
			// say it separately from the teaching around it
			if s.Action == "" {
				t.Errorf("lesson %q step %d has no action", l.Slug, si)
			}
			if s.Success == "" {
				t.Errorf("lesson %q step %d has no success line", l.Slug, si)
			}
			if s.Goal == "" {
				t.Errorf("lesson %q step %d has no goal", l.Slug, si)
			}
		}
	}
}

// genderedPronoun matches the third-person gendered pronouns, whole-word and
// case-insensitively.
var genderedPronoun = regexp.MustCompile(`(?i)\b(he|him|his|she|her|hers)\b`)

// TestNoGenderedPronouns keeps the course copy neutral. Nothing on this board
// has a gender: the opponent is a stranger or a bot, and the pieces are pieces.
// Calling either one "he" guesses at a real person and personifies wood, and
// both had crept in — "Black has only his king", "the queen ... where she
// covers". Every learner-visible string is checked, so a new lesson cannot
// quietly reintroduce it.
func TestNoGenderedPronouns(t *testing.T) {
	// guard the guard: a matcher that stopped matching would let this test pass
	// over any copy at all
	for _, sample := range []string{
		"Black has only his king", "where she covers a3", "checking him", "He is slow",
	} {
		if !genderedPronoun.MatchString(sample) {
			t.Fatalf("matcher does not catch %q; this test asserts nothing", sample)
		}
	}
	for _, sample := range []string{
		"the king", "there is no escape", "Check the king or leave it a square",
	} {
		if m := genderedPronoun.FindString(sample); m != "" {
			t.Fatalf("matcher false-positives on %q (matched %q)", sample, m)
		}
	}

	check := func(t *testing.T, where, text string) {
		t.Helper()
		if m := genderedPronoun.FindString(text); m != "" {
			t.Errorf("%s uses the gendered pronoun %q: %q", where, m, text)
		}
	}
	for _, l := range Lessons {
		where := "lesson " + l.Slug
		check(t, where+" title", l.Title)
		check(t, where+" blurb", l.Blurb)
		check(t, where+" chapter", l.Chapter)
		for si, s := range l.Steps {
			at := where + " step " + itoa(si)
			check(t, at+" prompt", s.Prompt)
			check(t, at+" action", s.Action)
			check(t, at+" hint", s.Hint)
			check(t, at+" success", s.Success)
		}
	}
}

// TestChaptersCoverEveryLesson checks the rail grouping keeps the course whole
// and in order.
func TestChaptersCoverEveryLesson(t *testing.T) {
	var got int
	for _, c := range Chapters() {
		if c.Title == "" {
			t.Error("chapter with no title")
		}
		got += len(c.Lessons)
	}
	if got != len(Lessons) {
		t.Fatalf("chapters cover %d lessons, course has %d", got, len(Lessons))
	}
}

// TestDeployMirrorsStandard pins this package's arrangement assembly to the
// room's: both sides arranging in the classic order must reproduce the exact
// standard starting position. That is the property the mirroring exists for,
// and it is the one an eye can check.
func TestDeployMirrorsStandard(t *testing.T) {
	white, ok := parseArrangement("nkpp")
	if !ok {
		t.Fatal("classic arrangement rejected")
	}
	// black is given in board order (files a..d), which for the standard
	// position is the mirror of a player-perspective "nkpp"
	black, ok := parseArrangement("ppkn")
	if !ok {
		t.Fatal("classic black arrangement rejected")
	}
	got, ok := assembleOFEN(white, black)
	if !ok {
		t.Fatal("assembly rejected the classic arrangement")
	}
	if got != standardStart {
		t.Fatalf("classic arrangement assembled to %q, want %q", got, standardStart)
	}
}

// TestParseArrangementRejectsIllegalArmies checks the deploy input guard: a
// learner's arrangement must be exactly one king, one knight and two pawns.
func TestParseArrangementRejectsIllegalArmies(t *testing.T) {
	for _, bad := range []string{"", "nkp", "nkppp", "nkpq", "kkpp", "nnpp", "nkpX"} {
		if _, ok := parseArrangement(bad); ok {
			t.Errorf("arrangement %q accepted, want rejected", bad)
		}
	}
	for _, good := range []string{"nkpp", "NKPP", "ppkn", "pnkp"} {
		if _, ok := parseArrangement(good); !ok {
			t.Errorf("arrangement %q rejected, want accepted", good)
		}
	}
}

// TestDoRejectsBadRequests checks the API guards a handler depends on.
func TestDoRejectsBadRequests(t *testing.T) {
	cases := []struct {
		name string
		req  Request
	}{
		{"unknown lesson", Request{Lesson: "nope"}},
		{"step out of range", Request{Lesson: "board", Step: 99}},
		{"negative step", Request{Lesson: "board", Step: -1}},
		{"malformed ofen", Request{Lesson: "pieces", Step: 2, OFEN: "not an ofen"}},
		{"illegal move", Request{Lesson: "pieces", Step: 2, UOI: "a1a4"}},
		{"bad deploy army", Request{Lesson: "deploy", Deploy: "kkkk"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Do(tc.req); err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
}

// TestDescribeOnly checks the call the client makes on entering a step: no
// move, just the position and its legal destinations.
func TestDescribeOnly(t *testing.T) {
	resp, err := Do(Request{Lesson: "pieces", Step: 2})
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if resp.Move != nil || resp.Reply != nil {
		t.Fatal("describe-only response carries a move")
	}
	if resp.OFEN != standardStart {
		t.Fatalf("described %q, want the step's start %q", resp.OFEN, standardStart)
	}
	if resp.Turn != "white" {
		t.Fatalf("turn %q, want white", resp.Turn)
	}
	if len(resp.Dests["c1"]) == 0 {
		t.Fatal("no destinations for the c1 pawn")
	}
}

// TestSoloKeepsTheMove checks the piece-movement drills: after a move that does
// not finish the step, the learner is still the side to move, so they can keep
// walking the same piece without an opponent replying.
func TestSoloKeepsTheMove(t *testing.T) {
	lesson, _ := BySlug("pieces")
	step := lesson.Steps[0]
	resp, err := Do(Request{Lesson: "pieces", Step: 0, OFEN: step.Start(), UOI: "a1a2"})
	if err != nil {
		t.Fatalf("solo move: %v", err)
	}
	if resp.Done {
		t.Fatal("step completed on the first of three steps")
	}
	if resp.Turn != "white" {
		t.Fatalf("turn %q after a solo move, want white", resp.Turn)
	}
	if resp.Reply != nil {
		t.Fatal("solo step produced an opponent reply")
	}
	if len(resp.Dests["a2"]) == 0 {
		t.Fatal("king has no destinations to keep walking with")
	}
}

// TestMoveBudgetFailsTheStep checks that a bounded puzzle rejects a wrong move
// immediately rather than letting the learner wander — the feedback that makes
// a mate-in-one worth setting.
func TestMoveBudgetFailsTheStep(t *testing.T) {
	lesson, _ := BySlug("check")
	step := lesson.Steps[1] // mate in one
	// a legal queen move that does not mate
	resp, err := Do(Request{Lesson: "check", Step: 1, OFEN: step.Start(), UOI: "c2c1"})
	if err != nil {
		t.Fatalf("wrong move: %v", err)
	}
	if resp.Done {
		t.Fatal("a non-mating move completed the mate step")
	}
	if !resp.Failed {
		t.Fatal("a wrong move inside a one-move budget did not fail the step")
	}
	if resp.Say == "" {
		t.Fatal("failed step said nothing")
	}
}

// TestSolutionIsOnlyLegalFromTheStart pins the reason "Show me" restarts the
// step before demonstrating. A wrong move leaves the board on a different
// position, and the step's solution is authored against the starting one — so
// replaying it from wherever the learner left off is an illegal move the server
// rejects. The client must reset first (showSolution in lio-learn.js); this test
// is what says the constraint is real rather than incidental.
func TestSolutionIsOnlyLegalFromTheStart(t *testing.T) {
	// the promotion step: a single authored move, and the lesson "Show me"
	// actually demonstrates
	lesson, _ := BySlug("promotion")
	step := lesson.Steps[0]
	if len(step.Solution) != 1 {
		t.Fatalf("expected a one-move solution to test against, got %v", step.Solution)
	}
	answer := step.Solution[0]

	// wander off with a legal move that does not solve it
	resp, err := Do(Request{Lesson: "promotion", Step: 0, OFEN: step.Start(), UOI: "a1a2"})
	if err != nil {
		t.Fatalf("wrong move: %v", err)
	}
	if resp.Done {
		t.Fatal("a king move completed the promotion step")
	}

	// the solution is not legal from where that left the board
	if _, err := Do(Request{
		Lesson: "promotion", Step: 0, OFEN: resp.OFEN, UOI: answer, Played: 1,
	}); err == nil {
		t.Fatal("the solution was accepted from the post-mistake position; " +
			"showSolution's reset would be unnecessary and this test wrong")
	}

	// ...and is legal, and completes the step, from the start
	fromStart, err := Do(Request{
		Lesson: "promotion", Step: 0, OFEN: step.Start(), UOI: answer,
	})
	if err != nil {
		t.Fatalf("solution from the step start: %v", err)
	}
	if !fromStart.Done {
		t.Fatal("the solution did not complete the step from its starting position")
	}
}

// TestPuzzleChaptersWithholdTheAnswer pins the choice made for the "Ending the
// game" chapter: those steps state no solution, so the learner gets no arrow
// pointing at the move, no "Show me", and — since the curriculum is what the
// page is built from — no answer anywhere in the HTML to read instead of
// thinking. TestCurriculum still proves each one solvable by searching.
func TestPuzzleChaptersWithholdTheAnswer(t *testing.T) {
	var checked int
	for _, l := range Lessons {
		if l.Chapter != "Ending the game" {
			continue
		}
		for si, s := range l.Steps {
			checked++
			if len(s.Solution) != 0 {
				t.Errorf("%s step %d states a solution %v; this chapter is worked out, not shown",
					l.Slug, si, s.Solution)
			}
			if s.Hint == "" {
				t.Errorf("%s step %d withholds the answer but offers no hint either",
					l.Slug, si)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no steps in the puzzle chapter; this test asserts nothing")
	}
}

// TestPromotionSuccessNamesTheChosenPiece checks the coach reports the piece the
// learner actually took. Underpromotion is a real choice, and being told you
// made a queen when you deliberately took a knight is simply wrong.
func TestPromotionSuccessNamesTheChosenPiece(t *testing.T) {
	lesson, _ := BySlug("promotion")
	step := lesson.Steps[0]
	if !strings.Contains(step.Success, piecePlaceholder) {
		t.Fatalf("promotion success line %q has no %s placeholder", step.Success, piecePlaceholder)
	}

	for uoi, want := range map[string]string{
		"c3c4q": "queen",
		"c3c4r": "rook",
		"c3c4b": "bishop",
		"c3c4n": "knight",
	} {
		t.Run(uoi, func(t *testing.T) {
			resp, err := Do(Request{Lesson: "promotion", Step: 0, OFEN: step.Start(), UOI: uoi})
			if err != nil {
				t.Fatalf("promote %s: %v", uoi, err)
			}
			if !resp.Done {
				t.Fatalf("promoting to %s did not complete the step", want)
			}
			if !strings.Contains(resp.Say, "A "+want+" out of nothing") {
				t.Fatalf("coach said %q, want it to name the %s", resp.Say, want)
			}
			if strings.Contains(resp.Say, piecePlaceholder) {
				t.Fatalf("placeholder leaked into %q", resp.Say)
			}
		})
	}
}

// TestPriorMovesAreWellFormed checks the annotated opponent moves: each names
// two real squares, and starts from a square that actually holds an enemy piece
// in the step's position — an arrow out of an empty square would be a lie the
// board draws in the learner's face.
func TestPriorMovesAreWellFormed(t *testing.T) {
	var annotated int
	for _, l := range Lessons {
		for si, s := range l.Steps {
			if s.PriorMove == "" {
				continue
			}
			annotated++
			if !validSquarePair(s.PriorMove) {
				t.Errorf("%s step %d: malformed PriorMove %q", l.Slug, si, s.PriorMove)
				continue
			}
			g, err := newGame(s.Start())
			if err != nil {
				t.Fatalf("%s step %d: %v", l.Slug, si, err)
			}
			dest := s.PriorMove[2:4]
			sq, ok := squareByName(dest)
			if !ok {
				t.Errorf("%s step %d: unknown square %q", l.Slug, si, dest)
				continue
			}
			piece := g.Position().Board().Piece(sq)
			if piece == octad.NoPiece {
				t.Errorf("%s step %d: PriorMove %q lands on empty %s",
					l.Slug, si, s.PriorMove, dest)
				continue
			}
			// the annotated move is the opponent's, so it must have moved one of
			// their pieces — the side that is *not* to move in this position
			if piece.Color() == g.Position().Turn() {
				t.Errorf("%s step %d: PriorMove %q moved a %s piece, but %s is to move",
					l.Slug, si, s.PriorMove, piece.Color(), g.Position().Turn())
			}
		}
	}
	if annotated == 0 {
		t.Fatal("no steps annotate a prior move; this test asserts nothing")
	}
}

// squareByName resolves a square name against octad's square list.
func squareByName(name string) (octad.Square, bool) {
	for sq := octad.A1; sq <= octad.D4; sq++ {
		if sq.String() == name {
			return sq, true
		}
	}
	return octad.NoSquare, false
}

// TestCastleTypesAreDistinguished checks the castle goals actually discriminate:
// the near castle must not satisfy the center step, and vice versa. Both are
// legal in the standard position, so a goal that only asked "did you castle"
// would pass this lesson without teaching the difference.
func TestCastleTypesAreDistinguished(t *testing.T) {
	lesson, _ := BySlug("castling")

	// the near step, answered with the center castle
	resp, err := Do(Request{Lesson: "castling", Step: 0, OFEN: lesson.Steps[0].Start(), UOI: "b1c1"})
	if err != nil {
		t.Fatalf("center castle in the near step: %v", err)
	}
	if resp.Done {
		t.Fatal("the center castle satisfied the near-castle step")
	}

	// the center step, answered with the near castle
	resp, err = Do(Request{Lesson: "castling", Step: 1, OFEN: lesson.Steps[1].Start(), UOI: "b1a1"})
	if err != nil {
		t.Fatalf("near castle in the center step: %v", err)
	}
	if resp.Done {
		t.Fatal("the near castle satisfied the center-castle step")
	}
}

// TestDeployRevealsAPosition checks the deploy step returns a playable position
// built from the learner's arrangement, with their pieces in the order they
// chose.
func TestDeployRevealsAPosition(t *testing.T) {
	resp, err := Do(Request{Lesson: "deploy", Deploy: "kpnp"})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if !resp.Done {
		t.Fatal("committing an arrangement did not complete the step")
	}
	rank1 := resp.OFEN[strings.LastIndex(resp.OFEN, "/")+1:]
	rank1 = strings.Split(rank1, " ")[0]
	if rank1 != "KPNP" {
		t.Fatalf("white home rank %q, want KPNP", rank1)
	}
	if len(resp.Dests) == 0 {
		t.Fatal("revealed position has no legal moves")
	}
}

// itoa keeps subtest names readable without pulling in strconv for one call.
func itoa(i int) string {
	return string(rune('0' + i))
}
