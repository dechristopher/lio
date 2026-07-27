package view

import (
	"testing"
	"time"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/title"
)

// TestHeadlineRating locks the "most games, among settled" rule for the one
// rating a page title quotes.
func TestHeadlineRating(t *testing.T) {
	ratings := []RatingView{
		NewRatingView("quarter-zero-bullet-deploy", "1488?", 4),
		NewRatingView("half-one-blitz-deploy", "1520", 12),
		NewRatingView("one-two-rapid-deploy", "1641", 18),
		NewRatingView("three-five-rapid-deploy", "1712", 40),
	}
	if got := HeadlineRating(ratings); got != "1712 rapid" {
		t.Errorf("HeadlineRating = %q, want 1712 rapid", got)
	}

	// a provisional rating never wins, however many games or however high —
	// a title is read without context, so an unsettled number is a claim the
	// account has not earned
	flukey := []RatingView{
		NewRatingView("quarter-zero-bullet-deploy", "1800?", 900),
		NewRatingView("one-two-rapid-deploy", "1450", 3),
	}
	if got := HeadlineRating(flukey); got != "1450 rapid" {
		t.Errorf("HeadlineRating = %q, want the settled 1450 rapid", got)
	}

	// all provisional, or none at all, means no number
	if got := HeadlineRating([]RatingView{
		NewRatingView("one-two-rapid-deploy", "1500?", 2),
	}); got != "" {
		t.Errorf("HeadlineRating = %q, want empty for provisional-only", got)
	}
	if got := HeadlineRating(nil); got != "" {
		t.Errorf("HeadlineRating = %q, want empty for an unrated account", got)
	}
}

// TestProfileMetaTitle covers the bare-text title: bracketed account title, then
// the headline rating.
func TestProfileMetaTitle(t *testing.T) {
	m := profileFixture()
	m.Ratings = []RatingView{NewRatingView("three-five-rapid-deploy", "1712", 40)}
	meta := ProfileMeta(m)

	if want := "[OG] drewtest · 1712 rapid"; meta.OGTitle != want {
		t.Errorf("OGTitle = %q, want %q", meta.OGTitle, want)
	}
	// an unrated account carries no number rather than a placeholder
	m.Ratings = nil
	if got := ProfileMeta(m).OGTitle; got != "[OG] drewtest" {
		t.Errorf("unrated OGTitle = %q", got)
	}
	// an untitled account has no brackets to show
	m.Title = title.Title{}
	if got := ProfileMeta(m).OGTitle; got != "drewtest" {
		t.Errorf("untitled OGTitle = %q", got)
	}
}

// TestCompactPlayed locks the hero figure's duration format. Compact because
// it is a figure rather than prose, and days carry their hours: collapsing to
// whole days discards up to 23 of them, so a player crossing 24h would watch
// "23h" become "1d" and reasonably read that as the number going down.
func TestCompactPlayed(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "—"},
		{30 * time.Second, "—"},
		{45 * time.Minute, "45m"},
		{time.Hour, "1h"},
		{23 * time.Hour, "23h"},
		{24 * time.Hour, "1d"},
		{47 * time.Hour, "1d 23h"},
		{72 * time.Hour, "3d"},
	}
	for _, c := range cases {
		if got := compactPlayed(c.d); got != c.want {
			t.Errorf("compactPlayed(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

// TestLifetimeViewNoZeroFacts confirms an account with no games shows no
// figures at all rather than two zeroes — a worse greeting for a new player
// than silence.
func TestLifetimeViewNoZeroFacts(t *testing.T) {
	if v := NewLifetimeView(db.Record{}, db.Lifetime{}); v.Show {
		t.Errorf("empty account rendered figures: %+v", v)
	}
	// 31 hours is a day and seven, not "31h" — the figure carries days once it
	// has them
	v := NewLifetimeView(db.Record{Games: 1204}, db.Lifetime{Played: 31 * time.Hour})
	if !v.Show || v.Games != "1,204" || v.Played != "1d 7h" {
		t.Errorf("figures = %+v", v)
	}
}
