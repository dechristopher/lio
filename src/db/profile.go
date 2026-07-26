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
		if asWhite {
			g.Score = r.WhiteScore
			g.OpponentName = strOrEmpty(r.BlackUsername)
			g.OpponentTitle = strOrEmpty(r.BlackTitleCode)
			g.OpponentIsBot = game.SeatIsBot(r.BlackUid, r.BlackUserID)
		} else {
			g.Score = r.BlackScore
			g.OpponentName = strOrEmpty(r.WhiteUsername)
			g.OpponentTitle = strOrEmpty(r.WhiteTitleCode)
			g.OpponentIsBot = game.SeatIsBot(r.WhiteUid, r.WhiteUserID)
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

// TotalsForUser returns an account's lifetime record.
func TotalsForUser(userID int64) (Record, error) {
	if Pool == nil {
		return Record{}, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).ProfileTotals(ctx, &userID)
	if err != nil {
		return Record{}, err
	}
	return Record{
		Games:  row.Games,
		Wins:   row.Wins,
		Draws:  row.Draws,
		Losses: row.Losses,
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
