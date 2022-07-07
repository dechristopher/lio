package octad

import (
	"fmt"
	"strconv"
	"strings"
)

// Decodes OFEN notation into a GameState. An error is returned
// if there is a parsing error. OFEN notation format:
// ppkn/4/4/NKPP w NCFncf - 0 1
func decodeOFEN(ofen string) (*Position, error) {
	ofen = strings.TrimSpace(ofen)
	parts := strings.Split(ofen, " ")
	if len(parts) != 6 {
		return nil, fmt.Errorf("octad: ofen invalid notiation %s must have 6 sections", ofen)
	}
	b, err := ofenBoard(parts[0])
	if err != nil {
		return nil, err
	}
	turn, ok := ofenTurnMap[parts[1]]
	if !ok {
		return nil, fmt.Errorf("octad: ofen invalid turn %s", parts[1])
	}
	rights, err := formCastleRights(parts[2])
	if err != nil {
		return nil, err
	}
	sq, err := formEnPassant(parts[3])
	if err != nil {
		return nil, err
	}
	halfMoveClock, err := strconv.Atoi(parts[4])
	if err != nil || halfMoveClock < 0 {
		return nil, fmt.Errorf("octad: ofen invalid half move clock %s", parts[4])
	}
	moveCount, err := strconv.Atoi(parts[5])
	if err != nil || moveCount < 1 {
		return nil, fmt.Errorf("octad: ofen invalid move count %s", parts[5])
	}
	return &Position{
		board:           b,
		turn:            turn,
		castleRights:    rights,
		enPassantSquare: sq,
		halfMoveClock:   halfMoveClock,
		moveCount:       moveCount,
	}, nil
}

// generates board from ofen format: ppkn/4/4/NKPP w NCFncf - 0 1
func ofenBoard(boardStr string) (*Board, error) {
	rankStrs := strings.Split(boardStr, "/")
	if len(rankStrs) != 4 {
		return nil, fmt.Errorf("octad: ofen invalid board %s", boardStr)
	}
	m := map[Square]Piece{}
	for i, rankStr := range rankStrs {
		rank := Rank(3 - i)
		fileMap, err := fenFormRank(rankStr)
		if err != nil {
			return nil, err
		}
		for file, piece := range fileMap {
			m[getSquare(file, rank)] = piece
		}
	}
	return NewBoard(m), nil
}

func fenFormRank(rankStr string) (map[File]Piece, error) {
	count := 0
	m := map[File]Piece{}
	err := fmt.Errorf("octad: ofen invalid rank %s", rankStr)
	for _, r := range rankStr {
		c := fmt.Sprintf("%c", r)
		piece := ofenPieceMap[c]
		if piece == NoPiece {
			skip, err := strconv.Atoi(c)
			if err != nil {
				return nil, err
			}
			count += skip
			continue
		}
		m[File(count)] = piece
		count++
	}
	if count != 4 {
		return nil, err
	}
	return m, nil
}

func formCastleRights(castleStr string) (CastleRights, error) {
	// check for duplicates aka. KKkq right now is valid
	for _, s := range []string{"N", "C", "F", "n", "c", "f", "-"} {
		if strings.Count(castleStr, s) > 1 {
			return "-", fmt.Errorf("octad: ofen invalid castle rights %s", castleStr)
		}
	}
	for _, r := range castleStr {
		c := fmt.Sprintf("%c", r)
		switch c {
		case "N", "C", "F", "n", "c", "f", "-":
		default:
			return "-", fmt.Errorf("octad: ofen invalid castle rights %s", castleStr)
		}
	}
	return CastleRights(castleStr), nil
}

func formEnPassant(enPassant string) (Square, error) {
	if enPassant == "-" {
		return NoSquare, nil
	}
	sq := strToSquareMap[enPassant]
	if sq == NoSquare || !(sq.Rank() == Rank3 || sq.Rank() == Rank2) {
		return NoSquare, fmt.Errorf("octad: ofen invalid En Passant square %s", enPassant)
	}
	return sq, nil
}

var (
	ofenSkipMap = map[int]string{
		1: "1",
		2: "2",
		3: "3",
		4: "4",
	}
	ofenPieceMap = map[string]Piece{
		"K": WhiteKing,
		"Q": WhiteQueen,
		"R": WhiteRook,
		"B": WhiteBishop,
		"N": WhiteKnight,
		"P": WhitePawn,
		"k": BlackKing,
		"q": BlackQueen,
		"r": BlackRook,
		"b": BlackBishop,
		"n": BlackKnight,
		"p": BlackPawn,
	}

	ofenTurnMap = map[string]Color{
		"w": White,
		"b": Black,
	}
)
