// Package demo generates short, self-contained random Octad games for the home
// page's "What is Octad?" explainer board. octadground (the client board) only
// renders positions — move generation, legality and outcome detection live in
// the octad library — so the games are produced here, server-side, and streamed
// to a small client animator (lio-home-demo.js) as a JSON batch.
//
// Each game starts from a randomized home-rank arrangement per side (the octad
// blind-deploy idea) and is played out with weighted-random moves that lean
// toward promotions and checks (the routes to a decisive Octad finish), always
// converting a forced mate, so the games stay lively and reach checkmates far
// more often than pure-uniform play (which almost always draws itself out).
//
// Generating a game is a full move-by-move playout — not free — so games are not
// made per request. A pool is built once per process (WarmPool) and each request
// samples a batch from it, the same per-process-constant approach the engine's
// deploy cache uses.
package demo

import (
	"strings"
	"sync"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/rng"
)

// Move-selection weights and the play cap are flavor knobs, not correctness.
// They deliberately favor the moves that make an Octad game reach a decisive
// finish: a checkmate needs a heavy piece, and the starting army (king, knight,
// two pawns) has none — mating material only arrives when a pawn promotes. So
// promotions and checks are weighted well above captures (heavy capturing just
// trades down to a king-and-knight insufficient-material draw). Quiet moves keep
// a baseline of 1 so nothing is ever unreachable. On top of the weights, a
// forced mate is always taken (see pickMove) — a random mover throws those away,
// which is the single biggest reason winning positions decay back to draws.
const (
	promoWeight   = 6 // a pawn promotion (the only route to mating material)
	checkWeight   = 3 // a move that gives check
	captureWeight = 2 // a capturing move (incl. en passant)
	quietWeight   = 1 // any other legal move

	// maxPlies bounds a game defensively. Natural termination (the 25-move rule,
	// threefold repetition, stalemate, insufficient material) fires well before
	// this on a 4x4 board, so the cap is only a backstop against a pathological
	// non-terminating line; a capped game is reported as a generic draw.
	maxPlies = 120
)

// Ply is one half-move of a demo game: the UOI move string, the full OFEN of the
// resulting position, and whether that position leaves the side to move in check
// (OFEN doesn't encode check, and the client has no rules engine to derive it, so
// it must ride along for the board's check highlight). The client derives the
// board, side-to-move and last-move highlight from U and O.
type Ply struct {
	U string `json:"u"`           // UOI move, e.g. "c2c3" (promotions carry a 5th char)
	O string `json:"o"`           // full OFEN after the move
	K bool   `json:"k,omitempty"` // side to move is in check after the move
}

// Game is a complete demo game: the starting position, every half-move played,
// and the result. Winner is "w"/"b"/"d"; Method is a short reason code
// ("checkmate", "stalemate", "repetition", "moverule", "insufficient") or "" for
// a generic/undetermined draw.
type Game struct {
	Start  string `json:"start"`
	Plies  []Ply  `json:"plies"`
	Winner string `json:"winner"`
	Method string `json:"method"`
}

// poolSize is how many games the process-level pool holds. A batch is sampled
// from it, so sampling 12 leaves ample room before a repeat while keeping the
// one-time build fast.
const poolSize = 96

var (
	poolOnce sync.Once
	pool     []Game
)

// WarmPool builds the demo game pool if it hasn't been built yet. Safe to call
// concurrently and repeatedly (sync.Once). The server kicks this off at startup
// so the first /home/demo request doesn't pay the build; Batch calls it too as a
// lazy fallback.
func WarmPool() {
	poolOnce.Do(func() { pool = buildPool(poolSize) })
}

// Batch returns n demo games sampled at random from the process-level pool
// (built once via WarmPool). Serving a request is then just a random sample, so
// the /home/demo path stays cheap. The pool is curated toward ~half decisive
// games, so any sample shows a lively mix rather than a run of draws.
func Batch(n int) []Game {
	if n <= 0 {
		n = 1
	}
	WarmPool()
	return sample(pool, n)
}

// buildPool generates size games curated toward a lively ~50/50 decisive/draw
// mix — unguided play draws ~70% of the time, which would loop the demo through
// dull runs of draws. It overgenerates within a bounded budget, buckets by
// result, then assembles exactly size games (topping up from either bucket if
// one comes short).
func buildPool(size int) []Game {
	targetDecisive := size / 2
	// bounded overgeneration: the natural decisive rate sits above the target
	// ratio so this usually breaks early; the ceiling caps the worst case.
	maxAttempts := size * 6

	var decisive, draws []Game
	for i := 0; i < maxAttempts; i++ {
		if len(decisive) >= targetDecisive && len(decisive)+len(draws) >= size {
			break
		}
		g := generateGame()
		if g.Winner == "d" {
			draws = append(draws, g)
		} else {
			decisive = append(decisive, g)
		}
	}

	out := make([]Game, 0, size)
	nd := targetDecisive
	if nd > len(decisive) {
		nd = len(decisive)
	}
	out = append(out, decisive[:nd]...)
	for i := 0; len(out) < size && i < len(draws); i++ {
		out = append(out, draws[i])
	}
	for i := nd; len(out) < size && i < len(decisive); i++ {
		out = append(out, decisive[i])
	}
	return out
}

