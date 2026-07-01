package room

import (
	"strings"
	"testing"

	"github.com/dechristopher/octad/v2"
)

func TestDeploymentValid(t *testing.T) {
	cases := []struct {
		name string
		d    Deployment
		want bool
	}{
		{"standard", standardDeployment, true},
		{"king on a-file", Deployment{octad.King, octad.Knight, octad.Pawn, octad.Pawn}, true},
		{"two kings", Deployment{octad.King, octad.King, octad.Pawn, octad.Pawn}, false},
		{"no knight", Deployment{octad.King, octad.Pawn, octad.Pawn, octad.Pawn}, false},
		{"empty/zero", Deployment{}, false},
	}
	for _, tc := range cases {
		if tc.d.valid() != tc.want {
			t.Errorf("%s: valid() = %v, want %v", tc.name, tc.d.valid(), tc.want)
		}
	}
}

func TestParseDeployment(t *testing.T) {
	good := map[string]Deployment{
		"nkpp": standardDeployment,
		"NKPP": standardDeployment, // case-insensitive
		"knpp": {octad.King, octad.Knight, octad.Pawn, octad.Pawn},
		"ppkn": {octad.Pawn, octad.Pawn, octad.King, octad.Knight},
	}
	for s, want := range good {
		got, err := parseDeployment(s)
		if err != nil {
			t.Errorf("parseDeployment(%q) errored: %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("parseDeployment(%q) = %v, want %v", s, got, want)
		}
	}

	bad := []string{"", "nkp", "nkppp", "kkpp", "nkpx", "abcd"}
	for _, s := range bad {
		if _, err := parseDeployment(s); err == nil {
			t.Errorf("parseDeployment(%q) should have errored", s)
		}
	}
}

func TestDeploymentOrderRoundTrip(t *testing.T) {
	// order() is the inverse of parseDeployment, used to replay a player's
	// committed arrangement on reconnect; every legal order must round-trip.
	orders := []string{"nkpp", "knpp", "ppkn", "pknp", "kpnp", "npkp"}
	for _, s := range orders {
		d, err := parseDeployment(s)
		if err != nil {
			t.Fatalf("parseDeployment(%q) errored: %v", s, err)
		}
		if got := d.order(); got != s {
			t.Errorf("Deployment(%q).order() = %q, want %q", s, got, s)
		}
	}
}

func TestAssembleDeployedOFEN(t *testing.T) {
	cases := []struct {
		name         string
		white, black Deployment
		want         string
	}{
		{
			// equal standard orderings reproduce the canonical starting position
			name:  "standard reproduces start",
			white: standardDeployment,
			black: standardDeployment,
			want:  "ppkn/4/4/NKPP w NCFncf - 0 1",
		},
		{
			// king-first for both: white KNPP on rank 1; black mirrors so its
			// leftmost (king) lands on d4 -> rank 4 reads ppnk. White king a1 and
			// black king d4 are mirror squares, matching the standard b1/c4 mirror.
			name:  "king on a-file both",
			white: Deployment{octad.King, octad.Knight, octad.Pawn, octad.Pawn},
			black: Deployment{octad.King, octad.Knight, octad.Pawn, octad.Pawn},
			want:  "ppnk/4/4/KNPP w NCFncf - 0 1",
		},
	}
	for _, tc := range cases {
		got, err := assembleDeployedOFEN(tc.white, tc.black)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: ofen = %q, want %q", tc.name, got, tc.want)
		}
	}

	// invalid deployments are rejected
	if _, err := assembleDeployedOFEN(Deployment{}, standardDeployment); err == nil {
		t.Error("assembleDeployedOFEN should reject an invalid white deployment")
	}
}

func TestAssembledOFENPlaysAsGame(t *testing.T) {
	// a permuted deployment must produce a position the game model can play
	ofen, err := assembleDeployedOFEN(
		Deployment{octad.Pawn, octad.King, octad.Knight, octad.Pawn},
		Deployment{octad.Pawn, octad.Pawn, octad.Knight, octad.King},
	)
	if err != nil {
		t.Fatal(err)
	}
	fromPos, err := octad.OFEN(ofen)
	if err != nil {
		t.Fatalf("octad.OFEN(%q): %v", ofen, err)
	}
	g, err := octad.NewGame(fromPos)
	if err != nil {
		t.Fatal(err)
	}
	if g.Position().Turn() != octad.White {
		t.Errorf("deployed game should have white to move, got %v", g.Position().Turn())
	}
	if len(g.ValidMoves()) == 0 {
		t.Error("deployed position should have legal moves")
	}
}

func TestRandomDeploymentValid(t *testing.T) {
	for i := 0; i < 200; i++ {
		if d := randomDeployment(); !d.valid() {
			t.Fatalf("randomDeployment() produced invalid army: %v", d)
		}
	}
}

// TestDeploymentFromPlacementRoundTrip verifies deploymentFromPlacement is the
// exact inverse of assembleDeployedOFEN's per-side mirroring: an engine
// board-order placement, mapped to a player-perspective Deployment and then
// reassembled, must land each piece back on the board file (a..d) the engine
// placed it on — so a bot plays from the very position the engine scored.
func TestDeploymentFromPlacementRoundTrip(t *testing.T) {
	placements := [][4]octad.PieceType{
		{octad.Knight, octad.King, octad.Pawn, octad.Pawn}, // standard board order
		{octad.King, octad.Knight, octad.Pawn, octad.Pawn},
		{octad.Pawn, octad.King, octad.Knight, octad.Pawn},
		{octad.Pawn, octad.Pawn, octad.King, octad.Knight},
		{octad.Pawn, octad.Knight, octad.Pawn, octad.King},
	}

	// board-order OFEN letters for a placement on the given color's home rank
	boardLetters := func(p [4]octad.PieceType, c octad.Color) string {
		var b strings.Builder
		for i := 0; i < 4; i++ {
			b.WriteString(ofenChar(p[i], c))
		}
		return b.String()
	}

	for _, p := range placements {
		// white deploys p; its home rank is the last board segment (rank 1)
		ofen, err := assembleDeployedOFEN(deploymentFromPlacement(octad.White, p), standardDeployment)
		if err != nil {
			t.Fatalf("assemble white %v: %v", p, err)
		}
		if got, want := strings.Split(strings.Fields(ofen)[0], "/")[3], boardLetters(p, octad.White); got != want {
			t.Errorf("white placement %v -> rank1 %q, want %q (ofen %q)", p, got, want, ofen)
		}

		// black deploys p; its home rank is the first board segment (rank 4)
		ofen, err = assembleDeployedOFEN(standardDeployment, deploymentFromPlacement(octad.Black, p))
		if err != nil {
			t.Fatalf("assemble black %v: %v", p, err)
		}
		if got, want := strings.Split(strings.Fields(ofen)[0], "/")[0], boardLetters(p, octad.Black); got != want {
			t.Errorf("black placement %v -> rank4 %q, want %q (ofen %q)", p, got, want, ofen)
		}
	}
}
