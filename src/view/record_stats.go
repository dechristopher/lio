package view

import (
	"sort"
	"strconv"

	"github.com/dechristopher/lio/db"
)

// Core record analytics (arch/PROFILE_STATS.md Phase 3): the colour split, how
// games end, game length, recent form and streaks.
//
// Every proportional mark here is a win/draw/loss split, which is *semantic* —
// so unlike the rating curve these do wear the --win/--draw/--loss tokens the
// clocks and archive already use. Those three are deliberately not retinted by
// the board theme, so a result reads the same colour everywhere on the site.

// WDLBar is a win/draw/loss split rendered as three proportional widths. The
// segments are laid out by CSS flex-grow rather than percentages so they always
// total exactly the track width — percentage rounding leaves a sliver of
// background at the end of a bar, which reads as a fourth category.
type WDLBar struct {
	Wins   int64
	Draws  int64
	Losses int64
	Games  int64
	// WinPct is the headline number beside the bar ("48%"), rounded for display.
	WinPct string
}

// NewWDLBar builds a proportional bar from a record.
func NewWDLBar(r db.Record) WDLBar {
	b := WDLBar{Wins: r.Wins, Draws: r.Draws, Losses: r.Losses, Games: r.Games}
	if r.Games > 0 {
		b.WinPct = strconv.FormatInt((r.Wins*100+r.Games/2)/r.Games, 10) + "%"
	}
	return b
}

// Grow renders a segment's flex-grow value. Zero-count segments collapse
// entirely rather than rendering a hairline that suggests a result that never
// happened.
func (b WDLBar) Grow(n int64) string { return strconv.FormatInt(n, 10) }

// ColorSplitView is the account's record in one seat, with its bar.
type ColorSplitView struct {
	Label string // "As White" / "As Black"
	RecordView
	Bar WDLBar
}

// NewColorSplits renders the seat split. Both seats always render once the
// account has any games: "you have never played Black" is itself worth seeing,
// and a missing row would read as a rendering bug rather than a fact.
func NewColorSplits(rows []db.ColorRecord) []ColorSplitView {
	seen := map[bool]db.Record{}
	for _, r := range rows {
		seen[r.AsWhite] = r.Record
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]ColorSplitView, 0, 2)
	for _, white := range []bool{true, false} {
		label := "As Black"
		if white {
			label = "As White"
		}
		rec := seen[white]
		out = append(out, ColorSplitView{
			Label:      label,
			RecordView: NewRecordView(rec),
			Bar:        NewWDLBar(rec),
		})
	}
	return out
}

// EndingView is one way games finish, with how those games went.
type EndingView struct {
	// Label is the human name of the method token ("Checkmate", "On time").
	Label string
	RecordView
	Bar WDLBar
	// Width is the row's share of the busiest ending, so the rows read as a
	// frequency ranking rather than every bar filling its track.
	Width string
}

// NewEndings renders the endings breakdown. Rows arrive busiest-first, so the
// first row sets the scale.
func NewEndings(rows []db.EndingRecord) []EndingView {
	if len(rows) == 0 {
		return nil
	}
	top := rows[0].Games
	for _, r := range rows {
		if r.Games > top {
			top = r.Games
		}
	}
	out := make([]EndingView, 0, len(rows))
	for _, r := range rows {
		v := EndingView{
			Label:      endingLabel(r.Reason),
			RecordView: NewRecordView(r.Record),
			Bar:        NewWDLBar(r.Record),
			Width:      "100%",
		}
		if top > 0 {
			v.Width = strconv.FormatInt(r.Games*100/top, 10) + "%"
		}
		out = append(out, v)
	}
	return out
}

// endingLabel names a DB-canonical method token. The vocabulary matches
// resultReasons in lio-game.js — the same game should not be described one way
// in the result overlay and another on a profile — recast from the overlay's
// adverbial phrasing ("by checkmate") into a standalone label.
func endingLabel(reason string) string {
	switch reason {
	case "checkmate":
		return "Checkmate"
	case "resignation":
		return "Resignation"
	case "time":
		return "On time"
	case "stalemate":
		return "Stalemate"
	case "insufficient":
		return "Insufficient material"
	case "agreement":
		return "Agreement"
	case "repetition":
		return "Repetition"
	case "moverule":
		return "25-move rule"
	case "abandoned":
		return "Opponent left"
	case "":
		// pre-tag archive rows carry no method; naming them honestly beats
		// inventing one or dropping the games from the tally
		return "Unrecorded"
	default:
		return reason
	}
}

