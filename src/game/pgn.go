package game

import (
	"fmt"
	"strings"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/pools"
	"github.com/dechristopher/lio/variant"
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
//
// There is deliberately no Event field: the Event tag is *derived* from the
// situation fields below (see EventName), so the two paths cannot drift on it.
type PGNMeta struct {
	Site               string
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
	// Situation fields, all straight off the games row (games.rated,
	// games.race_to) or derived from it exactly as the archive page derives them
	// (SeatIsBot on both seats). They name the game in the Event tag, alongside
	// Group/Variant above.
	Rated  bool // affected Glicko-2 ratings
	RaceTo int  // >0 for a race-to match: the points target
	VsBot  bool // either seat is the engine
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

// SeatIsBot reports whether an archived seat is the engine: the bot plays with
// no session uid and no account, so an empty uid with a nil user id is the
// engine and nothing else. It is the single rule both PGN paths (and the
// archive page's own seat rendering) key off, so a seat can never read as the
// bot on one surface and a human on another. A human who lost their session uid
// but has an account still resolves as a human.
func SeatIsBot(uid string, userID *int64) bool {
	return uid == "" && userID == nil
}

// EventName names the situation the game was played in, for the PGN Event tag.
// It is derived rather than passed in so the live archival path and the
// archive-page rebuild cannot disagree; every input is on the games row.
// (Exported for the room page, which pre-renders the same name for the copy
// button's client-side fallback PGN — see view.pgnEventName.)
//
// The shape is "<rating> <speed> <game|match> [vs Computer]":
//
//	Rated Blitz game
//	Unrated Bullet game vs Computer
//	Unrated Casual game
//	Rated Rapid match (race to 3)
//
// "Rated"/"Unrated" is the rating stake (there is no other tag carrying it).
// The speed word is the variant's group in the site's own vocabulary, with the
// untimed unlimited group reading as the site's "Casual" mode. The blind deploy
// pre-game is deliberately unnamed: it *is* octad now, so saying so would be
// noise on every game. A future non-standard gamemode is what belongs beside
// the speed here ("Rated Atomic Blitz game").
func (m PGNMeta) EventName() string {
	var sb strings.Builder
	if m.Rated {
		sb.WriteString("Rated ")
	} else {
		sb.WriteString("Unrated ")
	}
	if speed := eventSpeed(m.Group, m.Variant); speed != "" {
		sb.WriteString(speed)
		sb.WriteByte(' ')
	}
	if m.RaceTo > 0 {
		fmt.Fprintf(&sb, "match (race to %d)", m.RaceTo)
	} else {
		sb.WriteString("game")
	}
	if m.VsBot {
		sb.WriteString(" vs Computer")
	}
	return sb.String()
}

// eventSpeed renders a variant group as the Event tag's speed word. The deploy
// group is not a speed — it collects the blind-deploy forms of the standard time
// controls, which every game now uses — so it resolves to the speed of the
// control it shares a display label with (variantName, e.g. "½ + 1" → Blitz). An
// unrecognized group (a future group reaching an old binary) degrades to the raw
// token, and an unresolvable one to no speed word at all, rather than
// mislabeling the game.
func eventSpeed(group, variantName string) string {
	switch variant.Group(group) {
	case variant.BulletGroup:
		return "Bullet"
	case variant.BlitzGroup:
		return "Blitz"
	case variant.RapidGroup:
		return "Rapid"
	case variant.HyperGroup:
		return "Hyper"
	case variant.UltiGroup:
		return "Ulti"
	case variant.UnlimitedGroup:
		// the untimed variants; "Casual" is what the site calls this mode
		return "Casual"
	case variant.DeployGroup:
		for _, c := range pools.CreateControls {
			if c.Label == variantName {
				return eventSpeed(string(c.Group), variantName)
			}
		}
		return ""
	default:
		return group
	}
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

	// a non-standard starting position (the blind deploy's arranged home rank)
	// becomes the SetUp/FEN tag pair below
	deployed := false
	if std, err := octad.StartingPosition(); err == nil {
		deployed = m.StartOFEN != "" && m.StartOFEN != std.String()
	}

	add("Event", m.EventName())
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
	if deployed {
		add("SetUp", "1")
		add("FEN", m.StartOFEN)
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
