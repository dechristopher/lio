package backfill

import (
	"bytes"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/google/uuid"

	"github.com/dechristopher/lio/db"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name       string
		reasonTag  string
		result     string
		replayed   octad.Method
		wantMethod octad.Method
		wantReason string
	}{
		// board outcomes: replay is authoritative, tag confirms
		{"checkmate", "WHITE WINS BY CHECKMATE", "1-0", octad.Checkmate, octad.Checkmate, "checkmate"},
		{"stalemate", "DRAWN VIA STALEMATE", "1/2-1/2", octad.Stalemate, octad.Stalemate, "stalemate"},
		{"repetition", "DRAWN BY REPETITION", "1/2-1/2", octad.ThreefoldRepetition, octad.ThreefoldRepetition, "repetition"},
		{"insufficient", "DRAWN DUE TO INSUFFICIENT MATERIAL", "1/2-1/2", octad.InsufficientMaterial, octad.InsufficientMaterial, "insufficient"},
		{"moverule", "DRAWN DUE TO 25 MOVE RULE", "1/2-1/2", octad.TwentyFiveMoveRule, octad.TwentyFiveMoveRule, "moverule"},
		// declared outcomes: not in movetext, recovered from the tag
		{"resignation", "BLACK RESIGNED - WHITE WINS", "1-0", octad.NoMethod, octad.Resignation, "resignation"},
		{"agreement", "DRAWN BY AGREEMENT", "1/2-1/2", octad.NoMethod, octad.DrawOffer, "agreement"},
		// flag: no Method enum, stays NoMethod but reason is recoverable
		{"time white", "BLACK OUT OF TIME - WHITE WINS", "1-0", octad.NoMethod, octad.NoMethod, "time"},
		{"time black", "WHITE OUT OF TIME - BLACK WINS", "0-1", octad.NoMethod, octad.NoMethod, "time"},
		// missing tag: fall back to method/result
		{"empty decisive", "", "1-0", octad.NoMethod, octad.NoMethod, "resignation"},
		{"empty draw", "", "1/2-1/2", octad.NoMethod, octad.NoMethod, "agreement"},
		{"empty checkmate", "", "1-0", octad.Checkmate, octad.Checkmate, "checkmate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotMethod, gotReason := classify(c.reasonTag, c.result, c.replayed)
			if gotMethod != int16(c.wantMethod) {
				t.Errorf("method: got %d want %d", gotMethod, c.wantMethod)
			}
			if gotReason != c.wantReason {
				t.Errorf("reason: got %q want %q", gotReason, c.wantReason)
			}
		})
	}
}

