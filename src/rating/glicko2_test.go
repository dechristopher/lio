package rating

import (
	"math"
	"testing"
)

// approx compares floats to a tolerance.
func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// TestGlickmanExample pins the worked example from Glickman's Glicko-2 paper:
// player 1500/200/0.06 with tau=0.5 vs three opponents (win, loss, loss) yields
// r'≈1464.06, RD'≈151.52, σ'≈0.05999.
func TestGlickmanExample(t *testing.T) {
	p := Rating{R: 1500, RD: 200, Sigma: 0.06}
	out := p.updatePeriod([]opponent{
		{r: Rating{R: 1400, RD: 30}, score: Win},
		{r: Rating{R: 1550, RD: 100}, score: Loss},
		{r: Rating{R: 1700, RD: 300}, score: Loss},
	}, 0.5)

	if !approx(out.R, 1464.06, 0.1) {
		t.Errorf("R' = %.4f, want ≈1464.06", out.R)
	}
	if !approx(out.RD, 151.52, 0.1) {
		t.Errorf("RD' = %.4f, want ≈151.52", out.RD)
	}
	if !approx(out.Sigma, 0.05999, 0.0001) {
		t.Errorf("σ' = %.6f, want ≈0.05999", out.Sigma)
	}
}

// TestUpdateDirectionAndRDShrink: beating a peer raises R and a game lowers RD
// (a game adds information).
func TestUpdateDirectionAndRDShrink(t *testing.T) {
	p := New()
	won := p.Update(New(), Win)
	if won.R <= p.R {
		t.Errorf("winning did not raise rating: %.2f -> %.2f", p.R, won.R)
	}
	if won.RD >= p.RD {
		t.Errorf("a game did not shrink RD: %.2f -> %.2f", p.RD, won.RD)
	}
	lost := p.Update(New(), Loss)
	if lost.R >= p.R {
		t.Errorf("losing did not lower rating: %.2f -> %.2f", p.R, lost.R)
	}
	if won.Games != 1 || lost.Games != 1 {
		t.Errorf("games not incremented: won=%d lost=%d", won.Games, lost.Games)
	}
}

// TestSymmetry: two identical players, one beats the other — the winner's gain
// mirrors the loser's loss, and their new RDs match.
func TestSymmetry(t *testing.T) {
	a, b := New(), New()
	newA := a.Update(b, Win)
	newB := b.Update(a, Loss)

	if !approx(newA.R-a.R, b.R-newB.R, 0.001) {
		t.Errorf("asymmetric R change: +%.4f vs -%.4f", newA.R-a.R, b.R-newB.R)
	}
	if !approx(newA.RD, newB.RD, 0.001) {
		t.Errorf("RD diverged: %.4f vs %.4f", newA.RD, newB.RD)
	}
	// centered around 1500
	if !approx(newA.R+newB.R, 3000, 0.001) {
		t.Errorf("not centered: sum=%.4f want 3000", newA.R+newB.R)
	}
}

// TestRDInflation: a player who sits out a period sees RD grow (toward, but
// capped at, the unrated default).
func TestRDInflation(t *testing.T) {
	p := Rating{R: 1600, RD: 60, Sigma: 0.06}
	out := p.updatePeriod(nil, tau)
	if out.RD <= p.RD {
		t.Errorf("RD did not inflate over an idle period: %.2f -> %.2f", p.RD, out.RD)
	}
	if out.R != p.R {
		t.Errorf("R changed over an idle period: %.2f -> %.2f", p.R, out.R)
	}
	// even a long idle streak never exceeds the unrated default
	for i := 0; i < 100; i++ {
		out = out.updatePeriod(nil, tau)
	}
	if out.RD > DefaultRD {
		t.Errorf("RD exceeded cap: %.2f > %.2f", out.RD, DefaultRD)
	}
}

// TestDisplayProvisional: a new rating shows "?", a settled one does not.
func TestDisplayProvisional(t *testing.T) {
	if got := New().Display(); got != "1500?" {
		t.Errorf("new rating Display = %q, want 1500?", got)
	}
	settled := Rating{R: 1653.4, RD: 80, Sigma: 0.06}
	if settled.Provisional() {
		t.Error("RD 80 flagged provisional")
	}
	if got := settled.Display(); got != "1653" {
		t.Errorf("settled Display = %q, want 1653", got)
	}
}
