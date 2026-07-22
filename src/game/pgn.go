package game

import (
	"fmt"
	"strings"
	"time"

	"github.com/dechristopher/octad/v2"
)

// PGNMeta carries the tag-pair inputs for BuildPGN. It is decoupled from the
// room and db packages so both the live archival path (room, from the finished
// game) and the archive-page rebuild (www/handlers, from the DB row) can
// assemble byte-identical PGN from their own sources — the guarantee behind the
// copy button copying exactly what was archived.
//
// Every field must be reconstructable from the durable games row, or the two
// paths would diverge: Reason is therefore the DB-canonical short method token
// (games.reason), not a display sentence; White/Black are the archived display
// names ("BOT"/"Anonymous"/<username>), with the raw session uids in
// WhiteUID/BlackUID.
type PGNMeta struct {
	Event, Site        string
	Variant, Group     string
	White, Black       string
	WhiteUID, BlackUID string
	Result             string // octad outcome token ("1-0", "0-1", "1/2-1/2", "*")
	Reason             string // DB-canonical method token ("checkmate", "time", ...)
	Start, End         time.Time
	StartOFEN          string // starting position; a non-standard one becomes SetUp/FEN
	// Opening names derived from StartOFEN (see the opening package). When
	// WhiteFormation is empty the three name tags are omitted (an unresolvable
	// start), so BuildPGN never fabricates a name.
	WhiteFormation, BlackFormation, Matchup string
}

// PGNSeatName formats a seat's PGN display name, space-separated (no brackets):
//   - bot:              "BOT <glyph> <persona>"  (e.g. "BOT ♟︎ Pawn")
//   - titled account:   "<title> <username>"     (e.g. "OG drewtest")
//   - untitled account: "<username>"
//   - anonymous human:  "Anonymous"
//
// The space-separated title prefix follows the standard PGN convention (Lichess
// writes [White "GM DrNykterstein"]). Brackets/parens/braces are deliberately
// avoided: they are legal inside a PGN quoted string, but octad's own decoder
// strips those sections when recovering movetext, so a bracketed value breaks
// re-import (the --backfill path). The bot glyph/persona are resolved by the
// caller (from engine.PersonaByKey) and passed in so this stays engine-free.
// Both the live archival path and the archive-page rebuild call it with
// equivalent inputs, so the White/Black tags are byte-identical across them.
func PGNSeatName(username, title, botGlyph, botPersona string, isBot bool) string {
	if isBot {
		if botGlyph != "" {
			return "BOT " + botGlyph + " " + botPersona
		}
		return "BOT " + botPersona
	}
	if username == "" {
		return "Anonymous"
	}
	if title != "" {
		return title + " " + username
	}
	return username
}

// BuildPGN assembles the full archival PGN for a finished game: the tag-pair
// roster followed by numbered SAN movetext (with a { [%clk h:mm:ss.cc] } comment
// per move when per-ply timing was recorded) ending in the result token.
//
// It composes the movetext directly rather than via octad's Game.String so the
// %clk comments can be carried — octad's encoder has no comment support, and its
// decoder strips comments, so the PGN replays/backfills identically. times is
// the per-ply timing parallel to the move list; a nil or length-desynced slice
// yields plain movetext.
func BuildPGN(m PGNMeta, g *octad.Game, times []MoveTime) string {
	var tags [][2]string
	add := func(k, v string) { tags = append(tags, [2]string{k, v}) }

	// times are formatted in UTC so the string is a pure function of the instant,
	// independent of the source time.Time's location — the live game (server-local
	// g.Start) and the archive-page rebuild (a pgx timestamptz) then produce
	// byte-identical tags for the same game.
	start, end := m.Start.UTC(), m.End.UTC()

	add("Event", m.Event)
	add("Site", m.Site)
	add("Date", start.Format("2006.01.02"))
	add("Variant", m.Variant)
	add("Group", m.Group)
	// opening names sit beside the variant so tools and humans see the named
	// deploy/matchup; custom keys octad's PGN decoder safely ignores on import
	if m.WhiteFormation != "" {
		add("WhiteFormation", m.WhiteFormation)
		add("BlackFormation", m.BlackFormation)
		add("Matchup", m.Matchup)
	}
	// display names in the standard White/Black tags; raw session uids to the
	// dedicated *UID tags so the PGN reads human while identity survives
	add("White", m.White)
	add("Black", m.Black)
	add("WhiteUID", m.WhiteUID)
	add("BlackUID", m.BlackUID)
	add("Result", m.Result)
	add("Reason", m.Reason)
	add("Time", start.Format("15:04:05"))
	add("EndDate", end.Format("2006.01.02"))
	add("EndTime", end.Format("15:04:05"))

	// record a non-standard starting position (a deploy-mode game) as a SetUp/FEN
	// tag pair so the movetext replays from the correct initial OFEN. The tag key
	// must be the PGN-standard "FEN": that's the only key octad's own decoder
	// reads to seed a custom start.
	if start, err := octad.StartingPosition(); err == nil {
		if m.StartOFEN != "" && m.StartOFEN != start.String() {
			add("SetUp", "1")
			add("FEN", m.StartOFEN)
		}
	}

	var sb strings.Builder
	for _, t := range tags {
		fmt.Fprintf(&sb, "[%s \"%s\"]\n", t[0], t[1])
	}
	sb.WriteByte('\n')
	sb.WriteString(movetext(g, times))
	sb.WriteString(" " + m.Result)
	return sb.String()
}

// movetext renders the numbered SAN movetext, with a { [%clk h:mm:ss.cc] }
// comment after each move (the mover's remaining clock, Lichess-style) when
// per-ply timing was recorded (times parallel to the move list; an untimed or
// desynced record emits plain movetext).
func movetext(g *octad.Game, times []MoveTime) string {
	positions := g.Positions()
	moves := g.Moves()
	timed := len(times) == len(moves)
	var sb strings.Builder
	for i, m := range moves {
		if i > 0 {
			sb.WriteByte(' ')
		}
		if i%2 == 0 {
			fmt.Fprintf(&sb, "%d. ", i/2+1)
		}
		sb.WriteString(octad.AlgebraicNotation{}.Encode(positions[i], m))
		if timed {
			fmt.Fprintf(&sb, " { [%%clk %s] }", formatClk(times[i].ClockMs))
		}
	}
	return sb.String()
}

// formatClk renders a remaining-clock milliseconds value in the PGN %clk
// h:mm:ss.cc form (octad games are short enough that centis matter).
func formatClk(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%d:%02d:%02d.%02d",
		ms/3600000, ms%3600000/60000, ms%60000/1000, ms%1000/10)
}
