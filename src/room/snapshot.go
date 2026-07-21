package room

import (
	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/variant"
)

// HomeListing walks the active-room registry and returns the data behind the
// home-page activity feed: in-progress games, joinable open challenges, the
// derived site stats, and present — the set of distinct user ids currently
// connected to any room (seated players and spectators alike). The caller unions
// present with the home-page viewers to produce the site-wide "online now" count
// (see stats.Playing, which is seeded here with the in-room tally only).
//
// It is safe to call from HTTP handler goroutines — every room's mutable state
// is read under its own stateMu via snapshot, and the channel layer locks
// independently when its connected sockets are read.
func HomeListing() (live []message.LiveGame, challenges []message.OpenChallenge, stats message.SiteStats, present map[string]struct{}) {
	present = make(map[string]struct{})

	rooms.Range(func(_, value interface{}) bool {
		s := value.(*Instance).snapshot()

		switch s.state {
		case StateGameReady, StateGameOngoing:
			live = append(live, message.LiveGame{
				RoomID:  s.id,
				Variant: s.variant,
				VsBot:   s.vsBot,
				Moves:   s.moves,
			})
			stats.LiveGames++
		case StateWaitingForPlayers:
			// only list human-vs-human rooms that have an open seat and whose
			// creator opted into public listing; private challenges are reachable
			// by shared link only
			if !s.vsBot && s.openSeat && s.public {
				challenges = append(challenges, message.OpenChallenge{
					RoomID:  s.id,
					Variant: s.variant,
					Color:   s.joinerColor,
					RaceTo:  s.raceTo,
					Rated:   s.rated,
				})
				stats.OpenChallenges++
			}
		}

		// Tally every distinct human connected to this room toward the online
		// count: both seated players and spectators hold a socket on the room
		// channel, and bots hold none, so the SockMap contains only humans. Peek
		// never creates a SockMap, so walking idle rooms here spawns nothing.
		if sm := channel.Map.Peek(s.id); sm != nil {
			for _, sock := range sm.Sockets() {
				present[sock.UID] = struct{}{}
			}
		}
		return true
	})

	// in-room floor; the handler bumps this to the union with home-page viewers
	stats.Playing = len(present)
	return live, challenges, stats, present
}

// roomSnapshot is an immutable read of a room's display-relevant state,
// captured atomically under stateMu.
type roomSnapshot struct {
	id       string
	state    State
	variant  variant.Variant
	vsBot    bool
	moves    int
	openSeat bool
	public   bool
	// joinerColor is the side a visitor would take by joining (the still-open
	// seat) — the color shown on the home-page open challenge so a browser sees
	// the color they'd play, not the creator's. It is "r" (random) for a blind
	// room so the joiner doesn't preemptively learn their color.
	joinerColor string
	raceTo      int
	// rated marks a members-only (rated) seek vs an open (unrated) one — the
	// home list labels it and gates anonymous joining on it.
	rated bool
}

// snapshot reads the room's display-relevant state under stateMu so the home
// listing never races the room routine mutating the game/players.
func (r *Instance) snapshot() roomSnapshot {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	s := roomSnapshot{
		id:      r.ID,
		state:   State(r.stateMachine.Current()),
		variant: r.game.Variant,
		vsBot:   r.players.HasBot(),
		moves:   len(r.game.MoveHistory()),
		public:  r.public,
		raceTo:  r.params.RaceTo,
		rated:   r.params.Rated,
	}

	hasTwo, missing := r.players.HasTwoPlayers()
	s.openSeat = !hasTwo && missing != octad.NoColor

	// surface the still-open seat's color — the side a joiner would take — so a
	// browser sees the color they'd play. A blind (random-color) room hides it
	// behind "r" so the joiner doesn't preemptively learn their color; the board
	// reveals it once they join and the game begins.
	if r.blindColor {
		s.joinerColor = "r"
	} else if missing != octad.NoColor {
		s.joinerColor = missing.String()
	}

	return s
}
