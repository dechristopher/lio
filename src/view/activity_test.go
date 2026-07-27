package view

import (
	"testing"
	"time"

	"github.com/dechristopher/lio/db"
)

var actNow = time.Date(2026, 7, 26, 15, 0, 0, 0, time.UTC)

func act(daysAgo int, games int64) db.ActivityDay {
	return db.ActivityDay{
		Day:    actNow.AddDate(0, 0, -daysAgo).Truncate(24 * time.Hour),
		Record: db.Record{Games: games, Wins: games},
	}
}

// TestActivityGridShape locks the calendar geometry: whole weeks, seven rows,
// and no cell before the window or after today.
func TestActivityGridShape(t *testing.T) {
	v := NewActivityView([]db.ActivityDay{act(0, 3), act(40, 1)}, actNow)
	if !v.Show {
		t.Fatal("view should render with games present")
	}
	if len(v.Weeks) != activityWeeks {
		t.Fatalf("got %d weeks, want %d", len(v.Weeks), activityWeeks)
	}
	// the grid starts on a Sunday, so weekday rows line up down every column
	var future, past int
	for _, w := range v.Weeks {
		for _, d := range w.Days {
			if d.Future {
				future++
			} else {
				past++
			}
		}
	}
	if past+future != activityWeeks*7 {
		t.Errorf("cells = %d, want %d", past+future, activityWeeks*7)
	}
	// today is a Sunday-start week's second day, so some of the final column is
	// still ahead — but never a whole week of it
	if future >= 7 {
		t.Errorf("%d future cells, want fewer than a full week", future)
	}
	if len(v.Months) == 0 {
		t.Error("expected month labels")
	}
}

// TestActivityLevelsRelative locks that density is scaled to the account's own
// busiest day. An absolute scale would render a casual player's entire year at
// the faintest step.
func TestActivityLevelsRelative(t *testing.T) {
	// a light player: two games is their maximum, and it should reach the top
	light := NewActivityView([]db.ActivityDay{act(1, 1), act(2, 2)}, actNow)
	if got := levelOn(light, actNow.AddDate(0, 0, -2)); got != activityLevels {
		t.Errorf("busiest day of a light player = level %d, want %d", got, activityLevels)
	}
	// a heavy player: the same two games is now near the bottom
	heavy := NewActivityView([]db.ActivityDay{act(1, 40), act(2, 2)}, actNow)
	if got := levelOn(heavy, actNow.AddDate(0, 0, -2)); got != 1 {
		t.Errorf("2 games against a 40-game peak = level %d, want 1", got)
	}
	// and a day with no games is always level 0
	if got := levelOn(heavy, actNow.AddDate(0, 0, -3)); got != 0 {
		t.Errorf("empty day = level %d, want 0", got)
	}
}

// TestActivityEmpty confirms an account with no games in the window claims
// nothing at all rather than rendering a blank year.
func TestActivityEmpty(t *testing.T) {
	if v := NewActivityView(nil, actNow); v.Show || len(v.Weeks) != 0 {
		t.Errorf("empty activity rendered %d weeks (show=%v)", len(v.Weeks), v.Show)
	}
}

// TestBotLadderShowsEveryRung locks that the ladder is a progression, not a
// list of what happened to be played: a bot never faced still occupies its rung.
func TestBotLadderShowsEveryRung(t *testing.T) {
	ladder := NewBotLadder([]BotRecordView{
		{Persona: "Pawn", Glyph: "P", Bar: NewWDLBar(db.Record{Games: 4, Wins: 3, Losses: 1})},
	})
	if len(ladder) < 5 {
		t.Fatalf("ladder has %d rungs, want every persona", len(ladder))
	}
	if ladder[0].Name != "Pawn" {
		t.Errorf("ladder starts at %q, want the weakest persona first", ladder[0].Name)
	}
	if !ladder[0].Played || !ladder[0].Beaten || ladder[0].Score != "0.75" {
		t.Errorf("played rung = played:%v beaten:%v score:%q",
			ladder[0].Played, ladder[0].Beaten, ladder[0].Score)
	}
	// every other rung is unplayed but still present
	for _, r := range ladder[1:] {
		if r.Played || r.Beaten {
			t.Errorf("rung %q reported as played", r.Name)
		}
		if r.Score != "" {
			t.Errorf("unplayed rung %q claims %q", r.Name, r.Score)
		}
	}
}

