package opening

import "testing"

// TestFormationsComplete verifies all 12 formation keys are distinct legal
// armies (one king, one knight, two pawns) and each has a name.
func TestFormationsComplete(t *testing.T) {
	seen := make(map[string]bool)
	for _, k := range formationKeys {
		if len(k) != 4 {
			t.Fatalf("formation key %q is not 4 chars", k)
		}
		var kings, knights, pawns int
		for _, c := range k {
			switch c {
			case 'k':
				kings++
			case 'n':
				knights++
			case 'p':
				pawns++
			default:
				t.Fatalf("formation key %q has illegal piece %q", k, string(c))
			}
		}
		if kings != 1 || knights != 1 || pawns != 2 {
			t.Fatalf("formation key %q is not one king/one knight/two pawns", k)
		}
		if seen[k] {
			t.Fatalf("duplicate formation key %q", k)
		}
		seen[k] = true
		if formationNames[k] == "" {
			t.Fatalf("formation %q has no name", k)
		}
	}
	if len(seen) != 12 {
		t.Fatalf("expected 12 distinct formations, got %d", len(seen))
	}
}

// TestMatchupsUniqueAndPopulated verifies every one of the 144 matchup cells is
// non-empty and globally unique — the guarantee behind "144 distinct names".
func TestMatchupsUniqueAndPopulated(t *testing.T) {
	seen := make(map[string]string)
	for i := 0; i < 12; i++ {
		for j := 0; j < 12; j++ {
			name := matchups[i][j]
			if name == "" {
				t.Errorf("matchup [%s x %s] is empty", formationKeys[i], formationKeys[j])
				continue
			}
			if prev, dup := seen[name]; dup {
				t.Errorf("duplicate matchup name %q at [%s x %s] and %s",
					name, formationKeys[i], formationKeys[j], prev)
			}
			seen[name] = formationKeys[i] + "x" + formationKeys[j]
		}
	}
	if len(seen) != 144 {
		t.Errorf("expected 144 distinct matchup names, got %d", len(seen))
	}
}

// TestNamesStandardStart verifies the classic default start resolves to the
// Standard mirror. Both home ranks are the standard ordering; from each side's
// own perspective that is `nkpp`, so the matchup is the symmetric Standing Wave.
func TestNamesStandardStart(t *testing.T) {
	white, black, matchup, ok := Names("ppkn/4/4/NKPP w NCFncf - 0 1")
	if !ok {
		t.Fatal("standard start did not resolve")
	}
	if white != "The Standard" || black != "The Standard" {
		t.Errorf("standard start formations = %q / %q, want The Standard / The Standard", white, black)
	}
	if matchup != "Standing Wave" {
		t.Errorf("standard start matchup = %q, want Standing Wave", matchup)
	}
}

// TestNamesDeployPerspectives verifies Black's key is derived from its home
// rank reversed (its own left-to-right perspective). White plays PNKP on the
// board (own key pnkp = The Citadel); Black's rank is knpp on the board, which
// reversed is ppnk = The Bastion.
func TestNamesDeployPerspectives(t *testing.T) {
	white, black, matchup, ok := Names("knpp/4/4/PNKP w NCFncf - 0 1")
	if !ok {
		t.Fatal("deploy start did not resolve")
	}
	if white != "The Citadel" {
		t.Errorf("white formation = %q, want The Citadel", white)
	}
	if black != "The Bastion" {
		t.Errorf("black formation = %q, want The Bastion", black)
	}
	// pnkp (7) x ppnk (3)
	if want := matchups[7][3]; matchup != want {
		t.Errorf("matchup = %q, want %q", matchup, want)
	}
}

// TestNamesInvalid verifies a non-starting position (home ranks not full) does
// not resolve.
func TestNamesInvalid(t *testing.T) {
	for _, ofen := range []string{
		"",
		"ppkn/4/4/NKPP",                // no side-to-move field
		"pk1n/4/4/NKPP w NCFncf - 0 1", // black home rank has a gap
		"4/4/4/4 w - - 0 1",            // empty board
	} {
		if _, _, _, ok := Names(ofen); ok {
			t.Errorf("Names(%q) resolved, want not ok", ofen)
		}
	}
}
