package proto

// The watch protocol: one connection asks to follow one room's live game, and
// receives that room's state until it asks for another or the room ends
// (arch/PLAYER_CARD.md).
//
// It exists for the username hover card, which shows a live mini board of the
// game the hovered player is in. That game is almost never one of the home
// grid's featured slots, so the broadcast TV stream cannot serve it, and a
// second connection into the room's own channel would make a hover register as
// spectating — a card is a glance, not an audience.
//
// The frames are addressed to a single connection and are accepted on *every*
// channel, because the card rides whatever socket the page it sits on already
// holds (the room's, the home page's, or /socket/me). That is also why the
// state travels under a tag of its own rather than as another TVPayload field:
// the home page's grid shares the socket, and must not mistake a watched room
// for a featured one.

// WatchRequest is the inbound frame naming the room to follow. An empty RoomID
// cancels the standing watch, which is what the card sends when it closes.
//
// One watch per connection: the card shows one game at a time, so a second
// request replaces the first rather than adding to it.
type WatchRequest struct {
	RoomID string `json:"r,omitempty"`
}

// WatchPayload is the outbound frame: the watched room's current display state,
// in exactly the shape the mini board already renders (TVGame — the same struct
// the home grid streams), so both surfaces are driven by one client component.
//
// Gone reports that the room is no longer live — it closed, or it was never
// live to begin with. The card stops showing a board rather than leaving a
// frozen position that claims to be current.
type WatchPayload struct {
	RoomID string  `json:"r"`
	Game   *TVGame `json:"g,omitempty"`
	Gone   bool    `json:"x,omitempty"`
}

// Marshal fully JSON marshals the WatchPayload and wraps it in a Message struct.
func (w *WatchPayload) Marshal() []byte {
	message := Message{
		Tag:  string(WatchTag),
		Data: w,
	}

	return message.Please()
}
