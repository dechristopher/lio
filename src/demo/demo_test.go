package demo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dechristopher/octad/v2"
)

// TestBatchWellFormed checks that generated games are structurally sound: the
// requested count, a legal start position, a valid winner, and plies that parse
// as OFENs whose move field alternates from white.
func TestBatchWellFormed(t *testing.T) {
	const n = 25
	games := Batch(n)
	if len(games) != n {
		t.Fatalf("Batch(%d) returned %d games", n, len(games))
	}

	for i, g := range games {
		if _, err := octad.OFEN(g.Start); err != nil {
			t.Fatalf("game %d: start OFEN %q does not parse: %v", i, g.Start, err)
		}
		if !strings.HasSuffix(g.Start, " w NCFncf - 0 1") {
			t.Fatalf("game %d: unexpected start OFEN %q", i, g.Start)
		}
		switch g.Winner {
		case "w", "b", "d":
		default:
			t.Fatalf("game %d: invalid winner %q", i, g.Winner)
		}
		if len(g.Plies) == 0 {
			t.Fatalf("game %d: no plies played", i)
		}

		for j, p := range g.Plies {
			if len(p.U) < 4 {
				t.Fatalf("game %d ply %d: short UOI %q", i, j, p.U)
			}
			if _, err := octad.OFEN(p.O); err != nil {
				t.Fatalf("game %d ply %d: OFEN %q does not parse: %v", i, j, p.O, err)
			}
			// side to move after ply j alternates: white moved on even j, so it
			// is black's turn after (and vice versa).
			wantTurn := "b"
			if j%2 == 1 {
				wantTurn = "w"
			}
			if fields := strings.Fields(p.O); len(fields) < 2 || fields[1] != wantTurn {
				t.Fatalf("game %d ply %d: OFEN %q turn field != %q", i, j, p.O, wantTurn)
			}
		}
	}
}

// TestGameTerminates ensures the play loop always resolves (natural outcome or
// the defensive ply cap) rather than running away.
func TestGameTerminates(t *testing.T) {
	for i := 0; i < 200; i++ {
		g := generateGame()
		if len(g.Plies) > maxPlies {
			t.Fatalf("game exceeded ply cap: %d > %d", len(g.Plies), maxPlies)
		}
	}
}

// TestBatchJSON confirms the batch marshals to JSON (the handler returns it
// directly to the client animator).
func TestBatchJSON(t *testing.T) {
	b, err := json.Marshal(Batch(3))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"start"`) || !strings.Contains(string(b), `"plies"`) {
		t.Fatalf("unexpected JSON shape: %s", b)
	}
}
