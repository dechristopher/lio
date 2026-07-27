package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/dechristopher/lio/db"
)

// The rating curve (arch/PROFILE_STATS.md Phase 2). Geometry is computed here
// and rendered as inline SVG by the templ component — there is no charting
// library on the site, and adding one would mean a fourth pinned asset in the
// Docker build to draw a polyline.
//
// Mark specs follow the shared dataviz rules: a 2px round-capped line, a ≥8px
// end marker ringed in the surface colour, a ~10% area wash, and *solid*
// hairline gridlines. Colour is deliberately neutral ink rather than --accent:
// the accent is retinted by the player's board theme (so a data series would
// change hue with an unrelated preference) and is the same green as --win (so a
// line would read as a "win" mark). Only the peak marker takes a hue, --warn,
// which no board theme touches.

// Chart geometry in user units. The SVG scales to its container via viewBox, so
// these are aspect ratio and label spacing, not pixels on screen.
const (
	chartW = 640.0
	chartH = 180.0
	// padding leaves room for the y-axis tick labels (left) and the endpoint
	// label (right), which sit outside the plot area.
	padL = 38.0
	padR = 46.0
	padT = 14.0
	padB = 20.0
)

// minCurvePoints is how many samples a category needs before it draws a line
// rather than a placeholder. Two is the real floor: one point is a dot, and a
// "curve" through it would be a horizontal line implying a history that does not
// exist yet.
const minCurvePoints = 2

// RatingChartView is one time control's rating curve, fully resolved to SVG
// coordinates and display strings.
type RatingChartView struct {
	Category string // raw key, matches RatingView.Category
	Label    string // "1 + 2 rapid"
	Active   bool   // the panel shown first (server-set, so it works without JS)

	// Ready is false when the category has too few points to plot; the panel
	// then renders a placeholder naming how many more rated games it needs.
	Ready       bool
	Placeholder StatPlaceholder

	// Line is the settled stretch of the curve, Prov the provisional prefix
	// (rendered dashed — the rating was still uncertain there). They overlap by
	// one point so the two strokes join without a visible gap.
	Line string
	Prov string
	// Area is the closed path under the whole curve, filled at ~10%.
	Area string

	// Ticks are the horizontal gridlines: y position plus the rating they mark.
	Ticks []ChartTick
	// Dots are the hover targets, one per sample, carrying their own label.
	Dots []ChartDot

	// End is the final point — the current rating — direct-labelled.
	End ChartDot
	// Peak is the highest point, direct-labelled when it is not the endpoint
	// (labelling both when they coincide prints the same number twice).
	Peak     ChartDot
	PeakShow bool

	// Summary figures beside the chart.
	Current  string // "1653"
	Best     string // "1712"
	Change   string // "+24" / "−13", empty when there is nothing to compare
	ChangeUp bool
	Flat     bool // Change is zero — tinted neutral rather than win/loss
}

// ChartTick is one horizontal gridline and its axis label.
type ChartTick struct {
	Y     string
	Label string
}

// ChartDot is a plotted sample: its position, its value, and the label a
// tooltip or direct label shows.
type ChartDot struct {
	X, Y   string
	Rating string
	When   string // "Mar 4, 2026"
	// Provisional marks a sample whose rating was still settling.
	Provisional bool
}

// NewRatingCharts builds the per-category curves. history supplies the recorded
// points and current the account's live ratings, which close each series: the
// stored points are all "rating going into a game", so without the current
// rating the curve would stop short of the most recent result.
//
// Categories with a current rating but no history still produce a panel, so the
// tile a player clicks always has something behind it.
func NewRatingCharts(history []db.RatingSeries, current []RatingView, now time.Time) []RatingChartView {
	points := make(map[string][]db.RatingPoint, len(history))
	for _, s := range history {
		points[s.Category] = s.Points
	}

	out := make([]RatingChartView, 0, len(current))
	for _, r := range current {
		out = append(out, newRatingChart(r, points[r.Category], now))
	}

	// the first plottable category leads, so the page opens on a real curve
	// rather than on a placeholder that happens to sort first
	for i := range out {
		if out[i].Ready {
			out[i].Active = true
			return out
		}
	}
	if len(out) > 0 {
		out[0].Active = true
	}
	return out
}

