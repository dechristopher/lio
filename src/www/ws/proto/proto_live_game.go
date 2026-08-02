package proto

// The live-game frame (arch/ONE_GAME_AT_A_TIME.md): the reconnect bar's way of
// following a game that starts or ends while the player is looking at some other
// page.
//
// It is addressed to a *session* and, folded in, to an account — never to a
// channel. Every other payload here is the same for everyone on its connection;
// this one describes one person's own game, and the anonymous half of the site
// has no account to address, so it rides the uid.
//
// The frame is the whole state, not a delta. A page renders the bar server-side
// on load and then only ever replaces it wholesale, so a client that misses one
// frame is corrected by the next rather than drifting.

// LiveGamePayload is the reconnect bar's contents, or its absence.
//
// An empty RoomID clears the bar. That is the "your game just ended" frame, and
// it is sent rather than inferred: a bar left standing after a game finished
// would offer a trip to a room that is being torn down.
type LiveGamePayload struct {
	// RoomID is where the bar's control goes. Empty means there is no game and
	// the bar hides.
	RoomID string `json:"id,omitempty"`
	// Label describes the game in one line ("½ + 1 blitz vs Queen").
	Label string `json:"l,omitempty"`
	// Own reports that the session receiving this frame holds the seat itself,
	// rather than another session of the same account. Seats are keyed by uid,
	// so the other case can only watch — the bar renders a different control for
	// each, and the client must not have to guess which it is.
	Own bool `json:"o,omitempty"`
}

// LiveGameMessage builds the frame describing the game a session is committed
// to. A zero-value payload is the clear.
func LiveGameMessage(p LiveGamePayload) []byte {
	msg := Message{
		Tag:  string(LiveGameTag),
		Data: p,
	}
	return msg.Please()
}
