package proto

// TVGame is the full display state of a single featured game in the home-page
// TV grid. It is sent both in the initial snapshot (one per featured game) and
// as an add/move delta. Clocks are centi-seconds, matching ClockPayload, so the
// client can drive the thin progress bars off Control as the denominator.
type TVGame struct {
	RoomID   string       `json:"r"`            // slot key + watch-link target
	GameID   string       `json:"i"`            // changes on rematch → client resets that board
	Variant  string       `json:"vn"`           // variant display name
	VsBot    bool         `json:"vb,omitempty"` // human-vs-computer game
	BotColor string       `json:"bc,omitempty"` // side the bot plays: "w"/"b" ("" = no bot)
	Orient   string       `json:"or,omitempty"` // color anchored to the board's bottom: "w"/"b"
	OFEN     string       `json:"o"`            // position + side to move
	LastMove string       `json:"l,omitempty"`  // UOI, for last-move highlight
	Control  int64        `json:"tc"`           // total time control centis (bar denominator)
	White    int64        `json:"w"`            // white clock centis
	Black    int64        `json:"b"`            // black clock centis
	Score    ScorePayload `json:"sc,omitempty"` // match score, keyed "w"/"b"
	Running  bool         `json:"rn,omitempty"` // clock is live (a move has started it); false pre-first-move
	Over     bool         `json:"x,omitempty"`  // final position (freeze/dim the board)
}

// TVPayload is the union message streamed over the /socket/tv channel. Exactly
// one of Snapshot / Add / Move / Remove is populated per message; the client
// dispatches on whichever field is present:
//   - Snapshot: the full featured set, sent once when a viewer connects.
//   - Add:      a game entered a (newly free or newly filled) grid slot.
//   - Move:     a featured game advanced or ended (Over set on the final state).
//   - Remove:   the room id whose slot was freed (its game ended without rematch).
type TVPayload struct {
	Snapshot []TVGame `json:"s,omitempty"`
	Add      *TVGame  `json:"a,omitempty"`
	Move     *TVGame  `json:"m,omitempty"`
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
