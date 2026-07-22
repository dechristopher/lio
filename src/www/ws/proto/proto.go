package proto

import (
	"encoding/json"

	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// PayloadTag defines the message payload data type
type PayloadTag string

const (
	// OFENTag is the message type tag for the OFENPayload
	OFENTag PayloadTag = "o"
	// MoveTag is the message type tag for the MovePayload
	MoveTag PayloadTag = "m"
	// MoveAckTag is the message type tag for the MoveAckPayload
	MoveAckTag PayloadTag = "a"
	// CrowdTag is the message type tag for the CrowdPayload
	CrowdTag PayloadTag = "c"
	// GameOverTag is the message type tag for the GameOverPayload
	GameOverTag PayloadTag = "g"
	// RematchUpdateTag is the message type tag for the RematchUpdatePayload
	RematchUpdateTag PayloadTag = "ru"
	// RatingUpdateTag is the message type tag for the RatingUpdatePayload, sent
	// after a rated game archives to surface each seat's rating change.
	RatingUpdateTag PayloadTag = "rc"
	// DrawOfferTag is the message type tag for the DrawOfferPayload
	DrawOfferTag PayloadTag = "do"
	// RoomTag is the message type tag for the RoomMessage
	RoomTag PayloadTag = "r"
	// RedirectTag is the message type tag for the RedirectMessage
	RedirectTag PayloadTag = "e"
	// TVTag is the message type tag for the TVPayload (home-page live games)
	TVTag PayloadTag = "tv"
	// DeployTag is the message type tag for the DeployPayload (blind deploy phase)
	DeployTag PayloadTag = "d"
	// IdentityTag is the message type tag for the IdentityPayload (socket hello)
	IdentityTag PayloadTag = "id"
	// ServerInfoTag is the message type tag for the ServerInfoPayload (build
	// version hello — drives the client's "site updated, refresh" prompt)
	ServerInfoTag PayloadTag = "si"
)

// Message represents our websocket protocol messages container
type Message struct {
	Tag          string      `json:"t"`            // message type tag
	Data         interface{} `json:"d"`            // data payload
	Version      int         `json:"v,omitempty"`  // data payload version for series
	ProtoVersion int         `json:"pv,omitempty"` // protocol version for data type
}

// IdentityPayload is the one-shot hello sent to every connection the moment it
// is tracked: the uid the socket authenticated as and whether it was seated as
// a spectator. A page rendered for a seated player whose socket lands as a
// spectator is an identity desync — iOS Safari intermittently omits cookies
// from WS upgrade requests — and every game frame that socket sends would be
// silently dropped; the client compares this echo against its own rendering
// and re-authenticates (one guarded reload) on mismatch instead of playing
// into the void.
type IdentityPayload struct {
	UID  string `json:"u"` // uid this socket authenticated as
	Spec bool   `json:"s"` // seated as spectator
}

// IdentityMessage builds the socket-identity hello frame.
func IdentityMessage(uid string, spectator bool) []byte {
	msg := Message{
		Tag:  string(IdentityTag),
		Data: IdentityPayload{UID: uid, Spec: spectator},
	}
	return msg.Please()
}

// ServerInfoPayload carries the running build's version on every socket
// connect. The page embeds the version it was rendered by (a lio-version meta
// tag); after a deploy, a reconnecting client sees the mismatch and shows a
// passive "updated — refresh" prompt instead of running stale assets forever.
type ServerInfoPayload struct {
	Version string `json:"v"` // running build version (config.VersionString)
}

// ServerInfoMessage builds the server-version hello frame.
func ServerInfoMessage(version string) []byte {
	msg := Message{
		Tag:  string(ServerInfoTag),
		Data: ServerInfoPayload{Version: version},
	}
	return msg.Please()
}

// PingMessage is used to determine socket latency to server
type PingMessage struct {
	Ping string `json:"pi"`
}

// PongMessage is the response to the PingMessage
type PongMessage struct {
	Pong string `json:"po"`
}

// OFENPayloadVersion represents the current proto version of the OFENPayload
const OFENPayloadVersion = 1

// OFENPayload contains a full board state and is sent to
// spectators after each move to update game boards
type OFENPayload struct {
	OFEN       string `json:"o"` // OFEN (position, toMove)
	LastMove   string `json:"l"` // last move played in UOI notation
	BlackClock string `json:"b"` // black clock in seconds
	WhiteClock string `json:"w"` // white clock in seconds
	GameID     string `json:"i"` // game id for routing message to board
}

type ScorePayload map[string]float64

// GameHistoryEntry describes one finished game of a room's match, keyed by the
// players' *current* seats (the ScorePayload convention): White/Black are the
// points that game earned for the players now seated white/black. Reason is
// the short method code (same values as GameOverPayload.Reason). WhitePlayed
// is the color the currently-white player actually played that game ("w"/"b")
// — sides swap between games, and the client tints timeline cells by it.
type GameHistoryEntry struct {
	White       float64 `json:"w"`
	Black       float64 `json:"b"`
	Reason      string  `json:"r,omitempty"`
	WhitePlayed string  `json:"wp,omitempty"`
}

// MatchHistoryPayload lists every finished game of a room's match in order
type MatchHistoryPayload []GameHistoryEntry

// MovePayloadVersion represents the current proto version of the MovePayload
const MovePayloadVersion = 5

// MovePayload contains all data necessary to represent a single
// move during a live game and update game ui accordingly
type MovePayload struct {
	Clock      ClockPayload `json:"c,omitempty"` // clock state
	OFEN       string       `json:"o,omitempty"` // (position, toMove)
	SAN        string       `json:"s,omitempty"`
	UOI        string       `json:"u,omitempty"`
	MoveNum    int          `json:"n,omitempty"`
	Check      bool         `json:"k,omitempty"`
	Moves      []string     `json:"m,omitempty"`  // UOI move per ply, len == plies
	SANs       []string     `json:"sm,omitempty"` // SAN per ply, len == plies (parallel to Moves)
	OFENs      []string     `json:"om,omitempty"` // OFEN per position; OFENs[0] = start, OFENs[i] = after ply i, len == plies+1
	MoveTimes  []int64      `json:"mt,omitempty"` // think time ms per ply as charged, parallel to Moves; absent when unrecorded
	ClockTimes []int64      `json:"ct,omitempty"` // remaining clock ms after each ply (post-increment), parallel to Moves
	// Evals are cached engine evals (white-positive centipawns) per ply,
	// parallel to Moves: Evals[i] scores the position after ply i+1, null where
	// the background evaluator hasn't reached it. Archive-page-only (the
	// server-rendered #archive-data blob drives the eval bar); never sent on
	// the live wire or the immutable-cacheable game JSON endpoint, whose
	// responses must not change as evals lazily fill in.
	Evals      []*int16            `json:"ev,omitempty"`
	ValidMoves map[string][]string `json:"v,omitempty"`
	Latency    clock.CTime         `json:"l,omitempty"`  // player latency indicator
	Ack        int                 `json:"a,omitempty"`  // move ack from player
	White      string              `json:"w,omitempty"`  // white player id
	Black      string              `json:"b,omitempty"`  // black player id
	Score      ScorePayload        `json:"sc,omitempty"` // match score
	History    MatchHistoryPayload `json:"h,omitempty"`  // per-game match history
	GameStart  bool                `json:"gs,omitempty"`
	// GameID identifies the game this board state belongs to. Game-boundary
	// transitions (rematch reset, deploy reveal) are announced by single-shot
	// broadcasts; a client that misses one can recognize the new game from any
	// later snapshot by the id changing, instead of dropping it via the
	// stale-board heuristics (gs flag + ply monotonicity, which break across
	// game boundaries). See arch/DEPLOY_REMATCH_RACES.md (follow-up findings).
	GameID string `json:"i,omitempty"`
}

// MessageMove contains a MovePayload message
type MessageMove struct {
	Tag          string      `json:"t"`            // message type tag
	Data         MovePayload `json:"d"`            // move data payload
	Version      int         `json:"v,omitempty"`  // data payload version for series
	ProtoVersion int         `json:"pv,omitempty"` // protocol version for data type
}

// DeployPayloadVersion represents the current proto version of the DeployPayload
const DeployPayloadVersion = 2

// DeployPayload carries blind deploy-phase data. Inbound (client to server) it
// holds the player's four-character home-rank ordering (k/n/p letters from the
// player's own left-to-right perspective). Outbound (server to client) it
// announces the deploy phase; the revealed board is sent separately as a
// MovePayload.
type DeployPayload struct {
	Order   string `json:"o,omitempty"` // inbound: 4-char order, player's perspective; outbound (reconnect): the recipient's own confirmed order
	Active  bool   `json:"a,omitempty"` // outbound: deploy phase is active
	Seconds int    `json:"s,omitempty"` // outbound: seconds allotted to deploy
	White   string `json:"w,omitempty"` // outbound: white player id (so clients know their side after a rematch swap)
	Black   string `json:"b,omitempty"` // outbound: black player id
	// Confirmed reports (on a reconnect) that the recipient already committed their
	// arrangement, so their client re-enters the locked "waiting for opponent" state.
	Confirmed bool `json:"cf,omitempty"`
	// Locked names a color that just committed its arrangement ("white"/"black");
	// it drives the opponent/spectator "locked in" indicator. Present only on the
	// per-submission lock broadcast, never on the phase-start message.
	Locked string `json:"lk,omitempty"`
	// LockedWhite / LockedBlack convey both sides' committed status on a reconnect
	// so a late-joining client's indicator reflects who is already locked in.
	LockedWhite bool `json:"lw,omitempty"`
	LockedBlack bool `json:"lb,omitempty"`
	// GameID identifies the pre-deploy game the phase supersedes (the reveal's
	// board state carries a different, fresh id). It anchors the client's
	// reveal recognition — any board state whose id differs from this is the
	// deployed game, even when the single gs=true reveal broadcast was missed —
	// and lets the client reject a stale deploy-state message delivered after
	// the reveal (its id no longer matches the game the client is showing),
	// which would otherwise wedge it back into deploy mode over a live game.
	GameID string `json:"i,omitempty"`
	// Score / History carry the standing match state (same shape and keys as
	// MovePayload's sc/h) on the phase-snapshot messages, so a client joining or
	// reconnecting during the blind deploy phase hydrates the match timeline
	// immediately instead of waiting for the deploy reveal or the next board
	// update. Room-global (not per-recipient); absent from the lock-only delta.
	Score   ScorePayload        `json:"sc,omitempty"` // match score
	History MatchHistoryPayload `json:"h,omitempty"`  // per-game match history
}

// MessageDeploy contains a DeployPayload message
type MessageDeploy struct {
	Tag          string        `json:"t"`
	Data         DeployPayload `json:"d"`
	Version      int           `json:"v,omitempty"`
	ProtoVersion int           `json:"pv,omitempty"`
}

// MoveAckPayload is the move number acknowledgement
type MoveAckPayload int

// ClockPayload is a wire representation of the current state of a game's clock
type ClockPayload struct {
	Control int64 `json:"tc"` // time control total time
	Black   int64 `json:"b"`  // black clock in centi-seconds
	White   int64 `json:"w"`  // white clock in centi-seconds
	Lag     int64 `json:"l"`  // internal server lag in ms
	// PreStart is the remaining pre-start countdown in centi-seconds: the
	// bounded first-move grace after a deploy reveal, during which white may
	// move freely before their clock starts draining on its own. Present
	// (nonzero) only while the countdown is running, so reconnecting clients
	// resync the overlay from any board snapshot. PreStartTotal is the full
	// countdown for rendering the radial progress fraction.
	PreStart      int64 `json:"ps,omitempty"`
	PreStartTotal int64 `json:"pst,omitempty"`
	// Paused reports a clock that is not running (pre-game, restored-paused,
	// or stopped on game end). It disambiguates a ply-0 snapshot with no
	// countdown: paused means white's clock awaits their first move (classic
	// games), running means the pre-start countdown already expired and white
	// is draining — a reconnecting client must tick their clock.
	Paused bool `json:"p,omitempty"`
}

// CrowdPayload contains data about connected players and spectator count.
// Spec counts connected spectators only (seated players excluded) and is
// always emitted — zero spectators is the common case, so it must not be
// omitted from the wire payload.
type CrowdPayload struct {
	Black bool `json:"b"`
	White bool `json:"w"`
	Spec  int  `json:"s"`
}

// RatingChange is one seat's Glicko-2 rating change after a rated game: the new
// display rating ("1658" / "1500?") and the signed integer delta (+8 / -8).
type RatingChange struct {
	Rating string `json:"r"`
	Delta  int    `json:"d"`
}

// RatingUpdatePayload carries both seats' rating changes, broadcast to a room
// after a rated game archives (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5). The
// client maps White/Black to clocks by the finished game's color orientation —
// the same mapping the clocks already use — so the new ratings persist into a
// rematch, and fills the game-over card's per-player delta.
type RatingUpdatePayload struct {
	White RatingChange `json:"w"`
	Black RatingChange `json:"b"`
}

// GameOverPayload contains data regarding the outcome of the game
type GameOverPayload struct {
	Winner   string              `json:"w,omitempty"`
	StatusID int                 `json:"i,omitempty"`
	Status   string              `json:"s"`
	Reason   string              `json:"r,omitempty"` // short method code for the UI (checkmate, time, resignation, stalemate, agreement, repetition, moverule, abandoned)
	Clock    ClockPayload        `json:"c,omitempty"`
	Score    ScorePayload        `json:"sc,omitempty"`
	History  MatchHistoryPayload `json:"h,omitempty"` // per-game match history
	RoomOver bool                `json:"o,omitempty"`
	// RematchWindow, when > 0, is the number of seconds the human-vs-human
	// rematch window stays open before the room closes; drives the client
	// countdown. No new game is guaranteed — the room simply closes if both
	// players don't agree a rematch in time. Bot games are neither auto-rematched
	// nor time-boxed (the finished room stays open for review + manual rematch),
	// so they never carry this.
	RematchWindow int `json:"rw,omitempty"`
	// RematchWhite / RematchBlack report which seats' rematch agreements the
	// server has recorded so far. The initial game-over broadcast carries both
	// false; the repeats a waiting client's resync poll receives (and the
	// reconnect game-over state) carry live truth, letting the client reconcile
	// a rematch click that never arrived — resending it — and restore or surface
	// pending/opponent-wants state after a reload. This is the rematch analogue
	// of DeployPayload.Confirmed (see arch/DEPLOY_REMATCH_RACES.md, F4).
	RematchWhite bool `json:"rqw,omitempty"`
	RematchBlack bool `json:"rqb,omitempty"`
	// RaceTo is the match target for a race-to room (Params.RaceTo): the points
	// (draws count ½) a player needs, with a strict lead, to win the match.
	// Present on every game-over payload of a match room; zero otherwise.
	RaceTo int `json:"rt,omitempty"`
	// MatchOver reports the race is decided (a player reached RaceTo points with
	// a lead — the winner is the seat with the higher Score). The room then holds
	// the usual rematch window, in which agreement starts a fresh match.
	MatchOver bool `json:"mo,omitempty"`
	// NextGameIn, when > 0, is the number of seconds until the next game of an
	// undecided match auto-starts (the mid-match interlude). Mutually exclusive
	// with RematchWindow: mid-match game-overs carry no rematch affordance.
	NextGameIn int `json:"ng,omitempty"`
}

// RematchUpdatePayload retimes the human rematch-window countdown mid-window
// without re-rendering the whole result overlay — e.g. when the opponent
// disconnects and the window is shortened, or reconnects and it is restored.
type RematchUpdatePayload struct {
	// Seconds remaining in the (possibly shortened) rematch window.
	Seconds int `json:"s"`
	// OpponentLeft reports that the opponent disconnected, so a rematch is no
	// longer possible; the client reflects this and disables its rematch action.
	OpponentLeft bool `json:"ol,omitempty"`
	// Requested carries the id of a player who just asked for a rematch (before
	// both sides have agreed), so the opponent's client can surface an "opponent
	// wants a rematch" indicator. When set, this message is purely that signal and
	// does not retime the countdown (Seconds is omitted).
	Requested string `json:"rq,omitempty"`
}

// DrawOfferPayload signals draw-offer state to clients during a live game. A
// standing offer names the offering player (By) so the opponent's client can
// surface an "accept draw" affordance and the offerer's client can show a
// pending state; each client compares By to its own uid to pick the view.
// Declined reports that a standing offer was refused or withdrawn — the bot's
// engine evaluation declined it, or a move superseded it — so clients clear the
// affordance. Exactly one of By / Declined is meaningful per message.
type DrawOfferPayload struct {
	By       string `json:"by,omitempty"` // uid of the player who offered a draw
	Declined bool   `json:"dc,omitempty"` // a standing offer was declined/withdrawn
}

// RoomMessage contains room state data
type RoomMessage struct {
	RoomID  string `json:"id,omitempty"`
	Query   bool   `json:"q,omitempty"`
	Ready   bool   `json:"r,omitempty"`
	P1Score int    `json:"p1,omitempty"`
	P2Score int    `json:"p2,omitempty"`
}

// RedirectMessage instructs the client to redirect to a different page
// optionally displaying an intermediate message in a modal
type RedirectMessage struct {
	Message  string `json:"m,omitempty"`
	Location string `json:"l"`
}

// Marshal encodes the given message and payload into JSON
func (m *Message) Marshal() ([]byte, error) {
	return json.Marshal(&m)
}

// Please will return the marshaled text as a byte[], hoping it doesn't fail
func (m *Message) Please() []byte {
	b, err := m.Marshal()
	if err != nil {
		util.Error(str.CProt, str.EProtoMarshal, err)
		// we've got problems if these messages fail to marshal
		panic(err)
	}

	return b
}
