package view

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dechristopher/lio/db"
)

// day builds a point n days before the reference instant.
func day(base time.Time, back int, rating int, prov bool) db.RatingPoint {
	return db.RatingPoint{
		Day:         base.AddDate(0, 0, -back),
		Rating:      rating,
		Provisional: prov,
	}
}

var chartNow = time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

// TestRatingChartClosesWithCurrent locks the series construction rule: stored
// points are all "rating going into a game", so the live rating has to close the
// curve or the newest result is missing from it.
func TestRatingChartClosesWithCurrent(t *testing.T) {
	hist := []db.RatingSeries{{
		Category: "one-two-rapid-deploy",
		Points: []db.RatingPoint{
			day(chartNow, 3, 1500, false),
			day(chartNow, 2, 1520, false),
		},
	}}
	cur := []RatingView{NewRatingView("one-two-rapid-deploy", "1560", 3)}

	charts := NewRatingCharts(hist, cur, chartNow)
	if len(charts) != 1 {
		t.Fatalf("got %d charts, want 1", len(charts))
	}
	c := charts[0]
	if !c.Ready {
		t.Fatal("chart should be plottable with 3 points")
	}
	if len(c.Dots) != 3 {
		t.Fatalf("got %d dots, want 3 (2 stored + current)", len(c.Dots))
	}
	if got := c.Dots[len(c.Dots)-1].Rating; got != "1560" {
		t.Errorf("final point = %q, want the current rating 1560", got)
	}
	// overall change spans first stored point to current
	if c.Change != "+60" {
		t.Errorf("Change = %q, want +60", c.Change)
	}
	if !c.ChangeUp || c.Flat {
		t.Error("a rise should be marked up and not flat")
	}
}

// TestRatingChartNeedsTwoPoints covers the placeholder path: a category with a
// rating but no movement yet renders a placeholder naming what it needs, rather
// than a horizontal line implying a history.
func TestRatingChartNeedsTwoPoints(t *testing.T) {
	cur := []RatingView{NewRatingView("one-two-rapid-deploy", "1500?", 1)}
	charts := NewRatingCharts(nil, cur, chartNow)

	if len(charts) != 1 {
		t.Fatalf("got %d charts, want 1", len(charts))
	}
	c := charts[0]
	if c.Ready {
		t.Error("a single point is not a curve")
	}
	if !c.Placeholder.Meter() {
		t.Error("placeholder should show progress toward the threshold")
	}
	if got := c.Placeholder.Progress(); got != "1 of 2 rated days" {
		t.Errorf("Progress() = %q", got)
	}
	// it is still the active panel — a tile the player clicks must show
	// something, even when that something is a placeholder
	if !c.Active {
		t.Error("the only category should be active")
	}
}

// TestRatingChartProvisionalSplit locks the two-stroke split: the provisional
// stretch is its own polyline so it can render dashed, and the two overlap by a
// point so they join without a gap.
func TestRatingChartProvisionalSplit(t *testing.T) {
	hist := []db.RatingSeries{{
		Category: "one-two-rapid-deploy",
		Points: []db.RatingPoint{
			day(chartNow, 5, 1500, true),
			day(chartNow, 4, 1530, true),
			day(chartNow, 3, 1550, false),
			day(chartNow, 2, 1570, false),
		},
	}}
	cur := []RatingView{NewRatingView("one-two-rapid-deploy", "1580", 9)}
	c := NewRatingCharts(hist, cur, chartNow)[0]

	if c.Prov == "" || c.Line == "" {
		t.Fatalf("expected both strokes, got prov=%q line=%q", c.Prov, c.Line)
	}
	provPts := strings.Fields(c.Prov)
	linePts := strings.Fields(c.Line)
	if len(provPts) != 3 {
		t.Errorf("provisional stroke = %d points, want 3 (2 + the join)", len(provPts))
	}
	// the strokes must share their meeting point or the line shows a gap
	if provPts[len(provPts)-1] != linePts[0] {
		t.Errorf("strokes do not meet: %q vs %q", provPts[len(provPts)-1], linePts[0])
	}
}

