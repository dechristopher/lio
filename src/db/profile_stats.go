package db

import (
	"strconv"
	"strings"
	"time"

	"github.com/dechristopher/lio/db/gen"
)

// Profile statistics accessors (arch/PROFILE_STATS.md). Like the rest of the
// archive reads these degrade quietly: an unconfigured Postgres yields empty
// results rather than an error, since a profile page without a database is
// simply a page with nothing on it.

// RatingPoint is one sample of a rating curve: the rating an account carried
// into that day, and whether it was still provisional at the time.
type RatingPoint struct {
	Day    time.Time
	Rating int
	// Provisional mirrors the trailing "?" the archive recorded — the rating's
	// deviation was still above the provisional threshold when it was captured.
	// The curve renders that stretch differently rather than implying the early,
	// uncertain part of a history is as solid as the rest.
	Provisional bool
}

// RatingSeries is one time control's rating curve, oldest point first.
type RatingSeries struct {
	Category string
	Points   []RatingPoint
}

// RatingHistoryForUser returns the account's rating curve per category, each
// series ordered oldest-first. Series are returned in the query's category
// order; the display layer sorts them into canonical time-control order.
//
// The returned points stop at the account's last rated game — closing the series
// with the current rating is the caller's job, since only the ratings table
// knows where a curve ends today.
func RatingHistoryForUser(userID int64) ([]RatingSeries, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ProfileRatingHistory(ctx, &userID)
	if err != nil {
		return nil, err
	}

	// rows arrive grouped by category (the DISTINCT ON ordering guarantees it),
	// so one pass with a running index is enough — no map, and the series keep
	// their day ordering for free.
	var out []RatingSeries
	byCat := make(map[string]int, 8)
	for _, r := range rows {
		rating, provisional, ok := parseRating(r.Rating)
		if !ok {
			continue // an unparseable rating is a corrupt row, not a data point
		}
		i, seen := byCat[r.Category]
		if !seen {
			i = len(out)
			byCat[r.Category] = i
			out = append(out, RatingSeries{Category: r.Category})
		}
		out[i].Points = append(out[i].Points, RatingPoint{
			Day:         r.Day.Time,
			Rating:      rating,
			Provisional: provisional,
		})
	}
	return out, nil
}

// ColorRecord is the account's record in one seat.
type ColorRecord struct {
	AsWhite bool
	Record
}

// ColorSplitForUser returns the account's record as White and as Black, White
// first. Seats the account never held are absent rather than zero-filled.
func ColorSplitForUser(userID int64) ([]ColorRecord, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ProfileColorSplit(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := make([]ColorRecord, 0, 2)
	for _, r := range rows {
		out = append(out, ColorRecord{
			AsWhite: r.AsWhite,
			Record:  Record{Games: r.Games, Wins: r.Wins, Draws: r.Draws, Losses: r.Losses},
		})
	}
	// White first regardless of what the group-by happened to emit
	if len(out) == 2 && !out[0].AsWhite {
		out[0], out[1] = out[1], out[0]
	}
	return out, nil
}

// EndingRecord is how often the account's games ended one way, and how those
// games went for them. Reason is the DB-canonical method token.
type EndingRecord struct {
	Reason string
	Record
}

// EndingsForUser returns the account's games grouped by how they finished,
// busiest first.
func EndingsForUser(userID int64) ([]EndingRecord, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ProfileEndings(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := make([]EndingRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, EndingRecord{
			Reason: r.Reason,
			Record: Record{Games: r.Games, Wins: r.Wins, Draws: r.Draws, Losses: r.Losses},
		})
	}
	return out, nil
}

// LengthRecord is how many of the account's games ran to a given ply count.
type LengthRecord struct {
	Plies int
	Record
}

// LengthsForUser returns the account's games grouped by length in plies,
// shortest first.
func LengthsForUser(userID int64) ([]LengthRecord, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ProfileLengths(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := make([]LengthRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, LengthRecord{
			Plies:  int(r.Plies),
			Record: Record{Games: r.Games, Wins: r.Wins, Draws: r.Draws, Losses: r.Losses},
		})
	}
	return out, nil
}

// Streaks is the account's current run of identical results and its best-ever
// winning run.
type Streaks struct {
	// CurrentLen is 0 when the account has no games; CurrentScore is only
	// meaningful when it is not.
	CurrentLen   int
	CurrentScore float32
	BestWins     int
}

// StreaksForUser returns the account's current and best streaks.
func StreaksForUser(userID int64) (Streaks, error) {
	if Pool == nil {
		return Streaks{}, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).ProfileStreaks(ctx, &userID)
	if err != nil {
		return Streaks{}, err
	}
	return Streaks{
		CurrentLen:   int(row.CurrentLen),
		CurrentScore: row.CurrentScore,
		BestWins:     int(row.BestWins),
	}, nil
}

// parseRating splits a recorded display rating ("1653" / "1500?") into its
// value and whether it was provisional when captured.
func parseRating(s string) (rating int, provisional bool, ok bool) {
	s = strings.TrimSpace(s)
	if provisional = strings.HasSuffix(s, "?"); provisional {
		s = strings.TrimSuffix(s, "?")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false, false
	}
	return n, provisional, true
}

// FormationRecord is the account's record from one exact deployed start, in one
// seat. StartingOFEN is the raw position — naming the two formations is the
// display layer's job, since the opening package owns that table.
type FormationRecord struct {
	StartingOFEN string
	AsWhite      bool
	Record
}

// FormationsForUser returns the account's games grouped by deployed start and
// seat. Bounded by octad's deploy space (144 starts × 2 seats) rather than by
// how much the account has played.
func FormationsForUser(userID int64) ([]FormationRecord, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ProfileFormations(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := make([]FormationRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, FormationRecord{
			StartingOFEN: r.StartingOfen,
			AsWhite:      r.AsWhite,
			Record:       Record{Games: r.Games, Wins: r.Wins, Draws: r.Draws, Losses: r.Losses},
		})
	}
	return out, nil
}

// ActivityDay is how many games the account played on one UTC day.
type ActivityDay struct {
	Day time.Time
	Record
}

// ActivityForUser returns the account's games per day over the last year,
// oldest first. Days with no games are absent — the view fills the calendar.
func ActivityForUser(userID int64) ([]ActivityDay, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ProfileActivity(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := make([]ActivityDay, 0, len(rows))
	for _, r := range rows {
		out = append(out, ActivityDay{
			Day:    r.Day.Time,
			Record: Record{Games: r.Games, Wins: r.Wins, Draws: r.Draws, Losses: r.Losses},
		})
	}
	return out, nil
}

// OpponentRecord is the account's record against one other account.
type OpponentRecord struct {
	Username  string
	TitleCode string
	Record
}

// OpponentsForUser returns the accounts this one has played most, busiest
// first. Anonymous and bot seats are excluded by the query — neither is a
// person to have a rivalry with.
func OpponentsForUser(userID int64, limit int32) ([]OpponentRecord, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ProfileOpponents(ctx, gen.ProfileOpponentsParams{
		WhiteUserID: &userID,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]OpponentRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, OpponentRecord{
			Username:  r.Username,
			TitleCode: strOrEmpty(r.TitleCode),
			Record:    Record{Games: r.Games, Wins: r.Wins, Draws: r.Draws, Losses: r.Losses},
		})
	}
	return out, nil
}
