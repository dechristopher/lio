package view

import (
	"testing"

	"github.com/dechristopher/lio/db"
)

// The standard deploy start: White "nkpp" (The Standard), Black "ppkn" reversed
// to "nkpp" as well — both sides in their default arrangement.
const stdOFEN = "ppkn/4/4/NKPP w NCFncf - 0 1"

func formRec(ofen string, asWhite bool, g, w, d, l int64) db.FormationRecord {
	return db.FormationRecord{
		StartingOFEN: ofen, AsWhite: asWhite,
		Record: db.Record{Games: g, Wins: w, Draws: d, Losses: l},
	}
}

// TestNewFormationsSeatOrientation locks the rule that makes the whole section
// mean anything: the same position is one formation to White and a different
// one to Black, so the seat decides which name is "yours".
func TestNewFormationsSeatOrientation(t *testing.T) {
	white, black, _, ok := formationNamesOf(t, stdOFEN)

	asWhite := NewFormations([]db.FormationRecord{formRec(stdOFEN, true, 4, 3, 0, 1)})
	if !ok || len(asWhite.Mine) != 1 || asWhite.Mine[0].Name != white {
		t.Fatalf("as White, mine = %+v, want %q", asWhite.Mine, white)
	}
	if asWhite.Theirs[0].Name != black {
		t.Errorf("as White, theirs = %q, want %q", asWhite.Theirs[0].Name, black)
	}

	// the identical position from the other seat swaps which name is the
	// account's own
	asBlack := NewFormations([]db.FormationRecord{formRec(stdOFEN, false, 4, 3, 0, 1)})
	if asBlack.Mine[0].Name != black {
		t.Errorf("as Black, mine = %q, want %q", asBlack.Mine[0].Name, black)
	}
	if asBlack.Theirs[0].Name != white {
		t.Errorf("as Black, theirs = %q, want %q", asBlack.Theirs[0].Name, white)
	}
}

// TestNewFormationsSkipsNonDeploy checks that a position which is not a legal
// deploy start contributes nothing rather than being named wrongly.
func TestNewFormationsSkipsNonDeploy(t *testing.T) {
	v := NewFormations([]db.FormationRecord{
		formRec("4/4/4/4 w - - 0 1", true, 5, 5, 0, 0),
		formRec("", true, 2, 1, 0, 1),
	})
	if v.Games != 0 || len(v.Mine) != 0 {
		t.Errorf("non-deploy rows produced %d games / %d formations", v.Games, len(v.Mine))
	}
}

// TestFormationThinThreshold covers the reporting floor: a formation tried once
// or twice still lists, but claims no percentage.
func TestFormationThinThreshold(t *testing.T) {
	v := NewFormations([]db.FormationRecord{formRec(stdOFEN, true, 2, 2, 0, 0)})
	if len(v.Mine) != 1 {
		t.Fatalf("got %d formations, want 1", len(v.Mine))
	}
	if !v.Mine[0].Thin {
		t.Error("2 games should be below the reporting threshold")
	}
	if v.Mine[0].Score != "" {
		t.Errorf("thin formation scored %q, want no claim", v.Mine[0].Score)
	}
	// at the threshold it does report
	v = NewFormations([]db.FormationRecord{formRec(stdOFEN, true, 3, 3, 0, 0)})
	if v.Mine[0].Thin || v.Mine[0].Score != "100%" {
		t.Errorf("3 games = thin:%v score:%q, want false/100%%", v.Mine[0].Thin, v.Mine[0].Score)
	}
}

