package engine

import (
	"sort"
	"strings"
	"sync"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/rng"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// DeploySearchDepth is the alpha-beta depth used to score each candidate deploy
// position. The blind deploy scorer evaluates 12x12 = 144 opening positions, so
// the depth is kept shallow: enough to surface an immediate tactical imbalance
// between arrangements (a hung pawn, an early fork) on top of the static
// positional signal, without paying for a deep search 144 times. It is a var,
// not a const, so tests can lower it (mirrors room's deployTimeout); it is a
// strength/cost knob, not correctness.
var DeploySearchDepth = 3

// DeployVarietyMargin caps how far (in eval units, where a pawn is worth 10) a
// candidate deployment may trail the best-scoring one and still be eligible for
// the random pick. It keeps the bot's blind arrangement varied game-to-game
// without ever choosing an outright inferior setup — the deploy analogue of
// OpeningVarietyMargin.
const DeployVarietyMargin float64 = 12

// DeployPlacement is a home-rank arrangement of the four octad pieces (one king,
// one knight, two pawns) in board order: index i is file a+i on the deploying
// color's home rank (rank 1 for white, rank 4 for black). It is the
// board-absolute analogue of room's player-perspective Deployment — the engine
// reasons about actual squares, and the caller maps the result to its own
// per-player perspective.
type DeployPlacement [4]octad.PieceType

// String renders a placement as its four board-order piece letters (K/N/P),
// e.g. "NKPP" for the classic arrangement. Used for debug logging.
func (p DeployPlacement) String() string {
	var b strings.Builder
	for _, pt := range p {
		b.WriteString(deployOFENChar(pt, octad.White))
	}
	return b.String()
}

// deployPlacements returns the 12 distinct legal home-rank arrangements of the
// octad army (4!/2! = 12, because the two pawns are interchangeable). Choosing
// the king square and the (distinct) knight square uniquely determines an
// arrangement — the remaining two squares are pawns — so the 4x3 = 12 ordered
// (king, knight) square pairs enumerate every arrangement exactly once.
func deployPlacements() []DeployPlacement {
	out := make([]DeployPlacement, 0, 12)
	for kingSq := 0; kingSq < 4; kingSq++ {
		for knightSq := 0; knightSq < 4; knightSq++ {
			if knightSq == kingSq {
				continue
			}
			p := DeployPlacement{octad.Pawn, octad.Pawn, octad.Pawn, octad.Pawn}
			p[kingSq] = octad.King
			p[knightSq] = octad.Knight
			out = append(out, p)
		}
	}
	return out
}

// deployOFENChar returns the OFEN letter for a deployable piece type and color
// (uppercase for white, lowercase for black).
func deployOFENChar(pt octad.PieceType, c octad.Color) string {
	var s string
	switch pt {
	case octad.King:
		s = "K"
	case octad.Knight:
		s = "N"
	case octad.Pawn:
		s = "P"
	default:
		return ""
	}
	if c == octad.Black {
		return strings.ToLower(s)
	}
	return s
}

// deployOFEN builds the white-to-move starting OFEN for a fully deployed
// position from white's and black's board-order placements. White occupies rank
// 1 (files a..d), black rank 4 (files a..d); both retain full castle rights (no
// piece has moved). Board order means there is no perspective mirroring here —
// index i is always file a+i — which is the engine's natural frame; room mirrors
// per player only for the client protocol.
func deployOFEN(white, black DeployPlacement) string {
	var rank1, rank4 strings.Builder
	for i := 0; i < 4; i++ {
		rank1.WriteString(deployOFENChar(white[i], octad.White))
		rank4.WriteString(deployOFENChar(black[i], octad.Black))
	}
	return rank4.String() + "/4/4/" + rank1.String() + " w NCFncf - 0 1"
}

// positionValue returns the absolute white-positive minimax value of a position
// under alpha-beta search to the given depth. It is the sequential counterpart
// to minimaxABRoot's returned Eval (without the per-root-move goroutine
// fan-out), used to score the many candidate deploy positions cheaply. White
// maximizes and black minimizes, reusing mmABMax/mmABMin, which already keep the
// score in the single absolute white-positive space.
func positionValue(node *octad.Game, depth int) float64 {
	moves := node.ValidMoves()

	if depth == 0 || len(moves) == 0 {
		// Evaluate is side-to-move-relative; convert to white-positive
		eval := Evaluate(node)
		if node.Position().Turn() == octad.Black {
			return -eval
		}
		return eval
	}

	// moves[0] is only used by mmABMax/mmABMin for debug logging of the last
	// move; a real move keeps their depth-0 String() call safe. Deploy scoring
	// starts from a fresh game, so there is no repetition history to track.
	if node.Position().Turn() == octad.White {
		return mmABMax(node, moves[0], depth, -WinVal, WinVal, noStop, nil)
	}
	return mmABMin(node, moves[0], depth, -WinVal, WinVal, noStop, nil)
}

// scoredPlacement pairs one of a color's candidate placements with its expected
// white-positive value against an unknown opponent.
type scoredPlacement struct {
	placement DeployPlacement
	score     float64
}

// scoreDeployments returns each of color's 12 candidate placements paired with
// its expected white-positive value: the mean, over all 12 possible opponent
// placements, of the depth-limited minimax value of the resulting opening
// position. The deploy is blind and simultaneous, so the opponent's arrangement
// is unknown; averaging uniformly over every legal opponent arrangement is the
// expected value against an unmodeled opponent. Every assembled position is
// white-to-move, so the returned scores share one frame and are directly
// comparable. The list is in deterministic enumeration order.
func scoreDeployments(color octad.Color, depth int) []scoredPlacement {
	placements := deployPlacements()

	// score all 12x12 assembled openings in parallel — each builds its own
	// game from an OFEN, so the searches share no state. Mirrors the move
	// search's per-root-move fan-out. Each goroutine writes only its own cell
	// of the value grid; summing the grid sequentially afterward keeps the
	// floating-point accumulation order (and thus the scores) deterministic.
	values := make([][]float64, len(placements))
	wg := &sync.WaitGroup{}
	for i, mine := range placements {
		values[i] = make([]float64, len(placements))
		for j, theirs := range placements {
			var ofen string
			if color == octad.White {
				ofen = deployOFEN(mine, theirs)
			} else {
				ofen = deployOFEN(theirs, mine)
			}
			wg.Add(1)
			go func(i, j int, ofen string) {
				defer wg.Done()
				values[i][j] = positionValue(mustGame(ofen), depth)
			}(i, j, ofen)
		}
	}
	wg.Wait()

	scored := make([]scoredPlacement, len(placements))
	for i, mine := range placements {
		var sum float64
		for _, v := range values[i] {
			sum += v
		}
		scored[i] = scoredPlacement{
			placement: mine,
			score:     sum / float64(len(placements)),
		}
	}

	return scored
}

// deployCache holds each color's candidate placement list. The scoring is
// blind — a pure function of the color and DeploySearchDepth with no game-state
// input — so the list is a per-process constant: the 144-search scoring runs
// once per color (eagerly via WarmDeployCache when the engine comes online, or
// lazily on the first deploy that beats it) and every subsequent deploy is just
// a random pick from the cached list.
var deployCache = map[octad.Color]*deployCacheEntry{
	octad.White: {},
	octad.Black: {},
}

type deployCacheEntry struct {
	once       sync.Once
	candidates []DeployPlacement
}

// WarmDeployCache eagerly computes both colors' deploy candidate lists so the
// first bot deploy of the process doesn't pay the scoring cost. It is invoked
// by dispatch.UpEngine when the engine is brought online; instances that never
// run the engine never pay for the cache.
func WarmDeployCache() {
	for _, color := range []octad.Color{octad.White, octad.Black} {
		deployCandidates(color)
	}
}

// deployCandidates returns color's cached candidate placements, computing and
// caching them on first use at the DeploySearchDepth in effect at that moment.
func deployCandidates(color octad.Color) []DeployPlacement {
	entry := deployCache[color]
	entry.once.Do(func() {
		entry.candidates = candidatePlacements(color, DeploySearchDepth)
	})
	return entry.candidates
}

// SelectDeployment chooses the bot's blind home-rank arrangement for the given
// color: a random pick from the cached candidate list (see selectDeployment
// for the underlying scoring and filtering).
func SelectDeployment(color octad.Color) DeployPlacement {
	candidates := deployCandidates(color)
	choice := candidates[rng.Intn(len(candidates))]

	util.DebugFlag("engine", str.CEval, "deploy(%s): chose %s from %d cached candidates",
		color, choice, len(candidates))

	return choice
}

// RandomDeployment returns a uniformly random legal home-rank arrangement,
// skipping the expected-value filter entirely. It is the easy-difficulty
// deploy: a third of the arrangements trail the best by two-plus pawns of
// expected value, so an unfiltered pick is a genuine handicap on top of being
// maximally varied.
func RandomDeployment() DeployPlacement {
	placements := deployPlacements()
	return placements[rng.Intn(len(placements))]
}

// selectDeployment picks randomly among candidatePlacements — the uncached
// core of SelectDeployment, exercised directly by tests at custom depths.
func selectDeployment(color octad.Color, depth int) DeployPlacement {
	candidates := candidatePlacements(color, depth)
	choice := candidates[rng.Intn(len(candidates))]

	util.DebugFlag("engine", str.CEval, "deploy(%s): chose %s from %d candidates",
		color, choice, len(candidates))

	return choice
}

// candidatePlacements scores every candidate placement by its expected value
// against an unknown opponent, then returns those within DeployVarietyMargin
// of the best (white wants the highest white-positive score, black the lowest),
// best-first, so a random pick among them varies the bot's arrangement
// game-to-game without ever choosing an outright inferior setup — the deploy
// analogue of pickOpeningMove. Placements are in board order; the caller maps
// them to its own perspective.
func candidatePlacements(color octad.Color, depth int) []DeployPlacement {
	scored := scoreDeployments(color, depth)

	// sort best-first from color's perspective
	sort.SliceStable(scored, func(i, j int) bool {
		if color == octad.White {
			return scored[i].score > scored[j].score
		}
		return scored[i].score < scored[j].score
	})
	best := scored[0].score

	// keep every placement within the variety margin of the best; the list is
	// sorted, so stop at the first that falls short
	candidates := make([]DeployPlacement, 0, len(scored))
	for _, s := range scored {
		within := (color == octad.White && s.score >= best-DeployVarietyMargin) ||
			(color == octad.Black && s.score <= best+DeployVarietyMargin)
		if !within {
			break
		}
		candidates = append(candidates, s.placement)
	}

	return candidates
}

// mustGame builds a game from an engine-constructed OFEN. The OFEN is always
// well-formed (assembled by deployOFEN), so a parse error is a programming bug.
func mustGame(ofen string) *octad.Game {
	o, err := octad.OFEN(ofen)
	if err != nil {
		panic(err)
	}
	g, err := octad.NewGame(o)
	if err != nil {
		panic(err)
	}
	return g
}