// lengthBuckets is how ply counts are grouped for the distribution. Octad games
// are short — a 4x4 board with four pieces a side — so the interesting spread is
// under 40 plies and the tail is a single "longer" bucket rather than a run of
// near-empty columns.
var lengthBuckets = []struct {
	Label string
	Max   int // inclusive; the final entry is the open-ended tail
}{
	{"1-10", 10},
	{"11-20", 20},
	{"21-30", 30},
	{"31-40", 40},
	{"41+", 0},
}

// LengthBarView is one column of the game-length histogram.
type LengthBarView struct {
	Label  string
	Games  int64
	Count  string
	Height string // CSS height, share of the fullest bucket
	Bar    WDLBar

	// Top is the bucket's dominant result as a percentage ("58%"), tinted with
	// TopClass. It frames the column: knowing the biggest share and its colour
	// makes the two remaining segments readable at a glance, which a bare stack
	// of three unlabelled bands is not.
	Top      string
	TopClass string
	// Win / Draw / Loss are the full split for the readout, always totalling
	// exactly 100%.
	Win  string
	Draw string
	Loss string
}

// LengthsView is the game-length distribution plus its headline figure.
type LengthsView struct {
	Buckets []LengthBarView
	// Median is the middle game's length, which describes a skewed
	// distribution far better than a mean that one 60-ply grind can drag.
	Median string
	Games  int64
}

// NewLengths buckets the per-ply counts into the histogram.
func NewLengths(rows []db.LengthRecord) LengthsView {
	if len(rows) == 0 {
		return LengthsView{}
	}
	var v LengthsView
	buckets := make([]db.Record, len(lengthBuckets))
	for _, r := range rows {
		b := &buckets[bucketOf(r.Plies)]
		b.Games += r.Games
		b.Wins += r.Wins
		b.Draws += r.Draws
		b.Losses += r.Losses
		v.Games += r.Games
	}

	var top int64
	for _, b := range buckets {
		if b.Games > top {
			top = b.Games
		}
	}
	for i, def := range lengthBuckets {
		b := buckets[i]
		bar := LengthBarView{
			Label: def.Label, Games: b.Games,
			Count: strconv.FormatInt(b.Games, 10),
			Bar:   NewWDLBar(b), Height: "0%",
		}
		if top > 0 {
			bar.Height = strconv.FormatInt(b.Games*100/top, 10) + "%"
		}
		if b.Games > 0 {
			w, d, l := sharePercents(b)
			bar.Win = strconv.Itoa(w) + "%"
			bar.Draw = strconv.Itoa(d) + "%"
			bar.Loss = strconv.Itoa(l) + "%"
			bar.Top, bar.TopClass = dominantShare(w, d, l)
		}
		v.Buckets = append(v.Buckets, bar)
	}
	v.Median = strconv.Itoa(medianPlies(rows, v.Games)) + " plies"
	return v
}

// sharePercents splits a record into whole win/draw/loss percentages that total
// exactly 100.
//
// Largest remainder, not independent rounding: rounding each share on its own
// lets 1/3-1/3-1/3 print as 33/33/33 (99) and 1/6-1/6-4/6 as 17/17/67 (101).
// A readout that shows three percentages side by side is exactly where that
// missing or extra point gets noticed.
func sharePercents(r db.Record) (win, draw, loss int) {
	if r.Games == 0 {
		return 0, 0, 0
	}
	counts := [3]int64{r.Wins, r.Draws, r.Losses}
	var out [3]int
	var assigned int
	// floor each share, remembering the fractional part left behind
	type rem struct {
		idx  int
		frac int64
	}
	var rems []rem
	for i, c := range counts {
		scaled := c * 100
		out[i] = int(scaled / r.Games)
		assigned += out[i]
		rems = append(rems, rem{i, scaled % r.Games})
	}
	// hand the leftover points to the biggest remainders, ties going to the
	// earlier category so the result is deterministic
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].frac > rems[b].frac })
	for i := 0; assigned < 100; i++ {
		out[rems[i%len(rems)].idx]++
		assigned++
	}
	return out[0], out[1], out[2]
}