// playToEnd plays deterministic first-legal moves until the game ends (or a ply
// cap), returning a terminal game with a board-derived method where possible.
func playToEnd(t *testing.T) *octad.Game {
	t.Helper()
	g, err := octad.NewGame()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300 && g.Outcome() == octad.NoOutcome; i++ {
		vm := g.ValidMoves()
		if len(vm) == 0 {
			break
		}
		if err := g.Move(vm[0]); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

// TestParsePGNRoundTrip proves that a game exported the way the live seam writes
// it parses back into the identical move/position encoding (byte-for-byte blob),
// the replayed method, and the tag metadata — the correctness invariant that
// lets backfilled positions dedupe against live ones.
func TestParsePGNRoundTrip(t *testing.T) {
	g := playToEnd(t)
	expectedBlob, _ := db.BuildPlies(g)
	moveCount := len(g.Moves())
	method := g.Method()
	result := lastField(g.String())

	g.AddTagPair("White", "u_white")
	g.AddTagPair("Black", "u_black")
	g.AddTagPair("Variant", "Test")
	g.AddTagPair("Group", "blitz")
	g.AddTagPair("Result", result)
	// real archived PGNs always carry a Reason tag; it is what recovers the
	// automatic-draw methods that octad's decoder does not re-detect on replay.
	g.AddTagPair("Reason", reasonTagFor(method))
	g.AddTagPair("Date", "2026.07.14")
	g.AddTagPair("Time", "09:30:00")
	g.AddTagPair("EndDate", "2026.07.14")
	g.AddTagPair("EndTime", "09:35:00")
	pgn := g.String()

	key := "2026/07/14/09:30:00Z-1.pgn"
	rec, plies, err := parsePGN(key, []byte(pgn))
	if err != nil {
		t.Fatalf("parsePGN: %v", err)
	}

	if rec.WhiteUID != "u_white" || rec.BlackUID != "u_black" {
		t.Errorf("uids: white=%q black=%q", rec.WhiteUID, rec.BlackUID)
	}
	if rec.VariantName != "Test" || rec.VariantGroup != "blitz" {
		t.Errorf("variant: %q/%q", rec.VariantName, rec.VariantGroup)
	}
	if rec.Outcome != result {
		t.Errorf("outcome: got %q want %q", rec.Outcome, result)
	}
	if rec.Method != int16(method) {
		t.Errorf("method: got %d want %d", rec.Method, method)
	}
	if want := shortReasonFor(method); rec.Reason != want {
		t.Errorf("reason: got %q want %q", rec.Reason, want)
	}
	if rec.PGNObjectKey != key {
		t.Errorf("pgn key: got %q want %q", rec.PGNObjectKey, key)
	}
	if _, err := uuid.Parse(rec.GameID); err != nil {
		t.Errorf("game id not a uuid: %q", rec.GameID)
	}
	if len(plies) != moveCount {
		t.Errorf("ply count: got %d want %d", len(plies), moveCount)
	}
	if !bytes.Equal(rec.Moves, expectedBlob) {
		t.Errorf("packed move blob mismatch (backfill encoding diverged from live)")
	}
	if want := time.Date(2026, 7, 14, 9, 30, 0, 0, time.Local); !rec.StartTs.Equal(want) {
		t.Errorf("start_ts: got %v want %v", rec.StartTs, want)
	}
	if want := time.Date(2026, 7, 14, 9, 35, 0, 0, time.Local); !rec.EndTs.Equal(want) {
		t.Errorf("end_ts: got %v want %v", rec.EndTs, want)
	}
}

// TestParsePGNEndTsFallback: a PGN without EndDate/EndTime falls back to start.
func TestParsePGNEndTsFallback(t *testing.T) {
	g := playToEnd(t)
	g.AddTagPair("White", "w")
	g.AddTagPair("Black", "b")
	g.AddTagPair("Result", lastField(g.String()))
	g.AddTagPair("Date", "2026.07.14")
	g.AddTagPair("Time", "09:30:00")

	rec, _, err := parsePGN("k.pgn", []byte(g.String()))
	if err != nil {
		t.Fatalf("parsePGN: %v", err)
	}
	if !rec.EndTs.Equal(rec.StartTs) {
		t.Errorf("end_ts should fall back to start_ts: start=%v end=%v", rec.StartTs, rec.EndTs)
	}
}

func lastField(s string) string {
	fields := bytes.Fields([]byte(s))
	if len(fields) == 0 {
		return ""
	}
	return string(fields[len(fields)-1])
}

// reasonTagFor mirrors the human Reason string the live seam writes for a given
// board method (see genGameOverState); "" for a non-terminal / declared game.
func reasonTagFor(m octad.Method) string {
	switch m {
	case octad.Checkmate:
		return "WHITE WINS BY CHECKMATE"
	case octad.Stalemate:
		return "DRAWN VIA STALEMATE"
	case octad.ThreefoldRepetition:
		return "DRAWN BY REPETITION"
	case octad.InsufficientMaterial:
		return "DRAWN DUE TO INSUFFICIENT MATERIAL"
	case octad.TwentyFiveMoveRule:
		return "DRAWN DUE TO 25 MOVE RULE"
	}
	return ""
}

// shortReasonFor is the live short reason code for a board method.
func shortReasonFor(m octad.Method) string {
	switch m {
	case octad.Checkmate:
		return "checkmate"
	case octad.Stalemate:
		return "stalemate"
	case octad.ThreefoldRepetition:
		return "repetition"
	case octad.InsufficientMaterial:
		return "insufficient"
	case octad.TwentyFiveMoveRule:
		return "moverule"
	}
	return ""
}
