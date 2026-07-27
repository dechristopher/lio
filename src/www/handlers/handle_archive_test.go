package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/variant"
	"github.com/dechristopher/lio/view"
)

// TestArchivePGNRebuild verifies the archive-page PGN rebuild (archivePGN) maps
// an archived games row to the shared game.BuildPGN correctly: the seat names
// follow room.seatArchiveName ("[BOT] <glyph> <persona>" for the engine,
// "Anonymous" for an anon human), the deploy start becomes a SetUp/FEN tag, the
// opening/matchup names come from the starting OFEN, and the result re-imports
// move-for-move. This is the from-DB rebuild the copy button copies — no
// object-store fetch.
func TestArchivePGNRebuild(t *testing.T) {
	const startOFEN = "knpp/4/4/PNKP w NCFncf - 0 1"

	og, err := game.NewOctadGame(game.OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		White:   "uid_drew", // anonymous human (uid, no account)
		Black:   "",         // the bot
		OFEN:    startOFEN,
	})
	if err != nil {
		t.Fatalf("NewOctadGame failed: %v", err)
	}
	for i := 0; i < 4; i++ {
		moves := og.Game.ValidMoves()
		if len(moves) == 0 {
			break
		}
		if err := og.Game.Move(moves[0]); err != nil {
			t.Fatalf("move %d failed: %v", i, err)
		}
	}

	persona := "pawn"
	start := time.Date(2026, 7, 22, 14, 5, 0, 0, time.UTC)
	row := gen.Game{
		StartingOfen: startOFEN,
		// the blind deploy variant every room plays: its group is "deploy", which
		// the Event tag resolves to the speed of the "½ + 1" control (blitz)
		VariantName:  "½ + 1",
		VariantGroup: "deploy",
		Rated:        true,
		RaceTo:       3,
		WhiteUid:     "uid_drew",
		BlackUid:     "", // bot seat: no uid + no account
		BotPersona:   &persona,
		Outcome:      string(og.Game.Outcome()),
		Reason:       "checkmate",
		StartTs:      pgtype.Timestamptz{Time: start, Valid: true},
		EndTs:        pgtype.Timestamptz{Time: start.Add(3 * time.Minute), Valid: true},
	}

	pgn := archivePGN(row, og)

	// the bot seat carries its persona glyph + name; derive the expected string
	// from the same helpers to avoid a fragile literal glyph in the test
	wantBlack := `[Black "` + game.PGNSeatName("", "", view.BotSeatGlyph(persona), view.BotSeatLabel(persona), true) + `"]`
	for _, want := range []string{
		`[White "Anonymous"]`, wantBlack,
		`[WhiteUID "uid_drew"]`, `[BlackUID ""]`,
		`[SetUp "1"]`, `[FEN "` + startOFEN + `"]`,
		// the Event names the situation off the row: a rated blitz race-to match
		// against the engine
		`[Event "Rated Blitz match (race to 3) vs Computer"]`,
		// PNKP = The Citadel (white); knpp reversed = ppnk = The Bastion (black)
		`[WhiteFormation "The Citadel"]`, `[BlackFormation "The Bastion"]`,
		`[Matchup "White Dwarf"]`,
	} {
		if !strings.Contains(pgn, want) {
			t.Errorf("rebuilt PGN missing %s:\n%s", want, pgn)
		}
	}

	// the rebuilt PGN replays from the deploy start move-for-move
	sc := octad.NewScanner(strings.NewReader(pgn + "\n\n"))
	if !sc.Scan() {
		t.Fatalf("re-scanning rebuilt PGN failed: %v", sc.Err())
	}
	replayed := sc.Next()
	if got := replayed.Positions()[0].String(); got != startOFEN {
		t.Errorf("replayed start OFEN = %q, want %q", got, startOFEN)
	}
	if got, want := len(replayed.Moves()), len(og.Game.Moves()); got != want {
		t.Errorf("replayed %d moves, want %d", got, want)
	}
}

// TestReportTargetForSeats locks the archive page's report gating: it names the
// seat the viewer did *not* sit in, only for a seat that is a real account, and
// nothing at all for a visitor who did not play the game.
//
// Participation resolves by account id first and session uid second, so a
// player still recognises their own game from a new session, and a game played
// anonymously on the current session still resolves.
func TestReportTargetForSeats(t *testing.T) {
	const (
		drew = int64(1)
		cdp  = int64(2)
	)
	acct := func(id int64) *int64 { return &id }

	cases := []struct {
		name      string
		game      gen.Game
		viewerID  int64
		viewerUID string
		whiteName string
		blackName string
		want      string
	}{{
		name:      "viewer played white, reports black",
		game:      gen.Game{WhiteUserID: acct(drew), BlackUserID: acct(cdp)},
		viewerID:  drew,
		whiteName: "drewtest", blackName: "cdpplayer",
		want: "cdpplayer",
	}, {
		name:      "viewer played black, reports white",
		game:      gen.Game{WhiteUserID: acct(cdp), BlackUserID: acct(drew)},
		viewerID:  drew,
		whiteName: "cdpplayer", blackName: "drewtest",
		want: "cdpplayer",
	}, {
		// a spectator or a stranger browsing the permalink
		name:      "viewer played neither seat",
		game:      gen.Game{WhiteUserID: acct(cdp), BlackUserID: acct(drew)},
		viewerID:  99,
		whiteName: "cdpplayer", blackName: "drewtest",
		want: "",
	}, {
		// the bot has no account and no name to report
		name:      "opponent is the engine",
		game:      gen.Game{WhiteUserID: acct(drew), BlackUid: ""},
		viewerID:  drew,
		whiteName: "drewtest", blackName: "",
		want: "",
	}, {
		// an anonymous human has nothing to sanction
		name:      "opponent is anonymous",
		game:      gen.Game{WhiteUserID: acct(drew), BlackUid: "uid_anon"},
		viewerID:  drew,
		whiteName: "drewtest", blackName: "",
		want: "",
	}, {
		// played anonymously, signed in since — the uid still identifies the seat
		name:      "viewer matched by session uid",
		game:      gen.Game{WhiteUid: "uid_mine", BlackUserID: acct(cdp)},
		viewerID:  drew,
		viewerUID: "uid_mine",
		whiteName: "", blackName: "cdpplayer",
		want: "cdpplayer",
	}, {
		// an empty uid must never match an account-less seat
		name:      "empty uid matches nothing",
		game:      gen.Game{WhiteUid: "", BlackUid: ""},
		viewerID:  drew,
		viewerUID: "",
		whiteName: "", blackName: "",
		want: "",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reportTargetForSeats(tc.game, tc.viewerID, tc.viewerUID,
				tc.whiteName, tc.blackName)
			if got != tc.want {
				t.Errorf("reportTargetForSeats = %q, want %q", got, tc.want)
			}
		})
	}
}
