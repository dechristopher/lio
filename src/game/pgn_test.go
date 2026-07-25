package game

import (
	"strings"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/variant"
)

// newTestGame builds a short game from the given (possibly empty) start OFEN and
// plays n legal moves, for exercising BuildPGN.
func newTestGame(t *testing.T, ofen string, n int) *OctadGame {
	t.Helper()
	g, err := NewOctadGame(OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		White:   "white",
		Black:   "black",
		OFEN:    ofen,
	})
	if err != nil {
		t.Fatalf("NewOctadGame(%q) failed: %v", ofen, err)
	}
	for i := 0; i < n; i++ {
		moves := g.Game.ValidMoves()
		if len(moves) == 0 {
			break
		}
		if err := g.Game.Move(moves[0]); err != nil {
			t.Fatalf("playing move %s failed: %v", moves[0], err)
		}
	}
	return g
}

func sampleMeta() PGNMeta {
	return PGNMeta{
		Site:           "https://octad.gg",
		Variant:        "1+0",
		Group:          "blitz",
		White:          "drewtest",
		Black:          "BOT",
		WhiteUID:       "uid_drew",
		BlackUID:       "",
		Result:         "1-0",
		Reason:         "checkmate",
		Start:          time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC),
		End:            time.Date(2026, 7, 22, 9, 3, 30, 0, time.UTC),
		StartOFEN:      "ppkn/4/4/NKPP w NCFncf - 0 1",
		WhiteFormation: "The Standard",
		BlackFormation: "The Standard",
		Matchup:        "Standing Wave",
	}
}

// TestBuildPGNDeterministic verifies BuildPGN is a pure function of its inputs:
// the same meta + game + timing produce byte-identical output. This is the
// guarantee that lets the live archival path and the archive-page rebuild agree.
func TestBuildPGNDeterministic(t *testing.T) {
	g := newTestGame(t, "", 4)
	m := sampleMeta()
	a := BuildPGN(m, &g.Game, g.MoveTimes)
	b := BuildPGN(m, &g.Game, g.MoveTimes)
	if a != b {
		t.Fatalf("BuildPGN is not deterministic:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}

// TestBuildPGNNameTags verifies the opening/matchup names are emitted as tag
// pairs (and omitted when absent).
func TestBuildPGNNameTags(t *testing.T) {
	g := newTestGame(t, "", 2)
	pgn := BuildPGN(sampleMeta(), &g.Game, g.MoveTimes)
	for _, want := range []string{
		`[WhiteFormation "The Standard"]`,
		`[BlackFormation "The Standard"]`,
		`[Matchup "Standing Wave"]`,
	} {
		if !strings.Contains(pgn, want) {
			t.Errorf("PGN missing %s:\n%s", want, pgn)
		}
	}

	// an empty WhiteFormation omits all three name tags
	m := sampleMeta()
	m.WhiteFormation, m.BlackFormation, m.Matchup = "", "", ""
	bare := BuildPGN(m, &g.Game, g.MoveTimes)
	if strings.Contains(bare, "Formation") || strings.Contains(bare, "[Matchup ") {
		t.Errorf("PGN should omit name tags when unresolved:\n%s", bare)
	}
}

// TestBuildPGNEventTag verifies the Event tag names the situation the game was
// played in — rating stake, speed ("Casual" for the untimed variants), single
// game vs race-to match, and the engine opponent — instead of a fixed
// placeholder string. The blind deploy pre-game is never named: every game is
// played that way, so the deploy group resolves to the speed of the time
// control it shares a label with.
func TestBuildPGNEventTag(t *testing.T) {
	const deployOFEN = "knpp/4/4/PNKP w NCFncf - 0 1"

	cases := []struct {
		name  string
		meta  func(m *PGNMeta)
		event string
	}{
		{"unrated blitz", func(m *PGNMeta) {}, "Unrated Blitz game"},
		{"rated blitz", func(m *PGNMeta) { m.Rated = true }, "Rated Blitz game"},
		{"vs bot", func(m *PGNMeta) { m.VsBot = true }, "Unrated Blitz game vs Computer"},
		{"bullet", func(m *PGNMeta) { m.Group = "bullet" }, "Unrated Bullet game"},
		{"untimed casual", func(m *PGNMeta) { m.Group = "unlimited" }, "Unrated Casual game"},
		{"race to match", func(m *PGNMeta) { m.Rated, m.RaceTo = true, 3 }, "Rated Blitz match (race to 3)"},
		// the deploy start alone says nothing about the game: it is how every
		// game starts now
		{"deploy start", func(m *PGNMeta) { m.StartOFEN = deployOFEN }, "Unrated Blitz game"},
		{
			// the deploy group resolves through its shared time-control label
			// ("½ + 1" is the blitz control) to the speed word
			"deploy group reads as its speed",
			func(m *PGNMeta) { m.Group, m.Variant, m.StartOFEN = "deploy", "½ + 1", deployOFEN },
			"Unrated Blitz game",
		},
		{
			"deploy group rapid vs bot",
			func(m *PGNMeta) { m.Group, m.Variant, m.VsBot = "deploy", "1 + 2", true },
			"Unrated Rapid game vs Computer",
		},
		{
			// a deploy variant whose label no longer resolves drops the speed
			// word rather than claiming the wrong one
			"unresolvable deploy control",
			func(m *PGNMeta) { m.Group, m.Variant = "deploy", "9 + 9" },
			"Unrated game",
		},
		{
			"untimed deploy vs bot",
			func(m *PGNMeta) { m.Group, m.StartOFEN, m.VsBot = "unlimited", deployOFEN, true },
			"Unrated Casual game vs Computer",
		},
		// an unknown group (a newer variant group reaching an old binary) still
		// produces a sane Event rather than a mislabeled one
		{"unknown group", func(m *PGNMeta) { m.Group = "marathon" }, "Unrated marathon game"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGame(t, "", 2)
			m := sampleMeta()
			tc.meta(&m)
			want := `[Event "` + tc.event + `"]`
			if pgn := BuildPGN(m, &g.Game, g.MoveTimes); !strings.Contains(pgn, want) {
				t.Errorf("PGN missing %s:\n%s", want, pgn)
			}
		})
	}
}

// TestBuildPGNClockAndReimport verifies %clk comments are emitted per move when
// timing is present, and the annotated PGN still re-imports move-for-move
// (octad's decoder strips comments).
func TestBuildPGNClockAndReimport(t *testing.T) {
	g := newTestGame(t, "", 0)
	for i := 0; i < 3; i++ {
		moves := g.Game.ValidMoves()
		if err := g.Game.Move(moves[0]); err != nil {
			t.Fatalf("move %d failed: %v", i, err)
		}
		g.MoveTimes = append(g.MoveTimes, MoveTime{ThinkMs: int64(i) * 500, ClockMs: 60000 - int64(i)*500})
	}
	pgn := BuildPGN(sampleMeta(), &g.Game, g.MoveTimes)

	if got, want := strings.Count(pgn, "[%clk "), 3; got != want {
		t.Fatalf("PGN carries %d %%clk comments, want %d:\n%s", got, want, pgn)
	}

	sc := octad.NewScanner(strings.NewReader(pgn + "\n\n"))
	if !sc.Scan() {
		t.Fatalf("re-scanning PGN failed: %v", sc.Err())
	}
	if got, want := len(sc.Next().Moves()), 3; got != want {
		t.Errorf("re-imported PGN has %d moves, want %d", got, want)
	}
}
