package db

import (
	"context"
	"errors"
	"math"

	"github.com/dechristopher/octad/v2"
	"github.com/jackc/pgx/v5"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/rating"
)

// SeatRating is one seat's rating change from a finished rated game, surfaced to
// the room so it can refresh clocks and show the game-over delta.
type SeatRating struct {
	UID     string // the seat's uid — the room maps it to the correct player
	Display string // new rating display ("1658" / "1500?") — live clock/popup
	AtGame  string // pre-game rating display — stored per game for the archive
	Delta   int    // signed rounded rating change (+8 / -8)
}

// RatingResult pairs both seats' changes from a rated game.
type RatingResult struct {
	White SeatRating
	Black SeatRating
}

// Glicko-2 ratings data plane (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5). The
// rating update runs transactionally inside archiveGame (applyRatingUpdate);
// the display reads are used to capture RatingDisplay at seat-claim and to fill
// the profile popover's ratings summary. Like the other accounts accessors,
// these run only against a live pool.

// CategoryRating pairs a time-control category with the player's rating there.
type CategoryRating struct {
	Category string
	Rating   rating.Rating
}

// RatingOrDefault returns a player's rating in a category, or the unrated
// default (1500/350/0.06) when they have no row yet. Used for the seat-claim
// display capture, where a provisional "1500?" is the right thing to show.
func RatingOrDefault(userID int64, category string) rating.Rating {
	if Pool == nil {
		return rating.New()
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).GetRating(ctx, gen.GetRatingParams{
		UserID:   userID,
		Category: category,
	})
	if err != nil {
		return rating.New()
	}
	return rating.Rating{R: row.Rating, RD: row.Rd, Sigma: row.Volatility, Games: int(row.Games)}
}

// ListRatingsForUser returns only the categories a user has actually played
// (existing rows) — the profile popover summary.
func ListRatingsForUser(userID int64) ([]CategoryRating, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListRatingsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryRating, 0, len(rows))
	for _, r := range rows {
		out = append(out, CategoryRating{
			Category: r.Category,
			Rating:   rating.Rating{R: r.Rating, RD: r.Rd, Sigma: r.Volatility, Games: int(r.Games)},
		})
	}
	return out, nil
}

// applyRatingUpdate applies the Glicko-2 change for a finished rated game inside
// the archive transaction. It is a no-op unless the game is rated, both seats
// are logged-in (distinct) accounts, and the game had a real result. Both
// players' rating rows are locked in ascending user_id order to avoid deadlocks
// with a simultaneous game that shares a player; missing rows default to unrated.
func applyRatingUpdate(ctx context.Context, q *gen.Queries, rec GameRecord) (*RatingResult, error) {
	if !rec.Rated || rec.WhiteUserID == nil || rec.BlackUserID == nil {
		return nil, nil
	}
	white, black := *rec.WhiteUserID, *rec.BlackUserID
	if white == black {
		return nil, nil // same account both seats is never rated
	}

	var whiteScore float64
	switch octad.Outcome(rec.Outcome) {
	case octad.WhiteWon:
		whiteScore = rating.Win
	case octad.BlackWon:
		whiteScore = rating.Loss
	case octad.Draw:
		whiteScore = rating.Draw
	default:
		return nil, nil // aborted / no result — ratings untouched
	}
	category := rec.VariantGroup

	// lock both rows in a stable global order (ascending user_id)
	lockOrder := []int64{white, black}
	if lockOrder[0] > lockOrder[1] {
		lockOrder[0], lockOrder[1] = lockOrder[1], lockOrder[0]
	}
	current := make(map[int64]rating.Rating, 2)
	for _, id := range lockOrder {
		r, err := ratingForUpdate(ctx, q, id, category)
		if err != nil {
			return nil, err
		}
		current[id] = r
	}

	wr, br := current[white], current[black]
	// both updates use pre-game opponent ratings (br/wr, not the new values)
	newWhite := wr.Update(br, whiteScore)
	newBlack := br.Update(wr, rating.Win-whiteScore)

	if err := upsertRating(ctx, q, white, category, newWhite); err != nil {
		return nil, err
	}
	if err := upsertRating(ctx, q, black, category, newBlack); err != nil {
		return nil, err
	}

	// the change surfaced to clients: new display + signed rounded delta, keyed
	// by seat uid so the room can map it to the right player across a rematch.
	// AtGame is the pre-game display, snapshotted onto the game row for the
	// archive ("rating at the time of the game").
	return &RatingResult{
		White: SeatRating{UID: rec.WhiteUID, Display: newWhite.Display(), AtGame: wr.Display(), Delta: int(math.Round(newWhite.R - wr.R))},
		Black: SeatRating{UID: rec.BlackUID, Display: newBlack.Display(), AtGame: br.Display(), Delta: int(math.Round(newBlack.R - br.R))},
	}, nil
}

// ratingForUpdate row-locks and reads one player's rating, defaulting to unrated
// when absent.
func ratingForUpdate(ctx context.Context, q *gen.Queries, userID int64, category string) (rating.Rating, error) {
	row, err := q.GetRatingForUpdate(ctx, gen.GetRatingForUpdateParams{
		UserID:   userID,
		Category: category,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return rating.New(), nil
	}
	if err != nil {
		return rating.Rating{}, err
	}
	return rating.Rating{R: row.Rating, RD: row.Rd, Sigma: row.Volatility, Games: int(row.Games)}, nil
}

func upsertRating(ctx context.Context, q *gen.Queries, userID int64, category string, r rating.Rating) error {
	return q.UpsertRating(ctx, gen.UpsertRatingParams{
		UserID:     userID,
		Category:   category,
		Rating:     r.R,
		Rd:         r.RD,
		Volatility: r.Sigma,
		Games:      int32(r.Games),
	})
}
