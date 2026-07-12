package ws

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fastjson"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/tv"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
	"github.com/dechristopher/lio/www/ws/routes"
)

// UpgradeHandler catches anything under /ws/** and allows
// the websocket connection through the "allowed" local
func UpgradeHandler(c fiber.Ctx) error {
	// IsWebSocketUpgrade returns true if the client
	// requested upgrade to the WebSocket protocol and
	// originates from a trusted origin.
	//
	// An upgrade with no identity (the context middleware never mints one for
	// socket paths — iOS Safari intermittently omits cookies from WS upgrade
	// requests) is allowed through to connHandler, which completes the
	// handshake and immediately closes with closeNoIdentity — a machine-readable
	// "re-authenticate" signal the client recovers from with a page reload. A
	// 403 here would surface client-side as an opaque handshake failure
	// indistinguishable from any other outage.
	if websocket.IsWebSocketUpgrade(c) && okOrigin(c) {
		return c.Next()
	}
	return fiber.ErrUpgradeRequired
}

// closeNoIdentity is the WebSocket close code sent when an upgrade carried no
// usable identity cookies. Client code (lio.js onclose) treats it as a signal
// to re-authenticate via one guarded page reload — a full navigation reliably
// carries (or re-mints) the identity cookies that the WS upgrade lost.
const closeNoIdentity = 4001

// maxInboundMessage caps a single inbound websocket frame. Every legitimate
// client message is tiny JSON (moves, deploys, pings), so a small ceiling
// bounds memory per socket and, with permessage-deflate on, defuses a
// compressed-frame decompression bomb. A frame over the limit makes ReadMessage
// return an error, which unwinds the read loop and tears the socket down.
const maxInboundMessage = 4096

// ConnHandler returns a wrapped websocket connection handler
// for various websocket use-cases across the site
func ConnHandler() fiber.Handler {
	return func(ctx fiber.Ctx) error {
		return websocket.New(connHandler(ctx), websocket.Config{
			// NOTE: leaving compression ON. Flipping it OFF correlated with iOS
			// Safari falling into a permanent "RECONNECTING" loop on a local build,
			// and the deploy-sync bug this was meant to test occurs with it on
			// anyway (the submit is recorded correctly — see ios-deploy-confirm-bug),
			// so permessage-deflate is not the deploy cause. Do not disable without
			// verifying iOS handshake behavior against fasthttp/websocket first.
			EnableCompression: true,
		})(ctx)
	}
}

