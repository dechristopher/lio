package view

import (
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/www/ws/proto"
)

// The home activity region's wire projection (arch/HOME_ACTIVITY_STREAMING.md).
//
// The region is server-rendered once, on first paint, by the templ components
// below it, and streamed thereafter as JSON that the client renders. This file
// is the seam: it turns the same `message` structs the templ components take
// into the wire types the client renders from, so the two paints are fed by one
// source of data even though they are drawn by two renderers.
//
// It lives in `view` rather than beside the hub for two reasons. The relative
// time wording (shortAgo, RelativeDay) is here and must not be duplicated, and
// this is a presentation projection — deciding that a seek shows its speed group
// and not its raw variant is the same kind of decision the components make.
//
// Keep it in step with home.templ. A field added to a chip here and not there
// (or the reverse) shows up as a chip that changes shape the first time the
// socket refreshes it — see arch/HOME_ACTIVITY_STREAMING.md, "What we give up".

// HomeDigest projects the broadcast half of the activity region: the parts that
// are identical for every viewer.
//
// The viewer's own Following section is not here — it is per-socket, and
// HomeFollowingSection builds it.
func HomeDigest(challenges []message.OpenChallenge, stats message.SiteStats, c message.Community) proto.HomePayload {
	return proto.HomePayload{
		Stats: &proto.HomeStats{
			Playing: stats.Playing,
			Live:    stats.LiveGames,
			Total:   stats.TotalGames,
		},
		Challenges: &proto.HomeChallenges{Items: homeChallenges(challenges)},
		Players: &proto.HomePlayers{
			Online:   HomePlayers(c.Online),
			Anon:     c.Anon,
			Arrivals: homeArrivals(c.Newest),
		},
	}
}

// HomeFollowingSection projects one viewer's online follows into the per-socket
// section. Nil for a viewer who follows nobody who is here, which renders no
// section at all rather than an empty one.
func HomeFollowingSection(members []message.OnlineMember) *proto.HomeFollowing {
	return &proto.HomeFollowing{Items: HomePlayers(members)}
}

// HomePlayers projects roster chips. Shared by the broadcast roster and the
// per-socket Following section, because a followed player and a stranger are
// the same kind of row — the same reason rosterChip is one component.
func HomePlayers(members []message.OnlineMember) []proto.HomePlayer {
	out := make([]proto.HomePlayer, 0, len(members))
	for _, m := range members {
		// the account id deliberately stops here; see proto_home.go
		out = append(out, proto.HomePlayer{
			Name:      m.Username,
			Title:     m.Title.Code,
			TitleName: m.Title.Tooltip(),
			Playing:   m.Playing,
			Busy:      m.Busy,
		})
	}
	return out
}

// homeChallenges projects the open seeks.
func homeChallenges(challenges []message.OpenChallenge) []proto.HomeChallenge {
	out := make([]proto.HomeChallenge, 0, len(challenges))
	for _, c := range challenges {
		out = append(out, proto.HomeChallenge{
			RoomID: c.RoomID,
			// SpeedGroup, not Group: every seek is a deploy variant, so Group is
			// the constant "deploy" and says nothing — the same reasoning the
			// component's own comment gives.
			Variant:   c.Variant.Name,
			Speed:     c.Variant.SpeedGroup().String(),
			Color:     c.Color,
			RaceTo:    c.RaceTo,
			Rated:     c.Rated,
			Name:      c.CreatorName,
			Title:     c.CreatorTitle.Code,
			TitleName: c.CreatorTitle.Tooltip(),
			Rating:    c.CreatorRating,
		})
	}
	return out
}

// homeArrivals projects the recent registrations, formatting both relative
// times here so the client never words one itself.
func homeArrivals(members []message.NewMember) []proto.HomeArrival {
	out := make([]proto.HomeArrival, 0, len(members))
	for _, m := range members {
		out = append(out, proto.HomeArrival{
			Name:      m.Username,
			Title:     m.Title.Code,
			TitleName: m.Title.Tooltip(),
			Ago:       shortAgo(m.Joined),
			Joined:    "joined " + RelativeDay(m.Joined),
			// the id that resolved these stops here; see proto_home.go
			Online:  m.Online,
			Playing: m.Playing,
			Busy:    m.Busy,
		})
	}
	return out
}