// dominantShare names the biggest of the three shares and its tint. Ties break
// toward the better result — a bucket split evenly between wins and losses reads
// as the win, which is the same optimism the score line takes when it calls an
// even match a draw rather than a loss.
func dominantShare(win, draw, loss int) (string, string) {
	switch {
	case win >= draw && win >= loss:
		return strconv.Itoa(win) + "%", "win"
	case draw >= loss:
		return strconv.Itoa(draw) + "%", "draw"
	default:
		return strconv.Itoa(loss) + "%", "loss"
	}
}

// bucketOf maps a ply count to its histogram bucket index.
func bucketOf(plies int) int {
	for i, b := range lengthBuckets {
		if b.Max > 0 && plies <= b.Max {
			return i
		}
	}
	return len(lengthBuckets) - 1
}

// medianPlies finds the middle game's length by walking the already-sorted
// per-ply counts — no need to materialise one entry per game.
func medianPlies(rows []db.LengthRecord, total int64) int {
	if total == 0 {
		return 0
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Plies < rows[j].Plies })
	half, seen := total/2, int64(0)
	for _, r := range rows {
		seen += r.Games
		if seen > half {
			return r.Plies
		}
	}
	return rows[len(rows)-1].Plies
}

// FormPip is one game in the recent-form strip.
type FormPip struct {
	Class string // win / draw / loss
	Label string // "Won vs cdpplayer, 2 days ago"
	// Result / Detail are this one game's readout, split so the result can be
	// tinted with the same win/draw/loss token the pip itself wears. A pip
	// inside a match carries its own, so hovering a single game in a match
	// reports that game rather than the match around it.
	Result string // "Won by checkmate"
	Detail string // "vs cdpplayer · ½ + 1 blitz · 2 days ago"
	URL    string
	// Latest marks the newest game on the page. Highlighted with an outline
	// rather than a size or border change, so drawing attention to it costs no
	// layout shift.
	Latest bool
}

// FormGroup is one match: the run of consecutive games played in the same room,
// with the account's score across them.
type FormGroup struct {
	Pips  []FormPip
	Score string // "2½–½", from the account's perspective
	Class string // win / draw / loss tint for the match result
	// Result / Detail are the hover/tap readout, split so the result takes the
	// match's win/draw/loss tint. The strip itself stays a bare run of pips so it
	// survives a narrow viewport; this carries the context.
	Result string // "Won 2½–½"
	Detail string // "vs tester · ½ + 1 blitz · 2 days ago · partial match"
	// Pips is how many games the group holds, emitted as a flex-grow weight so a
	// match takes width proportional to its length and every pip on the row ends
	// up the same size. Server-side because CSS cannot count children.
	PipCount string
	// FirstURL is the match's opening game, where clicking the container leads.
	//
	// Built from the room id rather than taken from the oldest pip: on a match
	// the window cut into, the oldest *shown* game is not the match's first one,
	// and "go to the first game" pointing at game 3 is a quiet lie. Room-less
	// (backfilled) games never form a match, so they never need this.
	FirstURL string
	// Partial marks a match the recent-games window cut into. Its Score is the
	// score of the games *shown*, not the match's real final score — the tooltip
	// says so, since a number a viewer cannot check should not be the only place
	// the difference lives.
	Partial bool
}

