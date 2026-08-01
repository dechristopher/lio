package proto

// The home-page activity digest (arch/HOME_ACTIVITY_STREAMING.md): the stat
// tiles, the open challenges and the players roster, streamed over
// /socket/home instead of polled over HTTP.
//
// These types are a deliberate projection of the internal `message` structs
// rather than those structs marshalled directly. The reason is
// message.OnlineMember.ID, whose own comment requires that it never reach a
// client: it was safe while that struct was only ever rendered by templ, and
// putting it on a wire type would have made a `json:"-"` the only thing
// standing between an account id and every home page on the site. Projecting
// instead means the wire carries what it carries, and adding a field is a
// decision rather than an accident.
//
// Sections are pointers. nil means "unchanged since the last frame", and a
// non-nil section with an empty slice means "this is now empty" — a distinction
// omitempty alone cannot make, and getting it wrong leaves a challenge on the
// page after the last one is withdrawn.

// HomeStats are the three counters above the activity region.
type HomeStats struct {
	Playing int `json:"p"` // distinct humans holding a socket anywhere
	Live    int `json:"l"` // games in progress
	Total   int `json:"g"` // finished games in the archive
}

// HomeChallenge is one joinable open seek.
//
// Variant and Speed are the display strings rather than the variant itself: the
// client renders them and nothing more, and shipping the variant would invite
// the client to start deriving things the server already decided.
type HomeChallenge struct {
	RoomID  string `json:"r"`
	Variant string `json:"vn"`           // time control display name ("½ + 1")
	Speed   string `json:"sp"`           // speed group ("bullet")
	Color   string `json:"c"`            // the side a joiner takes: "w"/"b"/"r"
	RaceTo  int    `json:"rt,omitempty"` // match target; 0 = single game
	Rated   bool   `json:"rd,omitempty"` // members-only seek
	// Name / Title / TitleName are the challenger's identity, empty for an
	// anonymous creator (which the client renders as "Anonymous"). Rating is
	// only set for a rated seek, where it was captured at seat claim.
	Name      string `json:"n,omitempty"`
	Title     string `json:"t,omitempty"`
	TitleName string `json:"tn,omitempty"`
	Rating    string `json:"rg,omitempty"`
}

// HomePlayer is one roster chip: a named account currently on the site.
//
// Anonymous visitors are not representable here, exactly as they are not in
// message.OnlineMember — they are counted (HomeStats.Playing, HomePlayers.Anon)
// and never listed.
// It carries the same fields as HomeArrival below, under the same keys, and
// that is deliberate: the two are one chip and one client builder. The roster
// covers a 15-minute window rather than an instant, so a roster row has exactly
// what an arrival row always had — a presence flag that may be false, and a
// relative time to show when it is.
type HomePlayer struct {
	Name      string `json:"n"`
	Title     string `json:"t,omitempty"`
	TitleName string `json:"tn,omitempty"`
	// Online is a live socket at this instant. False for a member who was here
	// inside the window and has gone: no dot, and no challenge button, because
	// an invitation has to reach somebody who is actually here.
	Online  bool   `json:"o,omitempty"`
	Playing bool   `json:"p,omitempty"` // seated in a live game
	Busy    bool   `json:"b,omitempty"` // seated in any room, so not challengeable
	Ago     string `json:"a,omitempty"` // chip text for a departed member ("4m")
	Left    string `json:"j,omitempty"` // its tooltip ("left 4 minutes ago")
}

// HomeArrival is one recently registered account.
//
// Ago and Joined are pre-formatted server-side ("2d", "joined 2 days ago") so
// the two renderers cannot word the same relative time differently. They go
// stale between frames, which does not matter: the durations they describe are
// hours and days, and every digest refreshes them.
//
// Online / Playing / Busy are the same presence flags HomePlayer carries, and
// they are why this section is worth streaming at all rather than rendering
// once: the join date does not change, but whether that new player is here does.
// The account id behind the intersection stops on the server (see
// message.NewMember.ID).
type HomeArrival struct {
	Name      string `json:"n"`
	Title     string `json:"t,omitempty"`
	TitleName string `json:"tn,omitempty"`
	Ago       string `json:"a"` // chip text ("2d")
	Joined    string `json:"j"` // tooltip ("joined 2 days ago")
	// Playing and Busy deliberately use the same keys as HomePlayer's, so one
	// client builder reads both without a per-shape branch.
	Online  bool `json:"o,omitempty"` // holding a socket right now
	Playing bool `json:"p,omitempty"` // seated in a live game
	Busy    bool `json:"b,omitempty"` // seated in any room
}

