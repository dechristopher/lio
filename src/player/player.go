package player

import "github.com/dechristopher/octad/v2"

// GameResult records one finished game of a room's match from this player's
// perspective: the points they earned (1, 0.5, or 0), the color they played
// that game, and the short method code describing how the game ended (the
// same codes as GameOverPayload.Reason: checkmate, time, resignation, ...).
// Like the score counters, results travel with the *Player through
// Players.FlipColor, so the history survives the color swaps between games.
type GameResult struct {
	Points float64
	Color  octad.Color
	Reason string
}

// Player struct for keeping track of info, status, state, and score.
// Spectators are never Players: they hold no seat, are tagged at the socket
// layer (channel.SocketContext.IsSpectator), and are flagged to the view via
// RoomTemplatePayload.IsSpectator.
type Player struct {
	ID    string
	IsBot bool
	// UserID / Username carry the seat's account identity when the player is
	// logged in (arch/ACCOUNTS_AUTH_RATINGS.md Phase 2): UserID is the users
	// row id stamped onto the archived game, Username the display name shown
	// on clocks/timeline/pregame/OG. Both are zero-valued for an anonymous
	// human (the view renders "You"/"Anonymous") and for a bot (rendered
	// "BOT"). RatingDisplay is a Phase-5 slot, captured at seat-claim so page
	// renders never read the DB (currently always empty).
	UserID        *int64
	Username      string
	RatingDisplay string
	// Title is the seat's optional account display title ("" for anon/bot),
	// rendered to the left of the name in the theme accent color. Captured at
	// seat-claim like Username so page renders never read the DB.
	Title       string
	scorePoints int
	scoreHalf   int
	results     []GameResult
	// sendLatency bool // TODO send server latency stats if enabled
}

// Identity is the seat identity a handler hands to the room when a player
// creates or joins: the session uid plus the account fields (nil/empty for an
// anonymous seat). It decouples the room package from the request/session
// layer — handlers build it from user.GetContext.
type Identity struct {
	UID      string
	UserID   *int64
	Username string
	Title    string
}

// DisplayName returns the seat's account username, or "" for an anonymous
// human. Bots are not distinguished here (callers check IsBot) — this is the
// raw name the view resolves into "You"/"Anonymous"/"BOT" by context.
func (p *Player) DisplayName() string {
	return p.Username
}

// Results returns the player's per-game match history in game order
func (p *Player) Results() []GameResult {
	return p.results
}

// resetScore clears the player's accumulated score and per-game history, for
// when a decided race-to match restarts as a fresh match in the same room.
func (p *Player) resetScore() {
	p.scorePoints = 0
	p.scoreHalf = 0
	p.results = nil
}

// Snapshot is the serializable form of a Player for room persistence: seat
// identity (including the account fields as of Phase 2) plus the accumulated
// match score and per-game history, which are otherwise unexported and would
// not survive a JSON round-trip.
type Snapshot struct {
	ID            string       `json:"id"`
	IsBot         bool         `json:"bot,omitempty"`
	UserID        *int64       `json:"uid,omitempty"`
	Username      string       `json:"un,omitempty"`
	Title         string       `json:"tt,omitempty"`
	RatingDisplay string       `json:"rd,omitempty"`
	ScorePoints   int          `json:"sp,omitempty"`
	ScoreHalf     int          `json:"sh,omitempty"`
	Results       []GameResult `json:"res,omitempty"`
}

// Snapshot captures the player's persistable state. Title is an additive
// omitempty field: an older snapshot without it simply restores an empty title
// (a purely cosmetic gap for the rest of that one restored game — the title
// never affects game logic or archival), so it needs no persistVersion bump.
func (p *Player) Snapshot() Snapshot {
	return Snapshot{
		ID:            p.ID,
		IsBot:         p.IsBot,
		UserID:        p.UserID,
		Username:      p.Username,
		Title:         p.Title,
		RatingDisplay: p.RatingDisplay,
		ScorePoints:   p.scorePoints,
		ScoreHalf:     p.scoreHalf,
		Results:       p.results,
	}
}

// RestorePlayer rebuilds a Player from a persisted snapshot.
func RestorePlayer(s Snapshot) *Player {
	return &Player{
		ID:            s.ID,
		IsBot:         s.IsBot,
		UserID:        s.UserID,
		Username:      s.Username,
		Title:         s.Title,
		RatingDisplay: s.RatingDisplay,
		scorePoints:   s.ScorePoints,
		scoreHalf:     s.ScoreHalf,
		results:       s.Results,
	}
}

// ToJoin is a sample Player used to configure a room in which
// the opponent joins via URL and is then configured
var ToJoin = Player{
	ID:    "",
	IsBot: false,
}

// Score returns the player's match score
func (p *Player) Score() float64 {
	score := 0.0

	score += float64(p.scorePoints)
	score += 0.5 * float64(p.scoreHalf)

	return score
}
