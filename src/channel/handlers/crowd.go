package handlers

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// HandleCrowd monitors ChanMap on a channel and emits crowd message broadcasts
// to everyone in the channel: per-seat presence for the two players plus the
// count of connected spectators (seated players excluded). seats supplies the
// current white/black uids — a closure provided by the room so this package
// never has to import it (room already imports this package).
func HandleCrowd(thisChannel string, seats func() (white, black string)) {
	meta := channel.SocketContext{
		Channel: thisChannel,
		MT:      1,
	}
	sockMap := channel.Map.GetSockMap(thisChannel)
	// range over channel entries until it is closed, then exit routine. Each
	// receive is a wakeup: re-derive presence and the spectator count from the
	// live SockMap rather than trusting the (coalesced) count payload.
	for range sockMap.Listen() {
		payload := crowdPayload(sockMap, seats)
		util.DebugFlag("crowd", str.CChan, "w: %t b: %t spec: %d",
			payload.White, payload.Black, payload.Spec)
		payload.Broadcast(meta)
	}

	util.DebugFlag("crowd", str.CChan, "cleanup: %s", thisChannel)
}

// crowdPayload derives the crowd broadcast from the live SockMap: each seat's
// presence (a bot seat's uid never holds a socket, so it reads false — the
// client keys nothing off bot presence) and the spectator count, which is the
// distinct connected uids minus the connected seats.
func crowdPayload(sockMap *channel.SockMap, seats func() (white, black string)) proto.CrowdPayload {
	white, black := seats()
	whiteConnected := sockMap.Connected(white)
	blackConnected := sockMap.Connected(black)

	spectators := sockMap.Length()
	if whiteConnected {
		spectators--
	}
	if blackConnected {
		spectators--
	}

	return proto.CrowdPayload{
		White: whiteConnected,
		Black: blackConnected,
		Spec:  spectators,
	}
}