// HomeChallenges wraps the seek list so the section can be a pointer and still
// distinguish "unchanged" from "now empty".
type HomeChallenges struct {
	Items []HomeChallenge `json:"i"`
}

// HomePlayers is the players card's broadcast half: the capped site-wide
// roster, the headcounts beside it, and the recent arrivals.
//
// Online is capped for display (see onlineShown); More is how many named
// members the cap left out, so the footnote can account for them rather than
// letting them vanish. The viewer's own Following section is NOT drawn from
// this list — it arrives separately, because a followed player can rank past
// the cap (see HomeFollowing).
type HomePlayers struct {
	Online   []HomePlayer  `json:"o"`
	Anon     int           `json:"a,omitempty"`
	More     int           `json:"m,omitempty"`
	Arrivals []HomeArrival `json:"n,omitempty"`
}

// HomeFollowing is one viewer's followed players who are active on the site,
// available ones first. It is addressed to a single socket, never broadcast.
//
// It carries whole chips rather than names because these players may not appear
// in the broadcast roster at all: that list is capped, and the entire point of
// this section is to surface somebody the viewer cares about who did not make
// that cut.
type HomeFollowing struct {
	Items []HomePlayer `json:"i"`
}

// FollowOnlinePayload is how many of the viewer's followed players are on the
// site right now — the green dot on the header's following control.
//
// It rides **every** socket, not just the home page's, because the header is on
// every page. That is also why it is its own message rather than a field on
// HomePayload: a reader sitting in a game or on their profile holds a room or
// notification socket and never receives a home digest, but their badge must
// still go out when somebody they follow signs on.
//
// A count of zero is still sent, on the same reasoning as the notification
// count: zero is the answer that clears a badge the page painted before the
// last followed player left.
type FollowOnlinePayload struct {
	Online int `json:"o"`
}

// FollowOnlineMessage builds the following-badge frame.
func FollowOnlineMessage(online int) []byte {
	msg := Message{
		Tag:  string(FollowOnlineTag),
		Data: FollowOnlinePayload{Online: online},
	}
	return msg.Please()
}

// HomeSelf is the one-shot per-socket hello for the activity region: who this
// viewer is, and whether they may send a challenge at all.
//
// Name lets the client drop the viewer's own chip from the roster — they know
// they are here, and the sword is never offered against yourself. Challenge
// carries the rest of the view-layer canChallenge test that is a property of
// the viewer rather than of the target (signed in, accounts enabled, not
// already seated), so the client can decide per row without re-deriving policy.
type HomeSelf struct {
	Name      string `json:"n,omitempty"`
	Challenge bool   `json:"c,omitempty"`
}

// HomePayload is the activity-region message. Any subset of the sections may be
// present; each non-nil one replaces what the client holds.
//
// Stats / Challenges / Players are broadcast to every viewer. Self and
// Following are per-socket and are only ever enqueued on one connection.
type HomePayload struct {
	Stats      *HomeStats      `json:"st,omitempty"`
	Challenges *HomeChallenges `json:"ch,omitempty"`
	Players    *HomePlayers    `json:"pl,omitempty"`
	Following  *HomeFollowing  `json:"fl,omitempty"`
	Self       *HomeSelf       `json:"me,omitempty"`
}

// Empty reports whether the payload would tell a client nothing, so the hub can
// skip marshalling and sending it.
func (h *HomePayload) Empty() bool {
	return h.Stats == nil && h.Challenges == nil && h.Players == nil &&
		h.Following == nil && h.Self == nil
}

// Marshal fully JSON marshals the HomePayload and wraps it in a Message struct.
func (h *HomePayload) Marshal() []byte {
	message := Message{
		Tag:  string(HomeTag),
		Data: h,
	}

	return message.Please()
}