// NewFormGroups turns the recent-games list into per-match runs of result pips.
//
// It reuses the games the page already loaded rather than taking a query of its
// own, and reverses them: the list reads newest-first, but a form strip is a
// timeline and has to run oldest to newest. Games are grouped by *consecutive*
// room, not by collecting every game of a room — the strip is a timeline, so a
// room revisited later is a separate run on it.
//
// olderRoomID is the room of the first game beyond the window, used only to
// decide whether the oldest group is cut off. Empty when the account has no
// older games, in which case nothing is partial.
func NewFormGroups(games []ProfileGameView, olderRoomID string) []FormGroup {
	if len(games) == 0 {
		return nil
	}
	var out []FormGroup
	// the newest game of each group, kept to build the match readout once the
	// group's full membership (and therefore its score) is known, and each
	// group's room, which addresses its opening game
	var newest []ProfileGameView
	var rooms []string
	for i := len(games) - 1; i >= 0; i-- {
		g := games[i]
		pip := FormPip{
			Class:  g.Class,
			Label:  g.Result + " vs " + g.Opponent + ", " + g.When,
			Result: gameResult(g),
			Detail: readoutDetail(g),
			URL:    g.URL,
			Latest: i == 0,
		}
		// a room-less game (backfilled) never merges with a neighbour
		if n := len(out); n > 0 && g.RoomID != "" && g.RoomID == games[i+1].RoomID {
			out[n-1].Pips = append(out[n-1].Pips, pip)
			newest[n-1] = g
			continue
		}
		out = append(out, FormGroup{Pips: []FormPip{pip}})
		newest = append(newest, g)
		rooms = append(rooms, g.RoomID)
	}
	// the oldest group is the only one the window can have cut into
	if olderRoomID != "" && games[len(games)-1].RoomID != "" &&
		olderRoomID == games[len(games)-1].RoomID {
		out[0].Partial = true
	}
	for i := range out {
		scoreGroup(&out[i])
		if room := rooms[i]; room != "" {
			out[i].FirstURL = "/" + room + "/1"
		} else {
			out[i].FirstURL = out[i].Pips[0].URL
		}
		// a lone game reports itself; a match reports its score. Both are built
		// after scoring, since the match line quotes the score.
		if out[i].Match() {
			out[i].Result = matchResultWord(out[i].Class) + " " + out[i].Score
		} else {
			out[i].Result = gameResult(newest[i])
		}
		out[i].PipCount = strconv.Itoa(len(out[i].Pips))
		out[i].Detail = readoutDetail(newest[i])
		if out[i].Partial {
			out[i].Detail += " · partial match"
		}
	}
	return out
}

// gameResult is the tinted half of a game's readout: the outcome and how it was
// reached ("Won by checkmate", "Lost on time").
func gameResult(g ProfileGameView) string {
	if p := ReasonPhrase(g.Reason); p != "" {
		return g.Result + " " + p
	}
	return g.Result
}

// readoutDetail is the untinted half: who, what and when. A match uses the same
// builder as a game — the two lines differ only in their result half, so they
// read identically apart from the thing that actually differs.
func readoutDetail(g ProfileGameView) string {
	out := "vs " + g.Opponent + " · " + g.Variant
	if g.When != "" {
		out += " · " + g.When
	}
	return out
}

// ReasonPhrase renders a DB-canonical method token as the adverbial phrase that
// follows a result ("Won *by checkmate*"). This is the second rendering of the
// vocabulary endingLabel names standalone — the endings card wants a noun for a
// row label, a readout wants a phrase — and both mirror resultReasons in
// lio-game.js so a game is not described one way in the result overlay and
// another on a profile.
func ReasonPhrase(reason string) string {
	switch reason {
	case "checkmate":
		return "by checkmate"
	case "resignation":
		return "by resignation"
	case "time":
		return "on time"
	case "stalemate":
		return "by stalemate"
	case "insufficient":
		return "for insufficient material"
	case "agreement":
		return "by agreement"
	case "repetition":
		return "by repetition"
	case "moverule":
		return "by the 25-move rule"
	case "abandoned":
		return "when their opponent left"
	}
	return ""
}

// matchResultWord names a match outcome in the same vocabulary a single game
// uses, so the readout reads identically whichever is hovered.
func matchResultWord(class string) string {
	switch class {
	case "win":
		return "Won"
	case "loss":
		return "Lost"
	default:
		return "Drew"
	}
}

// Match reports whether the group is a real match rather than a lone game.
//
// A Partial group counts even at one pip: it is one end of a match whose other
// games fall outside the window, so it is a match the page can only see part of
// — not a one-off game.
func (fg FormGroup) Match() bool { return len(fg.Pips) > 1 || fg.Partial }

