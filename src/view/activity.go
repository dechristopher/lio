package view

import (
	"strconv"
	"time"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/engine"
)

// Activity & social (arch/PROFILE_STATS.md Phase 5): the year heatmap, the
// accounts played most, and the bot ladder as a progression.

// activityWeeks is how many columns the heatmap spans — a year plus the partial
// week at each end.
const activityWeeks = 53

// monthLabelGap is the minimum number of columns between two month labels. A
// three-letter label is wider than one 0.72rem cell, so closer than this they
// collide.
const monthLabelGap = 3

// activityLevels is the number of density steps above zero. Four is enough to
// read "a bit / some / a lot / a great deal" without asking anyone to
// distinguish shades that differ by a few percent.
const activityLevels = 4

// The win-rate ramp is diverging: two hues either side of a neutral middle,
// never a rainbow. rateMid is the even-scoring step, which must be the grey one
// — a diverging scale with a hue at its midpoint implies a third category.
const (
	rateLevels = 4
	rateMid    = 2
)

// ActivityDayView is one cell of the heatmap. It carries both ramps, because
// the toggle swaps which one the stylesheet reads rather than re-rendering:
// switching a year of cells should not cost a round trip.
type ActivityDayView struct {
	// Level is 0 (no games) to activityLevels — how much was played, a
	// sequential ramp in one hue.
	Level int
	// Rate is the diverging ramp: how *well* the day went. 0 is loss-heavy,
	// rateMid is even, rateLevels is win-heavy. Only meaningful when Level > 0.
	Rate int
	// Future marks a cell after today. The calendar starts on a week boundary,
	// so the final column usually runs past the current day; those cells render
	// as holes rather than as zero-activity days the account never had.
	Future bool
	// Label is the accessible/native description; When, Games and Score feed the
	// custom tooltip.
	Label string
	When  string // "4 Mar 2026"
	Games string // "3 games"
	Score string // "0.67", empty on a day with no games
}

// ActivityWeekView is one column: seven days, Sunday first.
type ActivityWeekView struct {
	Days [7]ActivityDayView
}

// ActivityMonthView is a month label above the grid, positioned by the column
// its first week falls in.
type ActivityMonthView struct {
	Name string
	// Col is the 1-based grid column, used directly as a grid-column-start so
	// the label sits over the week the month begins in.
	Col string
}

// ActivityView is the heatmap plus its summary.
type ActivityView struct {
	Weeks  []ActivityWeekView
	Months []ActivityMonthView
	// Total / Days summarise the window: how many games, across how many days
	// the account actually played.
	Total   string
	Days    string
	Busiest string
	Show    bool
}

// NewActivityView lays a year of per-day counts onto a week grid.
//
// The calendar is built from `now` backwards to the Sunday on or before a year
// ago, so every column is a whole week and the weekday rows line up. Days with
// no games are absent from the query, which is the point: the grid is dense and
// the data is sparse, so the view fills the calendar rather than the database
// storing a row per empty day.
func NewActivityView(days []db.ActivityDay, now time.Time) ActivityView {
	var v ActivityView
	if len(days) == 0 {
		return v
	}
	v.Show = true

	byDay := make(map[string]db.Record, len(days))
	var total, busiest int64
	for _, d := range days {
		byDay[dayKey(d.Day)] = d.Record
		total += d.Games
		if d.Games > busiest {
			busiest = d.Games
		}
	}
	v.Total = plural(total, "game", "games")
	v.Days = plural(int64(len(days)), "day", "days")
	v.Busiest = "busiest day " + plural(busiest, "game", "games")

	today := now.UTC().Truncate(24 * time.Hour)
	// walk back to the Sunday that starts the first column
	start := today.AddDate(0, 0, -(activityWeeks-1)*7)
	start = start.AddDate(0, 0, -int(start.Weekday()))

	lastMonth, lastLabelCol := "", -99
	for w := 0; w < activityWeeks; w++ {
		var week ActivityWeekView
		for d := 0; d < 7; d++ {
			date := start.AddDate(0, 0, w*7+d)
			cell := ActivityDayView{Future: date.After(today)}
			if !cell.Future {
				rec := byDay[dayKey(date)]
				cell.Level = densityLevel(rec.Games, busiest)
				cell.Rate = rateLevel(rec)
				cell.Label = activityLabel(rec.Games, date)
				cell.When = date.Format("2 Jan 2006")
				if rec.Games > 0 {
					cell.Games = plural(rec.Games, "game", "games")
					cell.Score = ScoreRate(rec)
				}
			}
			week.Days[d] = cell
		}
		v.Weeks = append(v.Weeks, week)

		// A month label goes over the column its first week falls in, but only
		// if there is room: a three-letter label is wider than one cell, so two
		// labels a column apart overlap into "JulAug". The year almost always
		// opens mid-month, and that stub month is the usual culprit.
		if m := start.AddDate(0, 0, w*7).Format("Jan"); m != lastMonth {
			if w-lastLabelCol >= monthLabelGap {
				v.Months = append(v.Months, ActivityMonthView{
					Name: m, Col: strconv.Itoa(w + 1),
				})
				lastLabelCol = w
			}
			lastMonth = m
		}
	}
	return v
}

