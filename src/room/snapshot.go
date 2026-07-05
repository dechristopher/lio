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
					Color:   s.creatorColor,
					RaceTo:  s.raceTo,
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
	id           string
	state        State
	variant      variant.Variant
	vsBot        bool
	moves        int
	openSeat     bool
	public       bool
	creatorColor string
	raceTo       int
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
	}

	hasTwo, missing := r.players.HasTwoPlayers()
	s.openSeat = !hasTwo && missing != octad.NoColor

	// the creator holds the seat opposite the open one; surface their color so
	// a joiner knows which side they'll take
	if missing == octad.White {
		s.creatorColor = octad.Black.String()
	} else if missing == octad.Black {
		s.creatorColor = octad.White.String()
	}

	return s
}
