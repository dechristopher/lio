// Package rating is a hand-written Glicko-2 implementation
// (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5). Lioctad rates per time-control
// category (variant.Group); each rated game is applied as a one-game rating
// period, transactionally, inside the archive commit. Reference: Glickman,
// "Example of the Glicko-2 system" (http://www.glicko.net/glicko/glicko2.pdf).
package rating

import (
	"math"
	"strconv"
)

const (
	// DefaultRating / DefaultRD / DefaultVol are an unrated player's starting
	// values (Glickman's defaults).
	DefaultRating = 1500.0
	DefaultRD     = 350.0
	DefaultVol    = 0.06

	// tau constrains volatility change over time. Smaller = steadier ratings;
	// 0.75 is a middle-of-the-road value for a fast, high-variance variant.
	tau = 0.75

	// scale converts between the display (Glicko-1) scale and the internal
	// Glicko-2 scale.
	scale = 173.7178

	// convergence tolerance for the volatility solver.
	epsilon = 1e-6

	// provisionalRD is the RD above which a rating is shown with a "?".
	provisionalRD = 110.0
)

// Outcome scores from the subject player's perspective.
const (
	Win  = 1.0
	Draw = 0.5
	Loss = 0.0
)

// Rating is a player's Glicko-2 state in one category: rating R, deviation RD,
// volatility Sigma, and the number of rated games contributing to it.
type Rating struct {
	R     float64
	RD    float64
	Sigma float64
	Games int
}

// New returns a fresh unrated player's rating.
func New() Rating {
	return Rating{R: DefaultRating, RD: DefaultRD, Sigma: DefaultVol}
}

// Provisional reports whether the rating is still uncertain (RD > 110) — shown
// with a trailing "?".
func (r Rating) Provisional() bool {
	return r.RD > provisionalRD
}

// Display renders the rating for the UI: the rounded rating, plus "?" while
// provisional ("1500?" / "1650").
func (r Rating) Display() string {
	s := strconv.Itoa(int(math.Round(r.R)))
	if r.Provisional() {
		s += "?"
	}
	return s
}

// Update returns the player's new rating after a single game against opp with
// the given score (Win/Draw/Loss). It is a one-game rating period — the way
// lioctad applies ratings (once per finished rated game).
func (r Rating) Update(opp Rating, score float64) Rating {
	return r.updatePeriod([]opponent{{r: opp, score: score}}, tau)
}

// opponent pairs an opponent's rating with the subject's score against them.
type opponent struct {
	r     Rating
	score float64
}

// updatePeriod applies one rating period (any number of opponents) with an
// explicit tau, following the Glicko-2 steps. Exposed to tests (with tau=0.5
// and three opponents) to pin Glickman's worked example.
func (r Rating) updatePeriod(opps []opponent, tauV float64) Rating {
	// Step 2: convert to the Glicko-2 scale.
	mu := (r.R - DefaultRating) / scale
	phi := r.RD / scale
	sigma := r.Sigma
	if sigma <= 0 {
		sigma = DefaultVol
	}

	// A player who did not compete only sees their RD inflate (Step 6 with no
	// estimated variance).
	if len(opps) == 0 {
		phiStar := math.Sqrt(phi*phi + sigma*sigma)
		return Rating{R: r.R, RD: capRD(phiStar * scale), Sigma: sigma, Games: r.Games}
	}

	// Step 3/4: estimated variance v and the rating-scale improvement sumDelta.
	var invV, sumDelta float64
	for _, o := range opps {
		muJ := (o.r.R - DefaultRating) / scale
		phiJ := o.r.RD / scale
		gj := g(phiJ)
		e := expectedScore(mu, muJ, phiJ)
		invV += gj * gj * e * (1 - e)
		sumDelta += gj * (o.score - e)
	}
	v := 1 / invV
	delta := v * sumDelta

	// Step 5: new volatility.
	sigmaPrime := newVolatility(sigma, delta, phi, v, tauV)

	// Step 6/7: pre-period RD, then updated RD and rating.
	phiStar := math.Sqrt(phi*phi + sigmaPrime*sigmaPrime)
	phiPrime := 1 / math.Sqrt(1/(phiStar*phiStar)+1/v)
	muPrime := mu + phiPrime*phiPrime*sumDelta

	// Step 8: back to the display scale.
	return Rating{
		R:     muPrime*scale + DefaultRating,
		RD:    capRD(phiPrime * scale),
		Sigma: sigmaPrime,
		Games: r.Games + len(opps),
	}
}

// g is the Glicko-2 weighting of an opponent by their deviation.
func g(phi float64) float64 {
	return 1 / math.Sqrt(1+3*phi*phi/(math.Pi*math.Pi))
}

// expectedScore is the expected result against an opponent.
func expectedScore(mu, muJ, phiJ float64) float64 {
	return 1 / (1 + math.Exp(-g(phiJ)*(mu-muJ)))
}

// newVolatility solves for σ' with the Illinois algorithm (Step 5).
func newVolatility(sigma, delta, phi, v, tauV float64) float64 {
	a := math.Log(sigma * sigma)
	delta2 := delta * delta
	phi2 := phi * phi
	tau2 := tauV * tauV

	f := func(x float64) float64 {
		ex := math.Exp(x)
		denom := phi2 + v + ex
		return ex*(delta2-phi2-v-ex)/(2*denom*denom) - (x-a)/tau2
	}

	A := a
	var B float64
	if delta2 > phi2+v {
		B = math.Log(delta2 - phi2 - v)
	} else {
		k := 1.0
		for f(a-k*tauV) < 0 {
			k++
		}
		B = a - k*tauV
	}

	fA, fB := f(A), f(B)
	for math.Abs(B-A) > epsilon {
		C := A + (A-B)*fA/(fB-fA)
		fC := f(C)
		if fC*fB <= 0 {
			A, fA = B, fB
		} else {
			fA /= 2
		}
		B, fB = C, fC
	}
	return math.Exp(A / 2)
}

// capRD bounds RD at the unrated default — inactivity should never make a rating
// less certain than a brand-new one.
func capRD(rd float64) float64 {
	if rd > DefaultRD {
		return DefaultRD
	}
	return rd
}