// TestBotLadderBeatenNeedsAWin separates "played it" from "beat it" — the
// ladder's actual question.
func TestBotLadderBeatenNeedsAWin(t *testing.T) {
	ladder := NewBotLadder([]BotRecordView{
		{Persona: "Queen", Bar: NewWDLBar(db.Record{Games: 6, Draws: 1, Losses: 5})},
	})
	var queen BotRungView
	for _, r := range ladder {
		if r.Name == "Queen" {
			queen = r
		}
	}
	if !queen.Played {
		t.Error("six games is played")
	}
	if queen.Beaten {
		t.Error("no wins is not beaten")
	}
	// half a point from six games, as a rate rather than a percentage
	if queen.Score != "0.08" {
		t.Errorf("score = %q, want 0.08 (one draw of six)", queen.Score)
	}
	if queen.Class != "loss" {
		t.Errorf("class = %q, want loss for a rate below even", queen.Class)
	}
}

// levelOn finds the rendered level for a given date.
func levelOn(v ActivityView, date time.Time) int {
	want := activityLabelDate(date)
	for _, w := range v.Weeks {
		for _, d := range w.Days {
			if !d.Future && dateOfLabel(d.Label) == want {
				return d.Level
			}
		}
	}
	return -1
}

func activityLabelDate(t time.Time) string { return t.UTC().Format("2 Jan 2006") }

func dateOfLabel(label string) string {
	// labels end with "on <date>"; everything after the last " on " is the date
	for i := len(label) - 1; i >= 3; i-- {
		if label[i-3:i+1] == " on " {
			return label[i+1:]
		}
	}
	return ""
}

// TestScoreRateAndClass locks the figure the social sections report: points per
// game as a decimal, tinted by which side of even it lands. A percentage would
// invite reading it as a win *rate*, which counts draws as losses.
func TestScoreRateAndClass(t *testing.T) {
	cases := []struct {
		r          db.Record
		rate, tint string
	}{
		{db.Record{Games: 4, Wins: 3, Losses: 1}, "0.75", "win"},
		{db.Record{Games: 4, Wins: 1, Losses: 3}, "0.25", "loss"},
		{db.Record{Games: 4, Wins: 2, Losses: 2}, "0.50", "draw"},
		// all draws is exactly even, and must not be tinted as a win or a loss
		{db.Record{Games: 4, Draws: 4}, "0.50", "draw"},
		{db.Record{Games: 3, Wins: 2, Draws: 1}, "0.83", "win"},
		{db.Record{Games: 0}, "", ""},
	}
	for _, c := range cases {
		if got := ScoreRate(c.r); got != c.rate {
			t.Errorf("ScoreRate(%+v) = %q, want %q", c.r, got, c.rate)
		}
		if got := RateClass(c.r); got != c.tint {
			t.Errorf("RateClass(%+v) = %q, want %q", c.r, got, c.tint)
		}
	}
}

// TestRateLevelDiverging checks the win-rate ramp: a neutral band around even so
// a 1-1 day is not painted as good or bad, and a no-games day sits at the middle
// where the volume ramp renders it empty anyway.
func TestRateLevelDiverging(t *testing.T) {
	cases := []struct {
		r    db.Record
		want int
	}{
		{db.Record{Games: 4, Wins: 4}, rateLevels},
		{db.Record{Games: 4, Losses: 4}, 0},
		{db.Record{Games: 2, Wins: 1, Losses: 1}, rateMid},
		{db.Record{Games: 4, Draws: 4}, rateMid},
		{db.Record{}, rateMid},
	}
	for _, c := range cases {
		if got := rateLevel(c.r); got != c.want {
			t.Errorf("rateLevel(%+v) = %d, want %d", c.r, got, c.want)
		}
	}
}

// TestFormGroupFirstURL locks where clicking a match container leads: the strip
// runs oldest to newest, so the match's first game is its first pip.
func TestFormGroupFirstURL(t *testing.T) {
	games := []ProfileGameView{
		{RoomID: "rA", Result: "Won", Class: "win", Opponent: "a", URL: "/rA/3"},
		{RoomID: "rA", Result: "Lost", Class: "loss", Opponent: "a", URL: "/rA/2"},
		{RoomID: "rA", Result: "Won", Class: "win", Opponent: "a", URL: "/rA/1"},
	}
	got := NewFormGroups(games, "")
	if len(got) != 1 {
		t.Fatalf("got %d groups, want 1", len(got))
	}
	if got[0].FirstURL != "/rA/1" {
		t.Errorf("FirstURL = %q, want the match's opening game /rA/1", got[0].FirstURL)
	}

	// A match the window cut into still points at game 1, not at the oldest
	// game that happens to be on screen — "go to the first game" landing on
	// game 3 would be a quiet lie.
	cut := NewFormGroups([]ProfileGameView{
		{RoomID: "rB", Result: "Won", Class: "win", Opponent: "b", URL: "/rB/4"},
		{RoomID: "rB", Result: "Won", Class: "win", Opponent: "b", URL: "/rB/3"},
	}, "rB")
	if !cut[0].Partial {
		t.Fatal("expected the group to be partial")
	}
	if cut[0].FirstURL != "/rB/1" {
		t.Errorf("partial FirstURL = %q, want /rB/1", cut[0].FirstURL)
	}
}