// TestRatingChartPeakLabel covers the direct-label rule: the peak is labelled
// only when it is not the endpoint, since labelling both prints one number twice.
func TestRatingChartPeakLabel(t *testing.T) {
	// peak in the middle, then a decline — peak gets its own label
	declining := []db.RatingSeries{{
		Category: "one-two-rapid-deploy",
		Points: []db.RatingPoint{
			day(chartNow, 4, 1500, false),
			day(chartNow, 3, 1700, false),
			day(chartNow, 2, 1600, false),
		},
	}}
	c := NewRatingCharts(declining,
		[]RatingView{NewRatingView("one-two-rapid-deploy", "1550", 9)}, chartNow)[0]
	if !c.PeakShow {
		t.Error("a peak behind the endpoint should be labelled")
	}
	if c.Best != "1700" {
		t.Errorf("Best = %q, want 1700", c.Best)
	}

	// climbing to a new high — the endpoint label already says it
	climbing := []db.RatingSeries{{
		Category: "one-two-rapid-deploy",
		Points: []db.RatingPoint{
			day(chartNow, 4, 1500, false),
			day(chartNow, 3, 1520, false),
		},
	}}
	c2 := NewRatingCharts(climbing,
		[]RatingView{NewRatingView("one-two-rapid-deploy", "1600", 9)}, chartNow)[0]
	if c2.PeakShow {
		t.Error("peak at the endpoint should not be double-labelled")
	}
}

// TestRatingChartGeometryInBounds guards the coordinate mapping: every plotted
// point must land inside the padded plot area, whatever the rating range. A
// point outside it renders clipped or on top of the axis labels.
func TestRatingChartGeometryInBounds(t *testing.T) {
	cases := [][]int{
		{1500, 1500, 1500}, // flat — exercises the minimum-band widening
		{1000, 2400, 1200}, // a huge swing
		{1500, 1508, 1496}, // a tiny swing
	}
	for i, ratings := range cases {
		pts := make([]db.RatingPoint, 0, len(ratings))
		for j, r := range ratings {
			pts = append(pts, day(chartNow, len(ratings)-j, r, false))
		}
		hist := []db.RatingSeries{{Category: "one-two-rapid-deploy", Points: pts}}
		c := NewRatingCharts(hist,
			[]RatingView{NewRatingView("one-two-rapid-deploy", "1500", 9)}, chartNow)[0]
		if !c.Ready {
			t.Fatalf("case %d: not plottable", i)
		}
		for _, d := range c.Dots {
			x, _ := strconv.ParseFloat(d.X, 64)
			y, _ := strconv.ParseFloat(d.Y, 64)
			if x < padL-0.01 || x > chartW-padR+0.01 {
				t.Errorf("case %d: x=%v outside plot area", i, x)
			}
			if y < padT-0.01 || y > chartH-padB+0.01 {
				t.Errorf("case %d: y=%v outside plot area", i, y)
			}
		}
		// gridlines must be round numbers, and there must be some
		if len(c.Ticks) == 0 {
			t.Errorf("case %d: no gridlines", i)
		}
		for _, tick := range c.Ticks {
			n, err := strconv.Atoi(tick.Label)
			if err != nil || n%25 != 0 {
				t.Errorf("case %d: tick %q is not a round number", i, tick.Label)
			}
		}
	}
}

// TestRatingChartDedupesDay covers the collision between a stored point from
// earlier today and the current rating appended at today's date: they must
// collapse to one point, keeping the live value.
func TestRatingChartDedupesDay(t *testing.T) {
	hist := []db.RatingSeries{{
		Category: "one-two-rapid-deploy",
		Points: []db.RatingPoint{
			day(chartNow, 2, 1500, false),
			day(chartNow, 0, 1540, false), // a game earlier today
		},
	}}
	c := NewRatingCharts(hist,
		[]RatingView{NewRatingView("one-two-rapid-deploy", "1555", 9)}, chartNow)[0]

	if len(c.Dots) != 2 {
		t.Fatalf("got %d dots, want 2 (today collapsed)", len(c.Dots))
	}
	if got := c.Dots[1].Rating; got != "1555" {
		t.Errorf("today's point = %q, want the live rating 1555", got)
	}
}

// TestRatingChartActivePrefersPlottable locks which panel opens: the first
// category with a real curve, not whichever placeholder happens to sort first.
func TestRatingChartActivePrefersPlottable(t *testing.T) {
	hist := []db.RatingSeries{{
		Category: "one-two-rapid-deploy",
		Points: []db.RatingPoint{
			day(chartNow, 3, 1500, false),
			day(chartNow, 2, 1520, false),
		},
	}}
	// bullet sorts first but has no history
	cur := []RatingView{
		NewRatingView("quarter-zero-bullet-deploy", "1500?", 1),
		NewRatingView("one-two-rapid-deploy", "1540", 5),
	}
	charts := NewRatingCharts(hist, cur, chartNow)

	if charts[0].Active {
		t.Error("an unplottable category should not open the section")
	}
	if !charts[1].Active {
		t.Error("the first plottable category should be active")
	}
}
