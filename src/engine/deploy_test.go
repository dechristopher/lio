package engine

import (
	"testing"

	"github.com/dechristopher/octad/v2"
)

// armyOf counts the pieces in a placement.
func armyOf(p DeployPlacement) (kings, knights, pawns, other int) {
	for _, pt := range p {
		switch pt {
		case octad.King:
			kings++
		case octad.Knight:
			knights++
		case octad.Pawn:
			pawns++
		default:
			other++
		}
	}
	return
}

// TestDeployPlacementsEnumeration confirms the enumeration yields exactly the 12
// distinct legal home-rank arrangements, each a legal octad army.
func TestDeployPlacementsEnumeration(t *testing.T) {
	placements := deployPlacements()
	if len(placements) != 12 {
		t.Fatalf("deployPlacements() returned %d arrangements, want 12", len(placements))
	}

	seen := make(map[DeployPlacement]bool, 12)
	for _, p := range placements {
		if seen[p] {
			t.Errorf("duplicate placement %s", p)
		}
		seen[p] = true

		kings, knights, pawns, other := armyOf(p)
		if kings != 1 || knights != 1 || pawns != 2 || other != 0 {
			t.Errorf("placement %s is not a legal army (K=%d N=%d P=%d other=%d)",
				p, kings, knights, pawns, other)
		}
	}
}

// TestDeployOFENIsPlayable confirms every assembled deploy position parses as a
// legal, white-to-move octad game with legal moves.
func TestDeployOFENIsPlayable(t *testing.T) {
	placements := deployPlacements()
	for _, w := range placements {
		for _, b := range placements {
			ofen := deployOFEN(w, b)
			g := mustGame(ofen) // panics on a malformed OFEN
			if g.Position().Turn() != octad.White {
				t.Fatalf("deployOFEN(%s,%s) = %q: not white to move", w, b, ofen)
			}
			if len(g.ValidMoves()) == 0 {
				t.Fatalf("deployOFEN(%s,%s) = %q: no legal moves", w, b, ofen)
			}
		}
	}
}

// TestStandardStartValue sanity-checks the scorer: the fully symmetric standard
// start (both sides NKPP in board order -> the canonical position) evaluates
// near zero — neither side has a material or structural edge, only white's small
// side-to-move tempo. It also proves positionValue stays in the absolute
// white-positive frame (a symmetric position must not land far from 0).
func TestStandardStartValue(t *testing.T) {
	// board-order NKPP is the canonical starting arrangement for both colors
	std := DeployPlacement{octad.Knight, octad.King, octad.Pawn, octad.Pawn}
	v := positionValue(mustGame(deployOFEN(std, std)), 2)
	if v < -30 || v > 30 {
		t.Fatalf("symmetric standard start value = %.2f, want within +/-30 of 0", v)
	}
}

// TestScoreDeploymentsDeterministic confirms scoring is a pure function of the
// color and depth (the search has no randomness), so the ranking that drives
// SelectDeployment is stable.
func TestScoreDeploymentsDeterministic(t *testing.T) {
	for _, color := range []octad.Color{octad.White, octad.Black} {
		a := scoreDeployments(color, 2)
		b := scoreDeployments(color, 2)
		if len(a) != 12 || len(b) != 12 {
			t.Fatalf("scoreDeployments(%s) len = %d/%d, want 12", color, len(a), len(b))
		}
		for i := range a {
			if a[i].placement != b[i].placement || a[i].score != b[i].score {
				t.Fatalf("scoreDeployments(%s) not deterministic at %d: %v(%.4f) vs %v(%.4f)",
					color, i, a[i].placement, a[i].score, b[i].placement, b[i].score)
			}
		}
	}
}

// TestSelectDeploymentCached confirms the cached production path: warming
// computes each color's candidate list once, and SelectDeployment only ever
// returns members of that cached list.
func TestSelectDeploymentCached(t *testing.T) {
	WarmDeployCache()
	for _, color := range []octad.Color{octad.White, octad.Black} {
		candidates := deployCandidates(color)
		if len(candidates) == 0 {
			t.Fatalf("deployCandidates(%s) is empty", color)
		}

		set := make(map[DeployPlacement]bool, len(candidates))
		for _, c := range candidates {
			set[c] = true
		}
		for i := 0; i < 15; i++ {
			if p := SelectDeployment(color); !set[p] {
				t.Fatalf("SelectDeployment(%s) = %s: not in the cached candidate list %v",
					color, p, candidates)
			}
		}
	}
}

// TestRandomDeploymentValid confirms the easy-difficulty deploy returns only
// legal enumerated arrangements.
func TestRandomDeploymentValid(t *testing.T) {
	legal := make(map[DeployPlacement]bool, 12)
	for _, p := range deployPlacements() {
		legal[p] = true
	}
	for i := 0; i < 50; i++ {
		if p := RandomDeployment(); !legal[p] {
			t.Fatalf("RandomDeployment() = %s: not among enumerated placements", p)
		}
	}
}

// TestSelectDeploymentValid confirms selectDeployment always returns a legal
// army and only ever chooses a candidate within the variety margin of the best
// (never an outright inferior arrangement), for both colors, across repeated
// randomized picks.
func TestSelectDeploymentValid(t *testing.T) {
	// depth 1 keeps the repeated 144-position selections cheap; the pick logic
	// (validity + variety margin) is independent of search depth.
	const depth = 1
	for _, color := range []octad.Color{octad.White, octad.Black} {
		scored := scoreDeployments(color, depth)
		best := scored[0].score
		for _, s := range scored {
			if color == octad.White && s.score > best {
				best = s.score
			}
			if color == octad.Black && s.score < best {
				best = s.score
			}
		}

		for i := 0; i < 15; i++ {
			p := selectDeployment(color, depth)
			if kings, knights, pawns, other := armyOf(p); kings != 1 || knights != 1 || pawns != 2 || other != 0 {
				t.Fatalf("selectDeployment(%s) = %s: not a legal army", color, p)
			}

			// the chosen placement's own score must sit within the margin of best
			var score float64
			var found bool
			for _, s := range scored {
				if s.placement == p {
					score, found = s.score, true
					break
				}
			}
			if !found {
				t.Fatalf("selectDeployment(%s) = %s: not among enumerated placements", color, p)
			}
			within := (color == octad.White && score >= best-DeployVarietyMargin) ||
				(color == octad.Black && score <= best+DeployVarietyMargin)
			if !within {
				t.Fatalf("selectDeployment(%s) = %s (%.2f) trails best %.2f beyond margin %.2f",
					color, p, score, best, DeployVarietyMargin)
			}
		}
	}
}
