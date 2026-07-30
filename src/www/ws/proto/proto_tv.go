package proto

// TVSeat is one side's identity in a featured game, as the grid renders it:
// who is sitting there, plus the badges that qualify the name. The TV stream is
// viewer-less — every viewer gets the same card — so Name is always a name a
// stranger can read ("Anonymous", a persona, or an account), never the room
// view's viewer-relative "You".
type TVSeat struct {
	Name      string `json:"n,omitempty"`   // account username / bot persona name / "Anonymous"
	Title     string `json:"t,omitempty"`   // account title badge code ("GM"); "" = untitled
	TitleName string `json:"tn,omitempty"`  // that badge's tooltip ("Grandmaster")
	Bot       bool   `json:"bot,omitempty"` // the engine holds this seat
	Glyph     string `json:"g,omitempty"`   // bot seat's difficulty-persona piece glyph ("♛︎")
	Locked    bool   `json:"lk,omitempty"`  // committed its arrangement (Deploying only)
}

// TVGame is the full display state of a single featured game in the home-page
// TV grid. It is sent both in the initial snapshot (one per featured game) and
// as an add/move delta. Clocks are centi-seconds, matching ClockPayload, so the
// client can drive the thin progress bars off Control as the denominator.
type TVGame struct {
	RoomID    string       `json:"r"`            // slot key + watch-link target
	GameID    string       `json:"i"`            // changes on rematch → client resets that board
	Variant   string       `json:"vn"`           // variant display name (the time control, e.g. "½ + 1")
	Deploy    bool         `json:"dp,omitempty"` // blind-deploy game mode (false = classic)
	Watchers  int          `json:"sp,omitempty"` // connected spectators (seated players excluded)
	VsBot     bool         `json:"vb,omitempty"` // human-vs-computer game
	Orient    string       `json:"or,omitempty"` // color anchored to the board's bottom: "w"/"b"
	OFEN      string       `json:"o"`            // position + side to move
	LastMove  string       `json:"l,omitempty"`  // UOI, for last-move highlight
	Control   int64        `json:"tc"`           // total time control centis (bar denominator)
	White     int64        `json:"w"`            // white clock centis
	Black     int64        `json:"b"`            // black clock centis
	WhiteSeat TVSeat       `json:"ws"`           // who is playing white
	BlackSeat TVSeat       `json:"bs"`           // who is playing black
	Casual    bool         `json:"ca,omitempty"` // untimed game: render clocks as a static ∞
	Score     ScorePayload `json:"sc,omitempty"` // match score, keyed "w"/"b"
	RaceTo    int          `json:"rt,omitempty"` // race-to match target; 0 = single game
	Over      bool         `json:"x,omitempty"`  // final position (freeze/dim the board)

	// Winner / Reason describe how the game on the board finished: "w"/"b"/"d"
	// and the same short method code the room's game-over payload carries
	// (checkmate, time, resignation, …). They are set only on the terminal
	// state and ride the hub's stored game through the whole between-games
	// interlude, so a viewer who arrives after the finish still gets the
	// result overlay rather than an unexplained frozen board. The score alone
	// cannot stand in for them: it says who gained a point, not who lost, and
	// carries no delta at all for a viewer who did not watch the game end.
	Winner string `json:"wr,omitempty"`
	Reason string `json:"rs,omitempty"`

	// Running reports that a side's clock is being charged *right now* — the one
	// question the grid's local interpolator needs answered. It is false through
	// every pre-game state (the blind deploy phase and the post-reveal first-move
	// grace included), because in those the times are static on the server: a
	// client that ticked them would run visibly ahead until the next real update
	// snapped them back, which is exactly what the grid used to do during the
	// pre-start countdown.
	Running bool `json:"rn,omitempty"`

	// Deploying marks the room as mid blind-deploy: the arrangements are secret,
	// so the grid covers both home ranks and reads each seat's Locked flag
	// instead of its clock.
	Deploying bool `json:"dg,omitempty"`
	// PhaseLeft / PhaseTotal drive the pre-game dial on the board: centi-seconds
	// remaining and the full span it counts down from. One pair covers both
	// pre-game timers in turn — the deploy phase's auto-fill deadline while
	// Deploying, then the post-reveal grace before White's clock starts — since
	// only one is ever live and the grid renders them identically. Both are zero
	// once play is under way.
	PhaseLeft  int64 `json:"pl,omitempty"`
	PhaseTotal int64 `json:"pt,omitempty"`
}

// TVCrowd is a count-only delta: the spectator count of a featured room
// changed between moves. It deliberately carries no game state so applying it
// never disturbs the client's board or clock-tick baseline.
type TVCrowd struct {
	RoomID   string `json:"r"`
	Watchers int    `json:"n"`
}

// TVPayload is the union message streamed over the /socket/tv channel. Exactly
// one of Snapshot / Add / Move / Crowd / Remove is populated per message; the
// client dispatches on whichever field is present:
//   - Snapshot: the full featured set, sent once when a viewer connects.
//   - Add:      a game entered a (newly free or newly filled) grid slot.
//   - Move:     a featured game advanced or ended (Over set on the final state).
//   - Crowd:    a featured room's spectator count changed (count-only patch).
//   - Remove:   the room id whose slot was freed (its game ended without rematch).
type TVPayload struct {
	Snapshot []TVGame `json:"s,omitempty"`
	Add      *TVGame  `json:"a,omitempty"`
	Move     *TVGame  `json:"m,omitempty"`
	Crowd    *TVCrowd `json:"w,omitempty"`
	Remove   string   `json:"d,omitempty"`
}

// Marshal fully JSON marshals the TVPayload and wraps it in a Message struct.
func (t *TVPayload) Marshal() []byte {
	message := Message{
		Tag:  string(TVTag),
		Data: t,
	}

	return message.Please()
}
