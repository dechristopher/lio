package engine

import (
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/rng"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// Persona is a named bot difficulty: a bundle of search and move-selection
// handicaps. Rooms store a persona by Key (room.Params.BotPersona) and the
// engine applies it at move time via SearchPersona. Depth limiting alone can't
// make a beatable bot — even a depth-1 search on a 4x4 board never hangs
// material — so the low rungs combine a shallow horizon with imperfect
// selection among the evaluated root moves and outright blunders.
type Persona struct {
	// Key is the persona's stable identifier: the form/query value, the room
	// snapshot field, and the games.bot_persona archive stamp.
	Key string
	// Name / Glyph / Blurb / Strength are the display surface: the difficulty
	// modal cards and the bot's seat label. Glyph is a unicode chess piece with
	// U+FE0E appended to force text presentation (never emoji).
	Glyph string
	Name  string
	Blurb string
	// Strength is the 1-based rating shown as filled pips out of len(Personas).
	Strength int

	// MaxDepth caps the room's time-control-derived search depth ceiling
	// (0 = no cap): the horizon handicap — a shallow bot genuinely doesn't
	// see deep tactics coming.
	MaxDepth int
	// VarietyMoves is the size of the top-N root-move pool a move is picked
	// from on every move (1 = always the single best move). It generalizes the
	// opening-variety pick to the whole game for imperfect, human-ish play.
	VarietyMoves int
	// VarietyMargin bounds how far (eval units, a pawn is 10) a picked move may
	// trail the best move, so variety never selects an outright lost move —
	// mate scores blow past any margin and are always excluded.
	VarietyMargin float64
	// BlunderRate is the chance a move is instead picked uniformly at random
	// from all legal moves. This is the lever that actually makes the low
	// rungs losable: it hangs pieces and misses mates the way beginners do.
	BlunderRate float64
	// TimeReserve is the fraction of the bot's initial clock it banks rather
	// than spending on thinking (0 = the room's DefaultBotTimeReserve): high
	// reserves make the bot move fast and search shallow within its
	// already-capped depth. The room reads it in calcSearchLocked.
	TimeReserve float64
	// RandomDeploy deploys a uniformly random home rank instead of an
	// engine-scored one (see RandomDeployment) — about a third of random
	// arrangements are materially inferior.
	RandomDeploy bool
}

// Personas is the fixed difficulty ladder, weakest first. The chess pieces
// double as names and strength ordering; richer named characters can replace
// them later without touching the parameter model.
var Personas = []Persona{
	{
		Key:           "pawn",
		Glyph:         "♟︎",
		Name:          "Pawn",
		Blurb:         "Just learned the rules. Hangs pieces and misses mates. A perfect first opponent.",
		Strength:      1,
		MaxDepth:      1,
		VarietyMoves:  6,
		VarietyMargin: 50,
		BlunderRate:   0.35,
		TimeReserve:   0.85,
		RandomDeploy:  true,
	},
	{
		Key:           "knight",
		Glyph:         "♞︎",
		Name:          "Knight",
		Blurb:         "Knows the moves but forgets the plan. Punishes big mistakes but makes plenty of its own.",
		Strength:      2,
		MaxDepth:      2,
		VarietyMoves:  5,
		VarietyMargin: 25,
		BlunderRate:   0.12,
		TimeReserve:   0.65,
		RandomDeploy:  true,
	},
	{
		Key:           "bishop",
		Glyph:         "♝︎",
		Name:          "Bishop",
		Blurb:         "A solid club player. Sees short tactics, but patient play wins out.",
		Strength:      3,
		MaxDepth:      4,
		VarietyMoves:  3,
		VarietyMargin: 10,
		TimeReserve:   0.4,
	},
	{
		Key:           "rook",
		Glyph:         "♜︎",
		Name:          "Rook",
		Blurb:         "Sharp and unforgiving. Few mistakes go unpunished.",
		Strength:      4,
		MaxDepth:      6,
		VarietyMoves:  2,
		VarietyMargin: 4,
		TimeReserve:   0.25,
	},
	{
		Key:      "queen",
		Glyph:    "♛︎",
		Name:     "Queen",
		Blurb:    "The full engine at maximum strength. Ruthless, near-perfect play.",
		Strength: 5,
		// no caps, no variety, no blunders: today's bot, unchanged
		VarietyMoves: 1,
	},
}

// PersonaByKey resolves a persona key to its definition. Unknown and empty
// keys resolve to the full-strength Queen: pre-persona rooms, snapshots, and
// archived games all predate the ladder and played at exactly that strength.
func PersonaByKey(key string) Persona {
	for _, p := range Personas {
		if p.Key == key {
			return p
		}
	}
	return Personas[len(Personas)-1]
}

// fullStrength reports whether the persona applies no selection handicap, so
// the search can take the untouched full-strength path (including its
// opening-variety pick).
func (p Persona) fullStrength() bool {
	return p.VarietyMoves <= 1 && p.BlunderRate == 0
}

// capDepth applies the persona's search-depth ceiling to the caller's
// time-control-derived depth.
func (p Persona) capDepth(depth int) int {
	if p.MaxDepth > 0 && depth > p.MaxDepth {
		return p.MaxDepth
	}
	return depth
}

// searchPersonaAB is the persona-handicapped counterpart of searchMinimaxAB:
// the same paced sleep and (budgeted) root search, but the move is picked
// imperfectly — a BlunderRate roll falls through to a uniform random legal
// move, and everything else picks among the top VarietyMoves root moves within
// VarietyMargin of best on every move of the game (which subsumes the opening
// variety of the full-strength path).
func searchPersonaAB(situation *octad.Game, depth int, deadline time.Time, repHist map[string]int, p Persona) MoveEval {
	handicapSleep(deadline)

	if p.BlunderRate > 0 && rng.Float64() < p.BlunderRate {
		moves := situation.ValidMoves()
		choice := MoveEval{Move: *moves[rng.Intn(len(moves))]}
		util.DebugFlag("engine", str.CEval, "persona %s blundered: %s for OFEN: %s",
			p.Key, choice.Move.String(), situation.Position().String())
		return choice
	}

	var results []MoveEval
	if deadline.IsZero() {
		moves := orderMoves(situation)
		results = evaluateRootMoves(situation, moves, depth, noStop, repHist)
	} else {
		_, results = deepeningRoot(situation, depth, deadline, repHist)
	}
	if len(results) == 0 {
		// no moves searched (shouldn't happen for a live position); defer to
		// the standard best-move logic and its losing-position fallback
		return minimaxABRoot(situation, depth, repHist)
	}

	return pickAmong(situation, results, p.VarietyMoves, p.VarietyMargin)
}
