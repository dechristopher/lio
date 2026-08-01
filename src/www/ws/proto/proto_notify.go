package proto

// The notification frame (arch/NOTIFICATIONS.md). It goes to one account's own
// sockets, never to a channel, so it is the one payload here that is not the
// same for every viewer on its connection.
//
// The frame has two uses and one shape:
//
//  1. On connect, every socket of a signed-in account receives the unread count
//     with no item. This is what makes the badge correct again after a deploy,
//     after a restore from the back/forward cache, and after any missed frame.
//     It is also why no page polls for notifications.
//  2. On arrival, the account's sockets receive the new count and the new
//     message, so the client renders the row without an HTTP request.
//
// The panel list is not on this wire. A person opens the panel rarely, so the
// list loads over HTTP on the first open instead of riding every connect.

// NotifyItem is one message as the client renders it. It carries the actor's
// name rather than an id: the client cannot resolve an id, and the server has
// the name in hand at the moment it writes the row.
type NotifyItem struct {
	ID    int64  `json:"id"`
	Kind  string `json:"k"`
	Body  string `json:"b"`
	Link  string `json:"l,omitempty"` // path on this site; empty = not a link
	Actor string `json:"a,omitempty"` // who caused it; empty = the site itself
	// Created is unix milliseconds. The client shows a relative time and must
	// agree with the same row after a reload, so this is the timestamp the
	// database wrote.
	Created int64 `json:"ts"`
	// Expires is unix milliseconds, and 0 for a message that does not expire.
	// The client counts down against it and stops offering the action once it
	// passes.
	//
	// It is a display bound, not the authority on whether a challenge is still
	// open. The room is, and a waiting room usually dies before this stamp is
	// reached. Accepting a dead one lands on the room-gone redirect, which is
	// the same answer a stale open challenge has always given.
	Expires int64 `json:"x,omitempty"`
	// Read marks a row the recipient has already read. It is only ever set on
	// the list the panel fetches over HTTP, which is why it is omitempty: a
	// message that just arrived is unread by definition.
	//
	// The field lives here, on the socket payload, so the list endpoint and this
	// frame have exactly one shape between them. The client then has one row
	// renderer instead of two that must be kept in step.
	Read bool `json:"r,omitempty"`
	// Follows reports that the recipient already follows the actor. It is set
	// only on a follow row, which is the one kind that offers a control about
	// the person who caused it (arch/FOLLOWING.md).
	//
	// It describes the relationship, not the button. A field meaning "offer a
	// follow-back" would have nothing to say once the reader pressed it, and the
	// control would vanish from under them; a relationship has two states, which
	// is exactly what the two-state toggle everywhere else on the site paints.
	Follows bool `json:"fw,omitempty"`
	// Choices are the answers this message demands, in the order they are shown,
	// and absent on a message that asks nothing. A row with choices renders them
	// as buttons and is not dismissible any other way: it does not clear by
	// being seen, and read-all leaves it alone.
	//
	// It is the general case of the rule a live challenge was the first instance
	// of. A challenge is not expressed through this field — it has its own kind,
	// its own expiry and two actions that are not a stored answer — but both
	// obey "a row that asks a question is not finished by being looked at".
	Choices []string `json:"c,omitempty"`
	// Response is what this reader already answered, empty while the question is
	// outstanding. The row then shows the answer instead of the buttons, which
	// is what stops somebody answering an offer twice from a second tab.
	Response string `json:"an,omitempty"`
	// Broadcast marks a row that came from the broadcasts table rather than from
	// notifications. The two have separate id sequences, so this is also half of
	// the client's row key: without it a notification and a broadcast that
	// happen to share an id would be the same row to every lookup in the panel.
	//
	// It routes the writes as well. Marking one read moves an account's
	// watermark; marking a notification read stamps one row.
	Broadcast bool `json:"bc,omitempty"`
}

// NotifyPayload carries the unread counts, and the new message when one just
// arrived.
//
// Both counts are pointers, and both are omitempty, so an absent field means
// "this frame does not carry that count" while a present one may be zero. The
// distinction is the whole point: a frame addressed to every moderator cannot
// know any one of their personal counts, and a frame telling one person their
// own count says nothing about the site's feedback backlog. A plain int64 would
// send 0 for whichever it did not know and blank a badge that should stay lit —
// and, worse, an omitempty int64 would drop a genuine 0 and leave a badge lit
// after the last message was read.
type NotifyPayload struct {
	// Unread is this account's own unread notifications after the event. The
	// client sets the badge to this number; it never counts the rows itself. A
	// client that added one for each frame would drift after any dropped or
	// repeated frame, and the badge must be correct on a page that nobody
	// reloads for hours.
	Unread *int64 `json:"n,omitempty"`
	// Staff is unread player feedback, and it is only ever sent to a moderator.
	// It is not a notification and has no row: feedback read state is site-wide
	// on purpose (see migration 00020), so it is derived on each send rather
	// than stored per person. The client adds it into the same badge, because a
	// moderator has one bell and does not care which of the two a mark is for.
	Staff *int64 `json:"s,omitempty"`
	// Item is the message that just arrived, or nil for a connect frame.
	Item *NotifyItem `json:"i,omitempty"`
}

// NotifyCountMessage builds the connect frame: both halves of one account's
// badge. staff is 0 for everyone who cannot moderate.
func NotifyCountMessage(unread, staff int64) []byte {
	msg := Message{
		Tag:  string(NotifyTag),
		Data: NotifyPayload{Unread: &unread, Staff: &staff},
	}
	return msg.Please()
}

// NotifyStaffMessage builds the feedback-backlog frame sent to every moderator
// at once: new feedback arrived, or somebody read some.
//
// It carries no personal count. One frame goes to every moderator, and their
// own unread notifications differ from each other — sending any number for that
// would overwrite most of them with somebody else's. The omitted field leaves
// each client's own count exactly as it was.
func NotifyStaffMessage(staff int64) []byte {
	msg := Message{
		Tag:  string(NotifyTag),
		Data: NotifyPayload{Staff: &staff},
	}
	return msg.Please()
}

// NotifyMessage builds the arrival frame: the badge and the new message.
//
// It carries no staff count. This frame goes to the recipient of one message,
// and the sender knows nothing about whether that person moderates. Leaving the
// field at zero is safe because the client only replaces a count it is actually
// sent: omitempty drops the field, and an absent field leaves the last staff
// count the connect frame gave it.
func NotifyMessage(unread int64, item NotifyItem) []byte {
	msg := Message{
		Tag:  string(NotifyTag),
		Data: NotifyPayload{Unread: &unread, Item: &item},
	}
	return msg.Please()
}

// NotifyBroadcastMessage builds the frame for a message sent to every account
// at once: the new row, and no count at all.
//
// It cannot carry one. This single frame goes to every signed-in socket on the
// site, and each account's unread total is its own — sending any number would
// overwrite everybody else's with one reader's. That is the same reason
// NotifyStaffMessage omits the personal count, and the client's rule covers
// both: an absent field means "unchanged".
//
// So the client adds one itself, which is the one place it is allowed to. A
// message that has just been created is unread for every account that existed
// before it, without exception — so +1 is not an estimate, it is the answer.
// The client still keys the bump on the row id, so a repeated frame cannot
// count twice, and the next socket connect replaces the total outright.
func NotifyBroadcastMessage(item NotifyItem) []byte {
	msg := Message{
		Tag:  string(NotifyTag),
		Data: NotifyPayload{Item: &item},
	}
	return msg.Please()
}