// newRatingChart resolves one category's curve.
func newRatingChart(r RatingView, pts []db.RatingPoint, now time.Time) RatingChartView {
	c := RatingChartView{Category: r.Category, Label: ratingChartLabel(r), Current: r.Rating}

	// close the series with the live rating: every stored point is a rating
	// carried *into* a game, so the newest result is not in them yet
	if cur, prov, ok := parseDisplayRating(r.Rating); ok {
		pts = append(append([]db.RatingPoint(nil), pts...),
			db.RatingPoint{Day: now, Rating: cur, Provisional: prov})
	}
	pts = dedupeByDay(pts)

	if len(pts) < minCurvePoints {
		c.Placeholder = StatPlaceholder{
			Copy: "Your rating in this time control appears here once it has moved. " +
				"Play another rated game to start the curve.",
			Have: int64(len(pts)), Need: minCurvePoints, Unit: "rated days",
		}
		return c
	}
	c.Ready = true

	lo, hi := ratingBounds(pts)
	x := func(i int) float64 {
		if len(pts) == 1 {
			return padL
		}
		return padL + (chartW-padL-padR)*float64(i)/float64(len(pts)-1)
	}
	y := func(v int) float64 {
		return padT + (chartH-padT-padB)*(1-float64(v-lo)/float64(hi-lo))
	}

	// x is index-based, not date-based: a rating curve is read as a sequence of
	// results, and date spacing would compress an active month into a sliver
	// beside a long idle gap. The tooltip carries the real date for each point.
	var line, prov []string
	peakAt := 0
	for i, p := range pts {
		px, py := x(i), y(p.Rating)
		pair := coord(px) + "," + coord(py)
		if p.Provisional {
			prov = append(prov, pair)
		} else {
			if len(prov) > 0 && len(line) == 0 {
				// overlap by the last provisional point so the strokes meet
				prov = append(prov, pair)
			}
			line = append(line, pair)
		}
		if p.Rating > pts[peakAt].Rating {
			peakAt = i
		}
		c.Dots = append(c.Dots, ChartDot{
			X: coord(px), Y: coord(py),
			Rating:      strconv.Itoa(p.Rating),
			When:        p.Day.Format("Jan 2, 2006"),
			Provisional: p.Provisional,
		})
	}
	c.Line, c.Prov = strings.Join(line, " "), strings.Join(prov, " ")

	// area wash under the whole curve, closed along the baseline
	all := make([]string, 0, len(pts))
	for i, p := range pts {
		all = append(all, coord(x(i))+","+coord(y(p.Rating)))
	}
	base := coord(chartH - padB)
	c.Area = "M" + coord(x(0)) + "," + base + " L" + strings.Join(all, " L") +
		" L" + coord(x(len(pts)-1)) + "," + base + " Z"

	for _, t := range niceTicks(lo, hi) {
		c.Ticks = append(c.Ticks, ChartTick{
			Y: coord(y(t)), Label: strconv.Itoa(t),
		})
	}

	c.End = c.Dots[len(c.Dots)-1]
	c.Peak = c.Dots[peakAt]
	c.PeakShow = peakAt != len(c.Dots)-1 && pts[peakAt].Rating > pts[len(pts)-1].Rating
	c.Best = strconv.Itoa(pts[peakAt].Rating)

	if d := pts[len(pts)-1].Rating - pts[0].Rating; len(pts) > 1 {
		c.Flat = d == 0
		c.ChangeUp = d > 0
		switch {
		case d > 0:
			c.Change = "+" + strconv.Itoa(d)
		case d < 0:
			// U+2212 minus, not a hyphen: it aligns with the digits in the
			// tabular-figures the summary uses
			c.Change = "−" + strconv.Itoa(-d)
		default:
			c.Change = "±0"
		}
	}
	return c
}

