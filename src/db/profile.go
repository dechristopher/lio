package db

import (
	"time"

	"github.com/google/uuid"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/game"
)

// Public player-page reads (arch/ADMIN_MODERATION.md). Everything here is
// oriented to *one account's perspective*: the archive stores per-seat scores,
// so each accessor resolves which seat the account held and reports the result
// from that side. Doing the flip once, here, keeps the view and handler from
// each re-deriving it (and getting it subtly different, as the seat/uid
// confusion in the archive page once did).
//
// Like the other archive accessors these degrade quietly: an unconfigured
// Postgres yields empty results rather than an error, since a profile page
// without a database is simply a page with nothing on it.

// Record is a win/draw/loss tally from one account's perspective.
type Record struct {
	Games  int64
	Wins   int64
	Draws  int64
	Losses int64
}

// Points is the account's score across the tallied games (win 1, draw ½) — the
// same arithmetic the match scoreboard uses.
func (r Record) Points() float64 {
	return float64(r.Wins) + 0.5*float64(r.Draws)
}

// Lifetime is an account's whole-history facts beyond the win/draw/loss tally:
// when they first played and how long they have spent at the board. Both ride
// along on the totals query, which already scans exactly these rows.
//
// Played is wall-clock game duration summed across every game, so it counts
// both sides' thinking — it is "time spent playing octad", not "time this
// account's clock ran". FirstGame is zero for an account with no games.
type Lifetime struct {
	FirstGame time.Time
	Played    time.Duration
}

// VariantRecord is a Record scoped to one time control.
type VariantRecord struct {
	Name  string
	Group string
	Record
}

// BotRecord is a Record against one bot persona. Persona is the engine.Personas
// key, empty for games archived before the persona ladder (which all played at
// full Queen strength — the display layer resolves empty the same way the
// archive does).
type BotRecord struct {
	Persona string
	Record
}

// ProfileGame is one row of an account's recent-games list, already flipped to
// that account's perspective.
type ProfileGame struct {
	GameID       uuid.UUID
	RoomID       string
	GameIndex    int16
	Start        time.Time
	VariantName  string
	VariantGroup string
	Casual       bool
	Rated        bool
	Reason       string
	// Score is this account's result in the game: 1, 0.5 or 0.
	Score float32
	// Plies is the game's length in half-moves.
	Plies int
	// OppRating is the opponent's display rating going into the game ("1520" /
	// "1500?"), empty for an unrated game or an anon/bot seat. Delta is the
	// change this game made to *this* account's rating; nil when unrated.
	OppRating string
	Delta     *int
	// Opponent identity. Name is empty for an anonymous human; IsBot marks the
	// engine seat, in which case BotPersona names the difficulty.
	OpponentName  string
	OpponentTitle string
	OpponentIsBot bool
	BotPersona    string
}

// ListGamesForUser returns an account's most recent games, newest first.
func ListGamesForUser(userID int64, limit, offset int32) ([]ProfileGame, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListGamesForUserID(ctx, gen.ListGamesForUserIDParams{
		WhiteUserID: &userID,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ProfileGame, 0, len(rows))
	for _, r := range rows {
		// which seat did this account hold? Everything else follows from it
		asWhite := r.WhiteUserID != nil && *r.WhiteUserID == userID
		g := ProfileGame{
			GameID:       r.GameID,
			RoomID:       r.RoomID,
			GameIndex:    r.GameIndex,
			Start:        r.StartTs.Time,
			VariantName:  r.VariantName,
			VariantGroup: r.VariantGroup,
			Casual:       r.Casual,
			Rated:        r.Rated,
			Reason:       r.Reason,
			BotPersona:   strOrEmpty(r.BotPersona),
		}
		// The result comes from outcome + seat, never from white_score /
		// black_score: those hold the *cumulative match score* at the time the
		// game finished, so from game 2 of a match onward they are not this
		// game's result at all (see the header note in db/query/profile.sql).
		g.Score = SeatScore(r.Outcome, asWhite)
		g.Plies = int(r.Plies)
		if asWhite {
			g.OpponentName = strOrEmpty(r.BlackUsername)
			g.OpponentTitle = strOrEmpty(r.BlackTitleCode)
			g.OpponentIsBot = game.SeatIsBot(r.BlackUid, r.BlackUserID)
			g.OppRating = strOrEmpty(r.BlackRating)
			g.Delta = intOrNil(r.WhiteRatingDelta)
		} else {
			g.OpponentName = strOrEmpty(r.WhiteUsername)
			g.OpponentTitle = strOrEmpty(r.WhiteTitleCode)
			g.OpponentIsBot = game.SeatIsBot(r.WhiteUid, r.WhiteUserID)
			g.OppRating = strOrEmpty(r.WhiteRating)
			g.Delta = intOrNil(r.BlackRatingDelta)
		}
		if !g.OpponentIsBot {
			// bot_persona is stamped on the game, not the seat; it is
			// meaningless on a human game and must not leak into the label
			g.BotPersona = ""
		}
		out = append(out, g)
	}
	return out, nil
}

// intOrNil widens an optional archived rating delta, dropping the storage width.
func intOrNil(v *int16) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

// SeatScore is one seat's result in a game: 1 for a win, 0.5 for a draw, 0 for
// a loss. The Go twin of the CASE every profile aggregate uses, and the only
// correct way to read a per-game result out of the archive — the
// games.*_match_score columns are match-cumulative and cannot answer it (see the
// header note in db/query/profile.sql).
//
// Exported because the archive page needs the same answer for its match
// timeline; one implementation means the profile and the archive cannot come to
// different conclusions about who won a game.
func SeatScore(outcome string, asWhite bool) float32 {
	switch outcome {
	case "1/2-1/2":
		return 0.5
	case "1-0":
		if asWhite {
			return 1
		}
	case "0-1":
		if !asWhite {
			return 1
		}
	}
	return 0
}

// TotalsForUser returns an account's lifetime record along with the whole-history
// facts the identity card shows beside it. One query, because the totals scan
// already visits precisely the rows both answers need.
func TotalsForUser(userID int64) (Record, Lifetime, error) {
	if Pool == nil {
		return Record{}, Lifetime{}, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).ProfileTotals(ctx, &userID)
	if err != nil {
		return Record{}, Lifetime{}, err
	}
	return Record{
			Games:  row.Games,
			Wins:   row.Wins,
			Draws:  row.Draws,
			Losses: row.Losses,
		}, Lifetime{
			// Valid is false for an account with no games at all, where min()
			// over an empty set is NULL — the zero Time the view reads as "no
			// first game yet".
			FirstGame: row.FirstGame.Time,
			Played:    time.Duration(row.PlayedSeconds) * time.Second,
		}, nil
}

// VariantRecordsForUser returns an account's record per time control, busiest
// first.
func VariantRecordsForUser(userID int64) ([]VariantRecord, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ProfileByVariant(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := make([]VariantRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, VariantRecord{
			Name:  r.VariantName,
			Group: r.VariantGroup,
			Record: Record{
				Games: r.Games, Wins: r.Wins, Draws: r.Draws, Losses: r.Losses,
			},
		})
	}
	return out, nil
}

// BotRecordsForUser returns an account's record against each bot persona,
// busiest first.
func BotRecordsForUser(userID int64) ([]BotRecord, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ProfileVsBots(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := make([]BotRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, BotRecord{
			Persona: strOrEmpty(r.BotPersona),
			Record: Record{
				Games: r.Games, Wins: r.Wins, Draws: r.Draws, Losses: r.Losses,
			},
		})
	}
	return out, nil
}
