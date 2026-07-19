package game

import (
	"testing"

	"github.com/dechristopher/octad/v2"
)

// packGameMoves encodes a game's move list exactly like db.BuildPlies does
// (2 bytes/ply big-endian PackMove values); duplicated here because db imports
// game, so this package cannot depend on it.
func packGameMoves(g *octad.Game) []byte {
	moves := g.Moves()
	blob := make([]byte, 0, len(moves)*2)
	for _, m := range moves {
		packed := PackMove(m)
		blob = append(blob, byte(packed>>8), byte(packed))
	}
	return blob
}

// assertRoundTrip replays the packed blob and asserts every derived history
// matches the original game's.
func assertRoundTrip(t *testing.T, original *octad.Game, startOFEN string) {
	t.Helper()

	replayed, err := ReplayArchive(startOFEN, packGameMoves(original))
	if err != nil {
		t.Fatalf("ReplayArchive: %v", err)
	}

	og := OctadGame{Game: *original}
	rg := OctadGame{Game: *replayed}

	for name, pair := range map[string][2][]string{
		"UOI":  {og.MoveHistory(), rg.MoveHistory()},
		"SAN":  {og.SANHistory(), rg.SANHistory()},
		"OFEN": {og.OFENHistory(), rg.OFENHistory()},
	} {
		want, got := pair[0], pair[1]
		if len(want) != len(got) {
			t.Fatalf("%s history length: want %d got %d", name, len(want), len(got))
		}
		for i := range want {
			if want[i] != got[i] {
				t.Errorf("%s history mismatch at %d: want %q got %q",
					name, i, want[i], got[i])
			}
		}
	}
}

// TestReplayArchiveRoundTrip replays a game from the standard starting
// position and requires byte-identical UOI/SAN/OFEN histories.
func TestReplayArchiveRoundTrip(t *testing.T) {
	g, err := octad.NewGame()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		vm := g.ValidMoves()
		if len(vm) == 0 {
			break
		}
		if err := g.Move(vm[0]); err != nil {
			t.Fatal(err)
		}
	}
	if len(g.Moves()) == 0 {
		t.Fatal("no moves played")
	}
	assertRoundTrip(t, g, "")
}

// TestReplayArchivePromotionAndCustomStart replays a promotion from a
// non-standard (deploy-style) starting OFEN — both the custom start and the
// promotion bits of the packed encoding must survive the round trip.
func TestReplayArchivePromotionAndCustomStart(t *testing.T) {
	const startOFEN = "3k/P3/4/K3 w - - 0 1"

	pos, err := octad.OFEN(startOFEN)
	if err != nil {
		t.Fatal(err)
	}
	g, err := octad.NewGame(pos)
	if err != nil {
		t.Fatal(err)
	}

	var promo *octad.Move
	for _, m := range g.ValidMoves() {
		if m.String() == "a3a4q" {
			promo = m
			break
		}
	}
	if promo == nil {
		t.Fatalf("promotion a3a4q not legal from %s", startOFEN)
	}
	if err := g.Move(promo); err != nil {
		t.Fatal(err)
	}

	assertRoundTrip(t, g, startOFEN)
}

// TestReplayArchiveBadInput covers the two failure modes: a truncated blob and
// a move that is illegal in its position.
func TestReplayArchiveBadInput(t *testing.T) {
	if _, err := ReplayArchive("", []byte{0x01}); err == nil {
		t.Error("odd-length blob: expected error")
	}

	// a1a1 (packed 0) is never a legal move
	if _, err := ReplayArchive("", []byte{0x00, 0x00}); err == nil {
		t.Error("illegal move: expected error")
	}
}