// sample returns n games chosen uniformly at random (without replacement) from
// games, as a fresh slice — a partial Fisher-Yates over a copy, so the shared
// pool is never mutated and concurrent callers never race. Fewer than n games
// available returns all of them, shuffled.
func sample(games []Game, n int) []Game {
	out := make([]Game, len(games))
	copy(out, games)
	if n > len(out) {
		n = len(out)
	}
	for i := 0; i < n; i++ {
		j := i + rng.Intn(len(out)-i)
		out[i], out[j] = out[j], out[i]
	}
	return out[:n]
}

// generateGame assembles a random start position and plays it out with
// weighted-random moves until the octad library reports an outcome (or the ply
// cap trips).
func generateGame() Game {
	start := randomStart()
	g := mustGame(start)

	plies := make([]Ply, 0, 32)
	for g.Outcome() == octad.NoOutcome && len(plies) < maxPlies {
		moves := g.ValidMoves()
		if len(moves) == 0 {
			break // terminal position; Outcome/Method already set
		}
		m := pickMove(g, moves)
		if err := g.Move(m); err != nil {
			break // impossible for a move drawn from ValidMoves, but stay safe
		}
		plies = append(plies, Ply{
			U: m.String(),
			O: g.Position().String(),
			K: g.Position().InCheck(),
		})
	}

	return Game{
		Start:  start,
		Plies:  plies,
		Winner: winnerCode(g.Outcome()),
		Method: methodCode(g.Method()),
	}
}

// pickMove chooses the move to play. A forced checkmate is always taken (random
// play otherwise squanders won positions back into draws); failing that, a move
// is drawn at weighted random, leaning toward promotions and checks so the game
// trends toward a decisive finish while still looking organic. A move that fits
// several buckets takes its highest applicable weight.
func pickMove(g *octad.Game, moves []*octad.Move) *octad.Move {
	// convert a mate-in-one if one exists (only checking moves can mate, so the
	// clone-and-test is limited to them)
	for _, m := range moves {
		if m.HasTag(octad.Check) && deliversMate(g, m) {
			return m
		}
	}

	weights := make([]int, len(moves))
	total := 0
	for i, m := range moves {
		w := quietWeight
		switch {
		case m.Promo() != octad.NoPieceType:
			w = promoWeight
		case m.HasTag(octad.Check):
			w = checkWeight
		case m.HasTag(octad.Capture) || m.HasTag(octad.EnPassant):
			w = captureWeight
		}
		weights[i] = w
		total += w
	}

	r := rng.Intn(total)
	for i, w := range weights {
		if r < w {
			return moves[i]
		}
		r -= w
	}
	return moves[len(moves)-1] // unreachable: r < total always resolves above
}

// deliversMate reports whether playing m checkmates the opponent. It plays the
// move and immediately takes it back (UndoMove restores the exact prior position
// via updatePosition), which is far cheaper than cloning the whole game history
// per candidate. m comes from the current position's cached move list, which
// UndoMove leaves intact, so the caller's move slice stays valid.
func deliversMate(g *octad.Game, m *octad.Move) bool {
	if err := g.Move(m); err != nil {
		return false
	}
	mate := g.Method() == octad.Checkmate
	g.UndoMove()
	return mate
}

// randomStart builds a white-to-move starting OFEN with a random home-rank
// arrangement for each side. White occupies rank 1, black rank 4, both retaining
// full castle rights (no piece has moved) — the same assembly the engine uses
// for blind-deploy openings (engine/deploy.go deployOFEN).
func randomStart() string {
	return randomRank(false) + "/4/4/" + randomRank(true) + " w NCFncf - 0 1"
}

// randomRank returns one side's four home-rank squares in board order (file a..d)
// as OFEN letters: one king, one knight, two pawns, placed randomly. Uppercase
// for white, lowercase for black.
func randomRank(white bool) string {
	// choosing the king square and a distinct knight square (the other two are
	// pawns) uniformly covers every distinct arrangement; the "shift past the
	// king" maps the 3 remaining slots to a uniform non-king square.
	kingSq := rng.Intn(4)
	knightSq := rng.Intn(3)
	if knightSq >= kingSq {
		knightSq++
	}

	sq := [4]byte{'P', 'P', 'P', 'P'}
	sq[kingSq] = 'K'
	sq[knightSq] = 'N'

	s := string(sq[:])
	if !white {
		return strings.ToLower(s)
	}
	return s
}

// winnerCode maps an octad outcome to the wire winner code. A game that hit the
// ply cap without a result reads as a draw ("d").
func winnerCode(o octad.Outcome) string {
	switch o {
	case octad.WhiteWon:
		return "w"
	case octad.BlackWon:
		return "b"
	default:
		return "d"
	}
}

// methodCode maps an octad draw/win method to a short wire reason code, or "" for
// a method the client renders as a generic draw.
func methodCode(m octad.Method) string {
	switch m {
	case octad.Checkmate:
		return "checkmate"
	case octad.Stalemate:
		return "stalemate"
	case octad.ThreefoldRepetition:
		return "repetition"
	case octad.TwentyFiveMoveRule:
		return "moverule"
	case octad.InsufficientMaterial:
		return "insufficient"
	default:
		return ""
	}
}

// mustGame builds a game from a demo-constructed OFEN. randomStart always
// produces a well-formed, legal, non-terminal white-to-move position, so a parse
// error here is a programming bug (mirrors engine/deploy.go mustGame).
func mustGame(ofen string) *octad.Game {
	fn, err := octad.OFEN(ofen)
	if err != nil {
		panic(err)
	}
	g, err := octad.NewGame(fn)
	if err != nil {
		panic(err)
	}
	return g
}
