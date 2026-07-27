package view

import (
	"sort"
	"strconv"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/opening"
)

// Formation intelligence (arch/PROFILE_STATS.md Phase 4): what the account
// deploys, what it faces, and which specific clashes it wins.
//
// This is the section no chess site can show. Every octad game opens with each
// side arranging king, knight and two pawns on its home rank — 12 formations a
// side, 144 ordered matchups — and unlike a chess opening the choice is *blind*,
// made without seeing the opponent's. So "The Redoubt scores 38% for me" is not
// trivia: it is the only feedback loop a player has on a decision they make
// every single game with no information.
//
// The archive stores the deployed position, not the names. opening.Names owns
// the naming table, so the fold from OFEN to names happens here rather than in
// SQL — one table, one place.

// minFormationGames is how many games a formation needs before the page reports
// a percentage for it. Below this the number swings wildly on one result and
// invites a player to abandon a formation on no evidence.
const minFormationGames = 3

// FormationView is one named formation with the account's record behind it.
type FormationView struct {
	Name string
	RecordView
	Bar WDLBar
	// Score is the account's points share ("58%"), the comparable figure across
	// formations played different numbers of times. Empty below the reporting
	// threshold, where a percentage would be noise.
	Score string
	// Width is the row's share of the most-played formation, so the list reads
	// as a frequency ranking before it reads as a performance one.
	Width string
	// Thin marks a formation with too few games to score. The row still renders
	// — that a player has tried it twice is itself worth seeing — but it makes
	// no claim about how it went.
	Thin bool
}

// MatchupView is one specific clash: the account's formation against the
// opponent's, from one seat.
type MatchupView struct {
	// Name is the matchup's own proper name from the opening package, e.g.
	// "Gravity Well" — the thing two players can actually talk about.
	Name string
	// Mine / Them are oriented to the account, so they read "yours vs theirs"
	// regardless of which colour it held.
	Mine string
	Them string
	// Seat says which side the account played. A matchup name denotes a
	// *White-vs-Black* clash, so the same name is two opposite experiences and
	// the page has to say which one it is reporting.
	Seat string
	RecordView
	Score string
}

// FormationsView is the whole section.
type FormationsView struct {
	// Mine and Theirs are the account's own formations and the ones played
	// against it, both most-played first.
	Mine   []FormationView
	Theirs []FormationView
	// Best and Worst are the account's strongest and weakest matchups among
	// those it has played enough times to judge. Empty when nothing qualifies.
	Best  []MatchupView
	Worst []MatchupView
	// Games is how many deploy games fed the section.
	Games int64
}

// matchupsShown bounds the best/worst lists. A leaderboard of every matchup
// played would be a table, not a highlight.
const matchupsShown = 3

// NewFormations folds per-position records into named formations and matchups.
//
// Each row arrives as (starting OFEN, seat), which opening.Names resolves into
// the White formation, the Black formation and the matchup's name. The account's
// own formation is whichever side it sat on — the same position is "The Redoubt"
// to White and something else entirely to Black, so the seat is what makes a row
// mean anything.
func NewFormations(rows []db.FormationRecord) FormationsView {
	var v FormationsView
	mine := map[string]*db.Record{}
	theirs := map[string]*db.Record{}
	matchups := map[string]*matchupAgg{}

	for _, r := range rows {
		white, black, matchup, ok := opening.Names(r.StartingOFEN)
		if !ok {
			// not a legal deploy start: a classic (non-deploy) game, or a row
			// whose OFEN predates the current board. It has no formation to
			// report, so it is not a formation game.
			continue
		}
		myName, theirName := white, black
		if !r.AsWhite {
			myName, theirName = black, white
		}
		v.Games += r.Games
		addRecord(mine, myName, r.Record)
		addRecord(theirs, theirName, r.Record)

		// Keyed by name *and seat*. A matchup name denotes a White-vs-Black
		// clash, so the same name covers two opposite experiences: the account's
		// score playing one side is the complement of its score playing the
		// other. Folding them together would average a strength against a
		// weakness and report neither.
		key := matchup + "\x00" + strconv.FormatBool(r.AsWhite)
		m, seen := matchups[key]
		if !seen {
			seat := "as Black"
			if r.AsWhite {
				seat = "as White"
			}
			m = &matchupAgg{Name: matchup, Mine: myName, Them: theirName, Seat: seat}
			matchups[key] = m
		}
		m.rec.Games += r.Games
		m.rec.Wins += r.Wins
		m.rec.Draws += r.Draws
		m.rec.Losses += r.Losses
	}

	v.Mine = formationList(mine)
	v.Theirs = formationList(theirs)
	v.Best, v.Worst = splitMatchups(matchups)
	return v
}

