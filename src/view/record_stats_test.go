package view

import (
	"strings"
	"testing"

	"github.com/dechristopher/lio/db"
)

func rec(games, wins, draws, losses int64) db.Record {
	return db.Record{Games: games, Wins: wins, Draws: draws, Losses: losses}
}

// TestWDLBarRounding covers the win-percentage rounding. Half-up, so a record
// that is genuinely half wins does not read as 49%.
func TestWDLBarRounding(t *testing.T) {
	cases := []struct {
		r    db.Record
		want string
	}{
		{rec(2, 1, 0, 1), "50%"},
		{rec(3, 1, 0, 2), "33%"},
		{rec(3, 2, 0, 1), "67%"},
		{rec(8, 3, 0, 5), "38%"},
		{rec(0, 0, 0, 0), ""}, // no games makes no claim
	}
	for _, c := range cases {
		if got := NewWDLBar(c.r).WinPct; got != c.want {
			t.Errorf("WinPct(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

// TestColorSplitsAlwaysBothSeats locks that both seats render once the account
// has played at all — "never played Black" is itself worth seeing, and a missing
// row reads as a rendering bug rather than a fact.
func TestColorSplitsAlwaysBothSeats(t *testing.T) {
	got := NewColorSplits([]db.ColorRecord{
		{AsWhite: true, Record: rec(5, 3, 1, 1)},
	})
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].Label != "As White" || got[1].Label != "As Black" {
		t.Errorf("labels = %q/%q, want As White/As Black", got[0].Label, got[1].Label)
	}
	if got[1].Games != "0" {
		t.Errorf("unplayed seat games = %q, want 0", got[1].Games)
	}
	// no games at all means no section, not two empty rows
	if n := len(NewColorSplits(nil)); n != 0 {
		t.Errorf("got %d rows for an account with no games, want 0", n)
	}
}

// TestEndingsRankByFrequency locks the track scaling: the busiest ending fills
// its track and the rest are proportional to it, so the rows read as a ranking.
func TestEndingsRankByFrequency(t *testing.T) {
	got := NewEndings([]db.EndingRecord{
		{Reason: "checkmate", Record: rec(20, 12, 0, 8)},
		{Reason: "time", Record: rec(10, 2, 0, 8)},
		{Reason: "", Record: rec(5, 1, 1, 3)},
	})
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	if got[0].Width != "100%" {
		t.Errorf("busiest width = %q, want 100%%", got[0].Width)
	}
	if got[1].Width != "50%" {
		t.Errorf("half-as-common width = %q, want 50%%", got[1].Width)
	}
	if got[0].Label != "Checkmate" || got[1].Label != "On time" {
		t.Errorf("labels = %q/%q", got[0].Label, got[1].Label)
	}
	// an untagged archive row is named honestly rather than dropped
	if got[2].Label != "Unrecorded" {
		t.Errorf("empty reason label = %q, want Unrecorded", got[2].Label)
	}
}

// TestLengthsBucketing covers the histogram: plies land in the right bucket, the
// open-ended tail catches everything long, and heights scale to the fullest.
func TestLengthsBucketing(t *testing.T) {
	v := NewLengths([]db.LengthRecord{
		{Plies: 4, Record: rec(2, 2, 0, 0)},
		{Plies: 10, Record: rec(1, 0, 0, 1)},
		{Plies: 11, Record: rec(6, 3, 1, 2)},
		{Plies: 55, Record: rec(1, 1, 0, 0)},
	})
	if len(v.Buckets) != len(lengthBuckets) {
		t.Fatalf("got %d buckets, want %d", len(v.Buckets), len(lengthBuckets))
	}
	// boundaries are inclusive: 10 belongs to 1-10, 11 starts the next bucket
	if v.Buckets[0].Games != 3 {
		t.Errorf("bucket 1-10 = %d games, want 3", v.Buckets[0].Games)
	}
	if v.Buckets[1].Games != 6 {
		t.Errorf("bucket 11-20 = %d games, want 6", v.Buckets[1].Games)
	}
	// the tail is open-ended, so a 55-ply game lands in 41+
	if v.Buckets[4].Games != 1 {
		t.Errorf("bucket 41+ = %d games, want 1", v.Buckets[4].Games)
	}
	if v.Buckets[1].Height != "100%" {
		t.Errorf("fullest bucket height = %q, want 100%%", v.Buckets[1].Height)
	}
	if v.Games != 10 {
		t.Errorf("total = %d, want 10", v.Games)
	}
	// 10 games; the 6th shortest is an 11-ply game
	if v.Median != "11 plies" {
		t.Errorf("Median = %q, want 11 plies", v.Median)
	}
	// an empty histogram claims nothing
	if n := len(NewLengths(nil).Buckets); n != 0 {
		t.Errorf("got %d buckets for no games, want 0", n)
	}
}

// TestFormGroupsRunOldestFirst locks the direction and the match grouping. The
// games list is newest-first, but a form strip is a timeline and has to read
// left to right.
func TestFormGroupsRunOldestFirst(t *testing.T) {
	// newest first, as the games list arrives: two rooms
	games := []ProfileGameView{
		{RoomID: "rB", Result: "Won", Class: "win", Opponent: "b", When: "today", URL: "/rB/2", Variant: "½ + 1 blitz", Reason: "time"},
		{RoomID: "rB", Result: "Lost", Class: "loss", Opponent: "b", When: "today", URL: "/rB/1", Variant: "½ + 1 blitz"},
		{RoomID: "rA", Result: "Drew", Class: "draw", Opponent: "a", When: "2 days ago", URL: "/rA/2", Variant: "1 + 2 rapid", Reason: "agreement"},
		{RoomID: "rA", Result: "Won", Class: "win", Opponent: "a", When: "2 days ago", URL: "/rA/1", Variant: "1 + 2 rapid", Reason: "checkmate"},
	}
	got := NewFormGroups(games, "")
	if len(got) != 2 {
		t.Fatalf("got %d groups, want 2 matches", len(got))
	}
	// oldest match leads
	if got[0].Pips[0].URL != "/rA/1" {
		t.Errorf("first pip = %s, want the oldest game /rA/1", got[0].Pips[0].URL)
	}
	// 1 win + 1 draw out of 2 games
	if got[0].Score != "1½–½" || got[0].Class != "win" {
		t.Errorf("match A = %q/%q, want 1½–½/win", got[0].Score, got[0].Class)
	}
	if got[1].Score != "1–1" || got[1].Class != "draw" {
		t.Errorf("match B = %q/%q, want 1–1/draw", got[1].Score, got[1].Class)
	}
	// only the newest game overall is marked latest
	last := got[1].Pips[len(got[1].Pips)-1]
	if !last.Latest || last.URL != "/rB/2" {
		t.Errorf("latest pip = %s (latest=%v), want /rB/2", last.URL, last.Latest)
	}
	var marked int
	for _, g := range got {
		for _, p := range g.Pips {
			if p.Latest {
				marked++
			}
		}
	}
	if marked != 1 {
		t.Errorf("%d pips marked latest, want exactly 1", marked)
	}
	// a match's readout leads with the match result and its score, tinted by the
	// match's own class
	if got[0].Result != "Won 1½–½" || got[0].Detail != "vs a · 1 + 2 rapid · 2 days ago" {
		t.Errorf("match readout = %q / %q", got[0].Result, got[0].Detail)
	}
	// each pip carries its own game-level readout, so hovering one game inside a
	// match reports that game rather than the match around it — including how
	// that individual game ended
	if got[0].Pips[0].Result != "Won by checkmate" {
		t.Errorf("pip result = %q", got[0].Pips[0].Result)
	}
	if got[0].Pips[1].Result != "Drew by agreement" {
		t.Errorf("second pip result = %q", got[0].Pips[1].Result)
	}
	// a game with no recorded method still names its result, just unqualified
	if got[1].Pips[0].Result != "Lost" {
		t.Errorf("reasonless pip result = %q", got[1].Pips[0].Result)
	}
}

// TestFormGroupsPartialMatch covers a match the recent-games window cut into.
// The score shown is of the games in view, so the group has to say so — the
// tooltip is where that lives, since the number itself cannot.
func TestFormGroupsPartialMatch(t *testing.T) {
	games := []ProfileGameView{
		{RoomID: "rA", Result: "Won", Class: "win", Opponent: "a", URL: "/rA/3", Variant: "1 + 2 rapid"},
		{RoomID: "rA", Result: "Won", Class: "win", Opponent: "a", URL: "/rA/2", Variant: "1 + 2 rapid"},
	}
	// the next game beyond the window belongs to the same match
	got := NewFormGroups(games, "rA")
	if len(got) != 1 {
		t.Fatalf("got %d groups, want 1", len(got))
	}
	if !got[0].Partial {
		t.Error("group should be marked partial")
	}
	if !strings.Contains(got[0].Detail, "partial match") {
		t.Errorf("Detail = %q, want it to disclose the truncation", got[0].Detail)
	}

	// a different older room means the match is whole
	if whole := NewFormGroups(games, "rZ"); whole[0].Partial {
		t.Error("a match whose predecessor is another room is not partial")
	}
	// no older games at all means nothing is partial
	if none := NewFormGroups(games, ""); none[0].Partial {
		t.Error("with no older games nothing can be partial")
	}
}

// TestFormGroupsRoomlessStandAlone checks that backfilled games with no room do
// not merge into one giant pseudo-match.
func TestFormGroupsRoomlessStandAlone(t *testing.T) {
	games := []ProfileGameView{
		{RoomID: "", Class: "win", Opponent: "a", URL: "/game/2"},
		{RoomID: "", Class: "loss", Opponent: "b", URL: "/game/1"},
	}
	if got := NewFormGroups(games, ""); len(got) != 2 {
		t.Errorf("got %d groups, want 2 standalone games", len(got))
	}
}

// TestStreakViewIgnoresRunsOfOne locks the "a streak of one is not a streak"
// rule — otherwise everyone who just won a game is told they are on a run.
func TestStreakViewIgnoresRunsOfOne(t *testing.T) {
	if v := NewStreakView(db.Streaks{CurrentLen: 1, CurrentScore: 1, BestWins: 1}); v.Show {
		t.Error("a single game should not render as a streak")
	}
	v := NewStreakView(db.Streaks{CurrentLen: 3, CurrentScore: 0, BestWins: 7})
	if !v.Show {
		t.Fatal("a real streak should render")
	}
	if v.Current != "3 losses" || v.Class != "loss" {
		t.Errorf("current = %q class %q, want 3 losses/loss", v.Current, v.Class)
	}
	if v.Best != "7 wins" {
		t.Errorf("best = %q, want 7 wins", v.Best)
	}
	// a drawn run is tinted neutrally, not as a win or a loss
	if d := NewStreakView(db.Streaks{CurrentLen: 2, CurrentScore: 0.5}); d.Class != "draw" {
		t.Errorf("drawn streak class = %q, want draw", d.Class)
	}
}

// TestSharePercentsTotal100 locks the largest-remainder split. Rounding each
// share independently lets three equal thirds print as 33/33/33 — 99% — and a
// readout showing all three side by side is exactly where that is noticed.
func TestSharePercentsTotal100(t *testing.T) {
	cases := []db.Record{
		rec(3, 1, 1, 1), // the classic 33/33/33 = 99 trap
		rec(6, 1, 1, 4), // 17/17/67 = 101 the other way
		rec(7, 2, 2, 3),
		rec(1, 0, 0, 1),
		rec(100, 58, 8, 34),
		rec(9, 3, 3, 3),
		rec(11, 4, 3, 4),
	}
	for _, c := range cases {
		w, d, l := sharePercents(c)
		if w+d+l != 100 {
			t.Errorf("sharePercents(%+v) = %d/%d/%d, totals %d, want 100",
				c, w, d, l, w+d+l)
		}
		if w < 0 || d < 0 || l < 0 {
			t.Errorf("sharePercents(%+v) produced a negative share", c)
		}
	}
	// no games makes no claim at all
	if w, d, l := sharePercents(rec(0, 0, 0, 0)); w+d+l != 0 {
		t.Errorf("empty record = %d/%d/%d, want zeroes", w, d, l)
	}
}

// TestDominantShare covers the label that frames each column: the biggest share
// and its tint, with ties breaking toward the better result.
func TestDominantShare(t *testing.T) {
	cases := []struct {
		w, d, l          int
		wantPct, wantCls string
	}{
		{58, 8, 34, "58%", "win"},
		{20, 10, 70, "70%", "loss"},
		{25, 50, 25, "50%", "draw"},
		{50, 0, 50, "50%", "win"},  // tie: the better result wins
		{0, 50, 50, "50%", "draw"}, // tie: draw beats loss
	}
	for _, c := range cases {
		pct, cls := dominantShare(c.w, c.d, c.l)
		if pct != c.wantPct || cls != c.wantCls {
			t.Errorf("dominantShare(%d,%d,%d) = %q/%q, want %q/%q",
				c.w, c.d, c.l, pct, cls, c.wantPct, c.wantCls)
		}
	}
}

// TestLengthsShares checks the per-bucket split reaches the view, and that an
// empty bucket claims nothing rather than printing 0%.
func TestLengthsShares(t *testing.T) {
	v := NewLengths([]db.LengthRecord{
		{Plies: 5, Record: rec(4, 3, 0, 1)},
	})
	b := v.Buckets[0]
	if b.Win != "75%" || b.Draw != "0%" || b.Loss != "25%" {
		t.Errorf("shares = %s/%s/%s, want 75%%/0%%/25%%", b.Win, b.Draw, b.Loss)
	}
	if b.Top != "75%" || b.TopClass != "win" {
		t.Errorf("dominant = %q/%q, want 75%%/win", b.Top, b.TopClass)
	}
	// a bucket with no games in it carries no label
	if empty := v.Buckets[1]; empty.Top != "" || empty.Win != "" {
		t.Errorf("empty bucket labelled: %q / %q", empty.Top, empty.Win)
	}
}