// connHandler returns the actual websocket handler implementation
func connHandler(ctx fiber.Ctx) func(*websocket.Conn) {
	// Deep-copy every string taken from the fiber ctx. The ctx — and the pooled
	// fasthttp buffers backing its strings — is recycled for other requests the
	// moment this builder returns, while the returned handler runs for the
	// socket's whole life. A captured view mutates in place when the buffer is
	// reused (any concurrent page/asset/join request can trigger it), after
	// which room.Get(roomId) fails for every subsequent frame on this socket:
	// inbound moves and deploy submissions silently die while pings and
	// broadcasts keep flowing — the "confirm/move does nothing until refresh"
	// wedge. The contrib websocket wrapper CopyStrings its own params for
	// exactly this reason; these captures bypassed it.
	uid := strings.Clone(user.GetID(ctx))

	// no usable identity on the upgrade (missing/mismatched cookies — an iOS
	// Safari hazard): complete the handshake, then close with the dedicated
	// code so the client re-authenticates instead of being silently seated as
	// a spectator whose game frames the handlers would drop.
	if uid == "" {
		util.Error(str.CWS, str.EWSNoUid, ctx.IP())
		return func(c *websocket.Conn) {
			_ = c.SetWriteDeadline(time.Now().Add(channel.WriteWait))
			_ = c.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(closeNoIdentity, "identity required"))
			_ = c.Close()
		}
	}

	// cloned for the same buffer-reuse reason as uid above: roomId is the string
	// every inbound frame resolves the room with, for the socket's whole life
	roomId := strings.Clone(ctx.Params("chan"))
	thisChannel := roomId

	// prepend channel type to channel name if it exists (Sprintf allocates, so
	// this branch is safe without an explicit clone)
	if channelType := ctx.Params("type"); channelType != "" {
		thisChannel = fmt.Sprintf("%s/%s", channelType, thisChannel)
	}

	// ensure room exists for connection. The global TV channel (/socket/tv) is
	// not a room — it is a site-wide read-only stream — so it bypasses this check.
	//
	// isSpectator is decided once, at connect time: a uid with no seat in the
	// room (or any TV viewer) is a spectator, and every message it sends is
	// tagged as such so game-affecting handlers drop it outright. Seat
	// membership is stable for a socket's lifetime — a joiner claims their seat
	// over HTTP before opening the game socket, and rematches flip colors, not
	// uids — so this never goes stale.
	isSpectator := true
	if !tv.IsTV(roomId) {
		thisRoom, err := room.Get(roomId)
		if thisRoom == nil {
			util.Error(str.CWS, str.EWSConn, err.Error())
			// the room this page is bound to no longer exists — typically an
			// open challenge dropped by a server restart (waiting rooms are not
			// persisted), or a client that missed its room's teardown. Complete
			// the handshake and send the client home with a notice instead of
			// silently closing and leaving it to reconnect-loop forever: both
			// the game page (lio.js default handler) and the waiting page act
			// on the redirect frame.
			return func(conn *websocket.Conn) {
				_ = conn.SetWriteDeadline(time.Now().Add(channel.WriteWait))
				redir := proto.RedirectMessage{Location: "/?notice=room-gone"}
				_ = conn.WriteMessage(websocket.TextMessage, redir.Marshal())
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "room gone"))
				_ = conn.Close()
			}
		}
		isSpectator = !thisRoom.IsPlayer(uid)
	}

	util.Info(str.CWS, str.MWSConn, uid, thisChannel, ctx.IP(), isSpectator)

	// return websocket handler injected with values from request context
	return func(c *websocket.Conn) {
		// recover panicked websocket handlers
		defer func() {
			err := recover()
			if err != nil {
				util.Error(str.CWS, "[%s @ %s] recovered panicked ws handler: %v", uid, roomId, err)
			}
		}()

		// Unique per-connection id so multiple connections for the same uid
		// (extra tabs, or independent multimodal streams) are tracked
		// independently, and a stale connection's teardown can never evict a
		// newer live socket for the same uid.
		connID := config.GenerateCode(16)
		socket := channel.NewSocket(c, uid, connID, c.Params("type"))

		// track this socket in the corresponding SockMap
		channel.Map.GetSockMap(thisChannel).Track(socket)

		// the writer goroutine owns all writes to this connection and emits
		// periodic protocol-level pings for liveness
		go socket.WritePump()

		// one-shot identity echo: tell the client who this socket authenticated
		// as and how it was seated. A player page bound to a spectator socket
		// (stale/partial cookies on the upgrade) detects the desync from this
		// frame and re-authenticates instead of playing into the void.
		socket.Enqueue(proto.IdentityMessage(uid, isSpectator))

		// one-shot version hello: a page reconnecting across a deploy compares
		// this against the version it was rendered by and surfaces a passive
		// "updated — refresh" prompt on mismatch (lio.js)
		socket.Enqueue(proto.ServerInfoMessage(config.VersionString()))

		// the global TV channel pushes a one-shot grid snapshot on connect so a
		// new viewer immediately sees the current featured games, then receives
		// add/move/remove deltas via the normal broadcast path
		if tv.IsTV(roomId) {
			tv.Connect(socket)
		}

		// UnTrack this socket and stop its writer when the read loop exits
		defer killSocket(socket, thisChannel)

		// Bound the size of any single inbound frame (see maxInboundMessage).
		c.SetReadLimit(maxInboundMessage)

		// Server-driven liveness: a vanished client (no TCP FIN) stops
		// answering pings, so the read deadline fires and unwinds this loop.
		// Any inbound traffic — including the client's app-level pings and the
		// browser's automatic pong to our ping frames — refreshes the deadline.
		_ = c.SetReadDeadline(time.Now().Add(channel.PongWait))
		c.SetPongHandler(func(string) error {
			return c.SetReadDeadline(time.Now().Add(channel.PongWait))
		})

		for {
			// read raw incoming messages from socket
			mt, b, err := c.ReadMessage()
			if err != nil {
				// don't log clean websocket close messages
				if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					util.Info(str.CWS, str.EWSRead, err.Error())
				}
				break
			}

			// any successful read means the client is alive; extend liveness
			_ = c.SetReadDeadline(time.Now().Add(channel.PongWait))

			if fastjson.GetInt(b, "pi") == 1 {
				// queue pong reply and continue
				socket.Enqueue(proto.Pong())
				continue
			}

			util.DebugFlag("ws", str.CWS, str.DWSRecv, string(b))

			// pull message tag for routing decision
			tag := fastjson.GetString(b, "t")

			// ignore if no route
			if !validTag(tag) {
				continue
			}

			// route message to proper handler and await response
			resp := routes.Map[proto.PayloadTag(tag)](b, channel.SocketContext{
				UID:         uid,
				Channel:     thisChannel,
				RoomID:      roomId,
				IsSpectator: isSpectator,
				MT:          mt,
			})

			if resp == nil {
				continue
			}

			// queue immediate response if any given
			util.DebugFlag("ws", str.CWS, str.DWSSend, string(resp))
			socket.Enqueue(resp)
		}
	}
}

// killSocket untracks the connection and signals its writer goroutine to close
// the underlying websocket.
func killSocket(socket *channel.Socket, thisChannel string) {
	util.Info(str.CWS, str.MWSDisc, socket.UID, thisChannel, socket.Connection.RemoteAddr())
	channel.Map.GetSockMap(thisChannel).UnTrack(socket.UID, socket.ID)
	socket.Close()
}

// validTag returns true if the message tag has a valid handler route
func validTag(tag string) bool {
	_, ok := routes.Map[proto.PayloadTag(tag)]
	return ok
}

// okOrigin approves a websocket connection if it comes from an origin we
// trust. The Origin header must exactly match an entry in the configured
// origin list — the old substring check would have admitted registerable
// near-miss domains (e.g. an Origin of https://lioctad.or is a substring of
// https://lioctad.org). An absent Origin is allowed through: only non-browser
// clients omit it, and the check exists to stop cross-site browser pages, not
// curl. Rejections are logged because they surface client-side as an opaque
// handshake failure indistinguishable from an outage (LAN-device testing
// against a non-local env died silently here).
func okOrigin(c fiber.Ctx) bool {
	if env.IsLocal() {
		return true
	}

	origin := string(c.RequestCtx().Request.Header.Peek("Origin"))
	if origin == "" {
		return true
	}
	for _, allowed := range strings.Split(config.CorsOrigins(), ",") {
		if origin == strings.TrimSpace(allowed) {
			return true
		}
	}

	util.Error(str.CWS, str.EWSBadOrigin, origin, c.Path())
	return false
}