// TestScorePercentCountsDraws locks that the figure is a points share, not a
// win rate — a draw is half a point and discarding it would misrank a formation
// that draws often against one that loses often.
func TestScorePercentCountsDraws(t *testing.T) {
	cases := []struct {
		r    db.Record
		want string
	}{
		{db.Record{Games: 4, Wins: 2, Draws: 0, Losses: 2}, "50%"},
		{db.Record{Games: 4, Wins: 0, Draws: 4, Losses: 0}, "50%"}, // all draws == even
		{db.Record{Games: 4, Wins: 1, Draws: 2, Losses: 1}, "50%"},
		{db.Record{Games: 4, Wins: 4, Draws: 0, Losses: 0}, "100%"},
		{db.Record{Games: 0}, ""},
	}
	for _, c := range cases {
		if got := scorePercent(c.r); got != c.want {
			t.Errorf("scorePercent(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

// TestMatchupsDoNotOverlap guards the one way best/worst can embarrass the page:
// the same matchup appearing in both lists.
func TestMatchupsDoNotOverlap(t *testing.T) {
	// one qualifying matchup — it can only be "best", never both
	v := NewFormations([]db.FormationRecord{formRec(stdOFEN, true, 5, 4, 0, 1)})
	if len(v.Best) != 1 || len(v.Worst) != 0 {
		t.Errorf("one matchup gave best=%d worst=%d, want 1/0", len(v.Best), len(v.Worst))
	}
	// a matchup below the threshold qualifies for neither
	thin := NewFormations([]db.FormationRecord{formRec(stdOFEN, true, 2, 2, 0, 0)})
	if len(thin.Best) != 0 || len(thin.Worst) != 0 {
		t.Errorf("thin matchup ranked: best=%d worst=%d", len(thin.Best), len(thin.Worst))
	}
}

// formationNamesOf resolves a start's names through the same package the view
// uses, so the test cannot drift from the naming table.
func formationNamesOf(t *testing.T, ofen string) (white, black, matchup string, ok bool) {
	t.Helper()
	v := NewFormations([]db.FormationRecord{formRec(ofen, true, 1, 1, 0, 0)})
	if len(v.Mine) == 0 || len(v.Theirs) == 0 {
		t.Fatalf("%q did not resolve to formations", ofen)
	}
	return v.Mine[0].Name, v.Theirs[0].Name, "", true
}

// TestMatchupsSeparateSeats locks the fix for a real bug: a matchup name denotes
// a White-vs-Black clash, so the *same* name covers two opposite experiences.
// Keying only by name folded them together, averaging the account's score
// playing one side with its score playing the other — which are complements, so
// the result described neither.
func TestMatchupsSeparateSeats(t *testing.T) {
	v := NewFormations([]db.FormationRecord{
		// won every game of this clash as White
		formRec(stdOFEN, true, 4, 4, 0, 0),
		// lost every game of the very same clash as Black
		formRec(stdOFEN, false, 4, 0, 0, 4),
	})

	all := append(append([]MatchupView{}, v.Best...), v.Worst...)
	if len(all) != 2 {
		t.Fatalf("got %d matchups, want 2 (one per seat)", len(all))
	}
	seats := map[string]string{}
	for _, m := range all {
		if m.Name != all[0].Name {
			t.Errorf("expected one matchup name, got %q and %q", all[0].Name, m.Name)
		}
		seats[m.Seat] = m.Score
	}
	if seats["as White"] != "100%" {
		t.Errorf("as White = %q, want 100%%", seats["as White"])
	}
	if seats["as Black"] != "0%" {
		t.Errorf("as Black = %q, want 0%%", seats["as Black"])
	}
	// the seats must not have been averaged into a single 50% row
	if len(seats) != 2 {
		t.Errorf("seats collapsed: %+v", seats)
	}
}

// TestMatchupOrientation checks the pair reads from the account's side whichever
// colour it held, since "yours vs theirs" is the only orientation a reader can
// interpret without knowing the naming table's convention.
func TestMatchupOrientation(t *testing.T) {
	asBlack := NewFormations([]db.FormationRecord{formRec(stdOFEN, false, 3, 3, 0, 0)})
	if len(asBlack.Best) != 1 {
		t.Fatalf("got %d matchups, want 1", len(asBlack.Best))
	}
	m := asBlack.Best[0]
	if m.Seat != "as Black" {
		t.Errorf("Seat = %q, want as Black", m.Seat)
	}
	// Mine is the account's own formation — the black side of the position
	if m.Mine != asBlack.Mine[0].Name {
		t.Errorf("matchup Mine = %q but formation list says %q", m.Mine, asBlack.Mine[0].Name)
	}
	if m.Them != asBlack.Theirs[0].Name {
		t.Errorf("matchup Them = %q but faced list says %q", m.Them, asBlack.Theirs[0].Name)
	}
}
