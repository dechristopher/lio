package view

import "github.com/dechristopher/lio/learn"

// The /learn tutorial page's client model. The rail and the first lesson's
// prompt are rendered server-side (so the page is readable and complete before
// any script runs), and the same curriculum is inlined as JSON for lio-learn.js
// to drive every lesson change without a fetch. Only what the client actually
// uses is included — the authored positions' Setup sequences and the coach's
// success lines stay on the server, the latter because the server is what
// decides a step was passed.

// LearnModel is the payload inlined as #learn-data.
type LearnModel struct {
	Lessons []LearnLesson `json:"lessons"`
	// Start is the slug the page was opened at, so the client knows which
	// lesson the server already rendered.
	Start string `json:"start"`
}

// LearnLesson is one lesson as the client needs it.
type LearnLesson struct {
	Slug    string      `json:"slug"`
	Title   string      `json:"title"`
	Chapter string      `json:"chapter"`
	Kind    string      `json:"kind"`
	Steps   []LearnStep `json:"steps"`
}

// LearnStep is one step as the client needs it: where the board starts, what
// the coach says on entry, and enough about the goal to run the board's input
// mode. Judging stays on the server — the client never decides a step was
// passed, with the single exception of the click-a-square board-literacy step,
// which makes no move for the server to judge.
type LearnStep struct {
	OFEN string `json:"o"`
	// Dests is the starting position's legal-move map, shipped so the board is
	// interactive the moment the step opens rather than one round trip later.
	Dests  map[string][]string `json:"v,omitempty"`
	Prompt string              `json:"prompt"`
	// Action is the instruction, rendered in the accent colour after the
	// prompt so what the board is waiting for survives a skim.
	Action  string   `json:"action,omitempty"`
	Hint    string   `json:"hint,omitempty"`
	Goal    string   `json:"goal"`
	Targets []string `json:"targets,omitempty"`
	// Solution is what the "Show me" button plays back, and what the board's
	// move marks are derived from client-side (the circled piece and the arrow).
	// It is deliberately in the page: this is a lesson, not a puzzle to be
	// defended, and having it client-side makes both instant.
	Solution []string `json:"solution,omitempty"`
	// PriorMove is the opponent move that produced this position, drawn as a
	// blue arrow until the learner moves (see the learn package).
	PriorMove string `json:"prior,omitempty"`
	// Success is the coach's line on completion, and it is sent for the
	// click-a-square step only. That step makes no move, so there is no request
	// for the server to answer and nothing else could ever say it. Every other
	// step's success line stays on the server, which is what decides the step
	// was passed and says so in its reply.
	Success string `json:"success,omitempty"`
	// Moves is the step's move budget, 0 for unlimited. The client shows it as
	// the "in one move" chip; the server is what enforces it.
	Moves int  `json:"moves,omitempty"`
	Solo  bool `json:"solo,omitempty"`
}

// buildLearnModel projects the curriculum into the client model.
func buildLearnModel(start string) LearnModel {
	m := LearnModel{Start: start}
	for i := range learn.Lessons {
		l := &learn.Lessons[i]
		cl := LearnLesson{
			Slug:    l.Slug,
			Title:   l.Title,
			Chapter: l.Chapter,
			Kind:    string(l.Kind),
		}
		for _, s := range l.Steps {
			cs := LearnStep{
				OFEN:      s.Start(),
				Dests:     s.StartDests(),
				Prompt:    s.Prompt,
				Action:    s.Action,
				Hint:      s.Hint,
				Goal:      string(s.Goal),
				Targets:   s.Targets,
				Solution:  s.Solution,
				PriorMove: s.PriorMove,
				Moves:     s.Moves,
				Solo:      s.Solo,
			}
			if s.Goal == learn.GoalSelect {
				cs.Success = s.Success
			}
			cl.Steps = append(cl.Steps, cs)
		}
		m.Lessons = append(m.Lessons, cl)
	}
	return m
}

// learnChapters exposes the grouped course to the templ rail.
func learnChapters() []learn.Chapter { return learn.Chapters() }

// learnStepCount is the number of steps in a lesson, for the rail's step dots.
func learnStepCount(l *learn.Lesson) int { return len(l.Steps) }

// learnFirstPrompt / learnFirstAction are the opening lesson's first step,
// rendered into the coach panel server-side so the page reads correctly before
// lio-learn.js runs.
func learnFirstPrompt(l *learn.Lesson) string {
	if l == nil || len(l.Steps) == 0 {
		return ""
	}
	return l.Steps[0].Prompt
}

func learnFirstAction(l *learn.Lesson) string {
	if l == nil || len(l.Steps) == 0 {
		return ""
	}
	return l.Steps[0].Action
}
