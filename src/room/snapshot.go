package room

import (
	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/title"
	"github.com/dechristopher/lio/variant"
)

// HomeListing walks the active-room registry and returns the data behind the
// home-page activity feed: in-progress games, joinable open challenges, the
// derived site stats, and present — the distinct users currently connected to
// any room (seated players and spectators alike), mapped to their account
// identity where they hold one. The caller unions present with the home-page
// viewers to produce the site-wide "online now" count and its named roster (see
// stats.Playing, which is seeded here with the in-room tally only).
//
// It is safe to call from HTTP handler goroutines — every room's mutable state
// is read under its own stateMu via snapshot, and the channel layer locks
// independently when its connected sockets are read.
func HomeListing() (live []message.LiveGame, challenges []message.OpenChallenge, stats message.SiteStats, present map[string]message.OnlineMember) {
	present = make(map[string]message.OnlineMember)

	rooms.Range(func(_, value interface{}) bool {
		s := value.(*Instance).snapshot()

		switch {
		case s.state.Live():
			live = append(live, message.LiveGame{
				RoomID:  s.id,
				Variant: s.variant,
				VsBot:   s.vsBot,
				Moves:   s.moves,
			})
			stats.LiveGames++
		case s.state == StateWaitingForPlayers:
			// only list human-vs-human rooms that have an open seat and whose
			// creator opted into public listing; private challenges are reachable
			// by shared link only
			if !s.vsBot && s.openSeat && s.public {
				challenges = append(challenges, message.OpenChallenge{
					RoomID:        s.id,
					Variant:       s.variant,
					Color:         s.joinerColor,
					RaceTo:        s.raceTo,
					Rated:         s.rated,
					CreatorName:   s.creatorName,
					CreatorTitle:  s.creatorTitle,
					CreatorRating: s.creatorRating,
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
				present[sock.UID] = message.OnlineMember{}
			}
		}

		// Overlay account identities onto the connected set. Names come from the
		// seats (the room knows who is sitting there), but presence stays
		// socket-derived: a seated player who has dropped their connection is
		// counted by neither, so nobody is listed as online on the strength of a
		// seat they are no longer holding open.
		for uid, m := range s.seated {
			if _, connected := present[uid]; connected {
				present[uid] = m
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
	// creatorName / creatorTitle / creatorRating are the challenger's account
	// identity, put on the home-page seek so it reads as a person. Zero-valued
	// for an anonymous creator; creatorRating is only captured for a rated room.
	creatorName   string
	creatorTitle  title.Title
	creatorRating string
	// seated maps each seated human's uid to their account identity (zero-valued
	// for an anonymous seat), so the home page can name the members currently at
	// a board. Bots hold no uid and are never included.
	seated map[string]message.OnlineMember
}

// snapshot reads the room's display-relevant state under stateMu so the home
// listing never races the room routine mutating the game/players.
func (r *Instance) snapshot() roomSnapshot {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	s := roomSnapshot{
		id:           r.ID,
		state:        State(r.stateMachine.Current()),
		variant:      r.game.Variant,
		vsBot:        r.players.HasBot(),
		moves:        len(r.game.MoveHistory()),
		public:       r.public,
		raceTo:       r.params.RaceTo,
		rated:        r.params.Rated,
		creatorName:  r.params.CreatorName,
		creatorTitle: r.params.CreatorTitle,
	}

	hasTwo, missing := r.players.HasTwoPlayers()
	s.openSeat = !hasTwo && missing != octad.NoColor

	// Seat identities: the creator's rating for the home-page seek (captured at
	// seat-claim, so this reads memory rather than the database), and the named
	// members at this board for the online roster. A member is "playing" exactly
	// when their room is live — the same predicate the live-games tally uses, so
	// the roster can never disagree with the counter beside it.
	playing := s.state.Live()
	for _, color := range []octad.Color{octad.White, octad.Black} {
		p := r.players[color]
		if p == nil || p.IsBot {
			continue
		}
		if s.rated && p.ID == r.creator {
			s.creatorRating = p.RatingDisplay
		}
		if s.seated == nil {
			s.seated = make(map[string]message.OnlineMember, 2)
		}
		// Busy is unconditional here: every seat in the registry belongs to a
		// room, live or still waiting for an opponent, and either way that
		// person is not available to be challenged. Playing stays narrower — it
		// says they are at a board right now, which is what the roster label
		// claims.
		s.seated[p.ID] = message.OnlineMember{
			Username: p.Username,
			Title:    p.Title,
			Playing:  playing,
			Busy:     true,
		}
	}

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