// Template helpers for the chart's fixed geometry. They exist so the SVG's
// frame numbers come from the same constants the geometry does, instead of
// being retyped as literals in the markup.

func chartViewBox() string {
	return "0 0 " + coord(chartW) + " " + coord(chartH)
}
func chartPlotLeft() string   { return coord(padL) }
func chartPlotRight() string  { return coord(chartW - padR) }
func chartPlotTop() string    { return coord(padT) }
func chartPlotBottom() string { return coord(chartH - padB) }

// chartAxisX is where y-axis tick labels sit: just left of the plot, right
// aligned into the left padding.
func chartAxisX() string { return coord(padL - 6) }

// chartEndLabelX places the endpoint's direct label clear of its marker.
func chartEndLabelX(c RatingChartView) string {
	x, err := strconv.ParseFloat(c.End.X, 64)
	if err != nil {
		return coord(chartW - padR + 8)
	}
	return coord(x + 8)
}

// chartChangeClass tints the overall change by direction. This one figure is
// genuinely semantic — a rating going up is good — so it takes the win/loss
// tokens the rest of the site uses for results, while the curve itself stays
// neutral ink.
func chartChangeClass(c RatingChartView) string {
	switch {
	case c.Flat:
		return "is-flat"
	case c.ChangeUp:
		return "is-up"
	default:
		return "is-down"
	}
}

// boolAttr renders a Go bool as an HTML attribute value.
func boolAttr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ratingChartLabel names a curve for its figure caption.
func ratingChartLabel(r RatingView) string {
	if r.Speed == "" {
		return r.Label
	}
	return r.Label + " " + r.Speed
}

// dedupeByDay collapses samples that share a calendar day, keeping the last —
// the appended current rating lands on today and would otherwise sit beside a
// stored point from a game played earlier the same day.
func dedupeByDay(pts []db.RatingPoint) []db.RatingPoint {
	out := pts[:0:0]
	for _, p := range pts {
		if n := len(out); n > 0 && sameDay(out[n-1].Day, p.Day) {
			out[n-1] = p
			continue
		}
		out = append(out, p)
	}
	return out
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

// ratingBounds pads the observed range out to a readable window. A curve that
// only ever moved eight points should not render as a dramatic mountain range,
// so the band is never narrower than minBand.
func ratingBounds(pts []db.RatingPoint) (lo, hi int) {
	lo, hi = pts[0].Rating, pts[0].Rating
	for _, p := range pts[1:] {
		if p.Rating < lo {
			lo = p.Rating
		}
		if p.Rating > hi {
			hi = p.Rating
		}
	}
	const minBand = 80
	if hi-lo < minBand {
		mid := (hi + lo) / 2
		lo, hi = mid-minBand/2, mid+minBand/2
	}
	// breathing room so the line never runs along the frame
	pad := (hi - lo) / 10
	return lo - pad, hi + pad
}

// niceTicks picks two or three round gridline values inside the band. Round
// numbers only — a gridline labelled 1637 is noise where 1600 is a landmark.
func niceTicks(lo, hi int) []int {
	step := 100
	for _, s := range []int{25, 50, 100, 200, 500} {
		if (hi-lo)/s <= 4 {
			step = s
			break
		}
	}
	var out []int
	for v := ((lo + step - 1) / step) * step; v <= hi; v += step {
		out = append(out, v)
	}
	return out
}

// coord formats an SVG coordinate compactly — two decimals is well below a
// pixel at any rendered size, and trims the generated markup.
func coord(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// parseDisplayRating splits a rendered rating ("1653" / "1500?") back into its
// value and provisional flag. The view holds ratings as display strings, so the
// chart reads them the same way the archive recorded them.
func parseDisplayRating(s string) (rating int, provisional bool, ok bool) {
	provisional = strings.HasSuffix(s, "?")
	n, err := strconv.Atoi(strings.TrimSuffix(s, "?"))
	if err != nil {
		return 0, false, false
	}
	return n, provisional, true
}