// matchupAgg accumulates one matchup while the rows are being folded.
type matchupAgg struct {
	Name string
	Mine string
	Them string
	Seat string
	rec  db.Record
}

// addRecord folds a record into a named bucket.
func addRecord(into map[string]*db.Record, name string, r db.Record) {
	cur, ok := into[name]
	if !ok {
		cur = &db.Record{}
		into[name] = cur
	}
	cur.Games += r.Games
	cur.Wins += r.Wins
	cur.Draws += r.Draws
	cur.Losses += r.Losses
}

// formationList renders a name→record map as a ranked list, most-played first.
func formationList(m map[string]*db.Record) []FormationView {
	if len(m) == 0 {
		return nil
	}
	out := make([]FormationView, 0, len(m))
	var top int64
	for name, r := range m {
		if r.Games > top {
			top = r.Games
		}
		f := FormationView{
			Name:       name,
			RecordView: NewRecordView(*r),
			Bar:        NewWDLBar(*r),
			Thin:       r.Games < minFormationGames,
		}
		if !f.Thin {
			f.Score = scorePercent(*r)
		}
		out = append(out, f)
	}
	// most played first; ties by name so the order is stable between renders
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bar.Games != out[j].Bar.Games {
			return out[i].Bar.Games > out[j].Bar.Games
		}
		return out[i].Name < out[j].Name
	})
	for i := range out {
		out[i].Width = "100%"
		if top > 0 {
			out[i].Width = strconv.FormatInt(out[i].Bar.Games*100/top, 10) + "%"
		}
	}
	return out
}

// splitMatchups picks the account's strongest and weakest clashes, considering
// only matchups played often enough to mean something.
//
// A matchup that appears in both lists would be a contradiction, so when there
// are too few qualifying matchups to fill both, the worst list yields: a "best"
// claim on thin evidence is merely optimistic, a "worst" one tells a player to
// stop doing something.
func splitMatchups(m map[string]*matchupAgg) (best, worst []MatchupView) {
	var pool []MatchupView
	for _, a := range m {
		if a.rec.Games < minFormationGames {
			continue
		}
		pool = append(pool, MatchupView{
			Name:       a.Name,
			Mine:       a.Mine,
			Them:       a.Them,
			Seat:       a.Seat,
			RecordView: NewRecordView(a.rec),
			Score:      scorePercent(a.rec),
		})
	}
	if len(pool) == 0 {
		return nil, nil
	}
	sort.Slice(pool, func(i, j int) bool {
		pi, pj := pointsOf(pool[i]), pointsOf(pool[j])
		if pi != pj {
			return pi > pj
		}
		if pool[i].Name != pool[j].Name {
			return pool[i].Name < pool[j].Name
		}
		return pool[i].Seat < pool[j].Seat
	})

	best = pool[:min(matchupsShown, len(pool))]
	// the worst list takes from the other end, and only what best did not claim
	if rest := len(pool) - len(best); rest > 0 {
		tail := pool[len(best):]
		n := min(matchupsShown, len(tail))
		worst = make([]MatchupView, 0, n)
		for i := 0; i < n; i++ {
			worst = append(worst, tail[len(tail)-1-i])
		}
	}
	return best, worst
}

// pointsOf is a matchup's score share, used only for ranking.
func pointsOf(m MatchupView) float64 {
	games, _ := strconv.ParseFloat(m.Games, 64)
	if games == 0 {
		return 0
	}
	wins, _ := strconv.ParseFloat(m.Wins, 64)
	draws, _ := strconv.ParseFloat(m.Draws, 64)
	return (wins + 0.5*draws) / games
}

// scorePercent renders a record as its points share — wins plus half the draws,
// over games. This is the figure that compares across formations played
// different numbers of times, and it counts draws for what they are worth
// rather than discarding them as a plain win rate would.
func scorePercent(r db.Record) string {
	if r.Games == 0 {
		return ""
	}
	pts := float64(r.Wins) + 0.5*float64(r.Draws)
	return strconv.FormatInt(int64(pts*100/float64(r.Games)+0.5), 10) + "%"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