// scoreGroup folds a match's pips into its score line. Points come from the pip
// classes, which are already this account's per-game results, so the match score
// cannot disagree with the dots above it.
func scoreGroup(fg *FormGroup) {
	var mine float64
	for _, p := range fg.Pips {
		switch p.Class {
		case "win":
			mine++
		case "draw":
			mine += 0.5
		}
	}
	theirs := float64(len(fg.Pips)) - mine
	fg.Score = FormatPoints(mine) + "–" + FormatPoints(theirs)
	switch {
	case mine > theirs:
		fg.Class = "win"
	case mine < theirs:
		fg.Class = "loss"
	default:
		fg.Class = "draw"
	}
}

// StreakView is the current and best-streak summary beside the form strip.
type StreakView struct {
	Current string // "3 wins" / "2 losses"
	Class   string // win / draw / loss tint for the current run
	Best    string // "7 wins"
	Show    bool
}

// NewStreakView phrases the streak figures. A streak of one is not a streak, so
// the current run only renders from two games up — otherwise every player who
// just won a game is told they are "on a 1-game streak".
func NewStreakView(s db.Streaks) StreakView {
	v := StreakView{}
	if s.BestWins > 1 {
		v.Best = plural(int64(s.BestWins), "win", "wins")
		v.Show = true
	}
	if s.CurrentLen > 1 {
		switch s.CurrentScore {
		case 1:
			v.Current, v.Class = plural(int64(s.CurrentLen), "win", "wins"), "win"
		case 0:
			v.Current, v.Class = plural(int64(s.CurrentLen), "loss", "losses"), "loss"
		default:
			v.Current, v.Class = plural(int64(s.CurrentLen), "draw", "draws"), "draw"
		}
		v.Show = true
	}
	return v
}

// FormReadoutDefault is what the readout shows before anything is hovered: the
// newest *game*, which is the one wearing the white ring.
//
// It is deliberately the game and not its match. The white ring marks one game,
// so a default line describing the match around it would label a highlight that
// is not the highlight — the "Latest:" prefix the view prepends only reads true
// if the two agree. It also means the line is never blank, so its reserved
// height is never mistaken for a gap.
func FormReadoutDefault(groups []FormGroup) FormPip {
	if len(groups) == 0 {
		return FormPip{}
	}
	last := groups[len(groups)-1]
	if len(last.Pips) == 0 {
		return FormPip{}
	}
	return last.Pips[len(last.Pips)-1]
}

// heatLevel is the density class for an activity cell. A helper rather than
// string concatenation in the template so the level can never render as an
// unstyled class name the stylesheet does not define.
func heatLevel(level int) string {
	if level < 0 {
		level = 0
	}
	if level > activityLevels {
		level = activityLevels
	}
	return "l" + strconv.Itoa(level)
}

// heatRate is the diverging-ramp class for an activity cell, kept beside
// heatLevel so neither can render a class the stylesheet does not define.
func heatRate(level int) string {
	if level < 0 {
		level = 0
	}
	if level > rateLevels {
		level = rateLevels
	}
	return "r" + strconv.Itoa(level)
}

// RatingDelta renders a rated game's rating change and its tint. Nil (an unrated
// game, or one archived before deltas were recorded) renders nothing rather than
// a zero, which would claim the rating held steady when in fact it was never
// touched.
func RatingDelta(d *int) (string, string) {
	if d == nil {
		return "", ""
	}
	switch {
	case *d > 0:
		return "+" + strconv.Itoa(*d), "win"
	case *d < 0:
		// U+2212 minus, not a hyphen: it aligns with the digits in the
		// tabular figures the row is set in
		return "\u2212" + strconv.Itoa(-*d), "loss"
	}
	return "±0", "draw"
}

// MoveCount labels a game's length in half-moves. Plies rather than moves
// because that is the unit the archive stores and the Game length section
// buckets by — one page should not count a game two different ways.
func MoveCount(plies int) string {
	if plies <= 0 {
		return ""
	}
	return plural(int64(plies), "ply", "plies")
}
