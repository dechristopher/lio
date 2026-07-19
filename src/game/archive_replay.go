package game

import (
	"fmt"

	"github.com/dechristopher/octad/v2"
)

// ReplayArchive rebuilds a finished game from its archived starting OFEN and
// packed move blob (2 bytes/ply big-endian, the inverse of db.BuildPlies'
// encoding of PackMove values). It is the read path behind the permanent game
// permalinks: the returned game yields the UOI/SAN/OFEN histories the archive
// viewer renders from. Deploy-mode games replay correctly by construction —
// the walk starts from the archived (non-standard) starting position.
func ReplayArchive(startingOFEN string, packed []byte) (*octad.Game, error) {
	if len(packed)%2 != 0 {
		return nil, fmt.Errorf("replay: odd packed move blob length %d", len(packed))
	}

	g, err := genGame(startingOFEN)
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(packed); i += 2 {
		mv := int16(packed[i])<<8 | int16(packed[i+1])
		uoi := UnpackMoveUOI(mv)
		var match *octad.Move
		for _, m := range g.ValidMoves() {
			if m.String() == uoi {
				match = m
				break
			}
		}
		if match == nil {
			return nil, fmt.Errorf("replay: illegal move %q at ply %d", uoi, i/2+1)
		}
		if err := g.Move(match); err != nil {
			return nil, fmt.Errorf("replay: move %q at ply %d: %w", uoi, i/2+1, err)
		}
	}

	return g, nil
}