// densityLevel buckets a day's game count into the ramp. Buckets are relative to
// the account's own busiest day, so a heatmap reads the same whether someone
// plays two games a week or forty — an absolute scale would render a casual
// player's whole year as the faintest step.
func densityLevel(games, busiest int64) int {
	if games <= 0 {
		return 0
	}
	if busiest <= 1 {
		return activityLevels
	}
	lvl := int((games*int64(activityLevels) + busiest - 1) / busiest)
	if lvl < 1 {
		lvl = 1
	}
	if lvl > activityLevels {
		lvl = activityLevels
	}
	return lvl
}

// rateLevel buckets a day's scoring rate onto the diverging ramp. A day with no
// games has no rate at all and sits at the neutral middle, where the volume ramp
// renders it as empty anyway.
//
// The bands are deliberately narrow around even: 0.45–0.55 reads as neutral, so
// a day split 1-1 is not painted as a good or a bad one.
func rateLevel(r db.Record) int {
	if r.Games == 0 {
		return rateMid
	}
	rate := (float64(r.Wins) + 0.5*float64(r.Draws)) / float64(r.Games)
	switch {
	case rate < 0.25:
		return 0
	case rate < 0.45:
		return 1
	case rate <= 0.55:
		return rateMid
	case rate <= 0.75:
		return 3
	default:
		return rateLevels
	}
}

// ScoreRate renders a record as an average scoring rate — points per game, where
// a win is 1 and a draw is a half. "0.65" rather than "65%": this is a rate per
// game, the same figure a scoreboard reports, and a percentage invites reading
// it as a win *percentage*, which it is not.
func ScoreRate(r db.Record) string {
	if r.Games == 0 {
		return ""
	}
	pts := float64(r.Wins) + 0.5*float64(r.Draws)
	return strconv.FormatFloat(pts/float64(r.Games), 'f', 2, 64)
}

// RateClass tints a scoring rate: above even is a win, below it a loss, and
// exactly even is neutral rather than being rounded into one of them.
func RateClass(r db.Record) string {
	if r.Games == 0 {
		return ""
	}
	pts := float64(r.Wins) + 0.5*float64(r.Draws)
	switch rate := pts / float64(r.Games); {
	case rate > 0.5:
		return "win"
	case rate < 0.5:
		return "loss"
	default:
		return "draw"
	}
}

// activityLabel is a cell's hover text. A day with no games still gets one, so
// pointing anywhere on the grid answers rather than staying silent.
func activityLabel(games int64, date time.Time) string {
	when := date.Format("2 Jan 2006")
	if games == 0 {
		return "No games on " + when
	}
	return plural(games, "game", "games") + " on " + when
}

// dayKey is the UTC calendar-day key both sides of the lookup agree on.
func dayKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// OpponentView is one row of the most-played list.
type OpponentView struct {
	Username string
	Title    string
	URL      string
	RecordView
	Bar WDLBar
	// Score is the average scoring rate against them ("0.65"), Class its tint.
	Score string
	Class string
}

// NewOpponents renders the most-played opponents.
func NewOpponents(rows []db.OpponentRecord) []OpponentView {
	out := make([]OpponentView, 0, len(rows))
	for _, r := range rows {
		out = append(out, OpponentView{
			Username:   r.Username,
			Title:      r.TitleCode,
			URL:        "/@/" + r.Username,
			RecordView: NewRecordView(r.Record),
			Bar:        NewWDLBar(r.Record),
			Score:      ScoreRate(r.Record),
			Class:      RateClass(r.Record),
		})
	}
	return out
}

// BotRungView is one step of the bot ladder.
type BotRungView struct {
	Name  string
	Glyph string
	RecordView
	Bar WDLBar
	// Score is the average scoring rate against this persona ("0.42"); empty
	// when it has never played one. Class is its tint.
	Score string
	Class string
	// Beaten marks a rung the account has won at least one game on. The ladder
	// is a progression, so "have I ever taken one off this bot" is the question
	// it answers — not a win rate, which the score already gives.
	Beaten bool
	Played bool
}

// NewBotLadder renders every persona in ladder order, weakest first, whether or
// not the account has met it.
//
// Unplayed rungs render too: the ladder is the shape of the climb, and a gap in
// it is information ("you have never faced the Rook"). A list of only the bots
// already played cannot show what is left.
func NewBotLadder(bots []BotRecordView) []BotRungView {
	byName := make(map[string]BotRecordView, len(bots))
	for _, b := range bots {
		byName[b.Persona] = b
	}
	out := make([]BotRungView, 0, len(engine.Personas))
	for _, p := range engine.Personas {
		rung := BotRungView{Name: p.Name, Glyph: p.Glyph}
		if b, ok := byName[p.Name]; ok {
			rung.RecordView = b.RecordView
			rung.Bar = b.Bar
			rung.Played = b.Bar.Games > 0
			rung.Beaten = b.Bar.Wins > 0
			rec := db.Record{
				Games: b.Bar.Games, Wins: b.Bar.Wins,
				Draws: b.Bar.Draws, Losses: b.Bar.Losses,
			}
			rung.Score = ScoreRate(rec)
			rung.Class = RateClass(rec)
		}
		out = append(out, rung)
	}
	return out
}
