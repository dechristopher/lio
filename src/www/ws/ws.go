package ws

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
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
func UpgradeHandler(c *fiber.Ctx) error {
	uid := user.GetID(c)
	if uid == "" {
		c.Status(403)
		util.Error(str.CWS, str.EWSNoUid, c.String())
		return nil
	}

	// IsWebSocketUpgrade returns true if the client
	// requested upgrade to the WebSocket protocol and
	// originates from a trusted origin.
	if websocket.IsWebSocketUpgrade(c) && okOrigin(c) {
		return c.Next()
	}
	return fiber.ErrUpgradeRequired
}

// ConnHandler returns a wrapped websocket connection handler
// for various websocket use-cases across the site
func ConnHandler() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		return websocket.New(connHandler(ctx), websocket.Config{
			EnableCompression: true,
		})(ctx)
	}
}

// connHandler returns the actual websocket handler implementation
func connHandler(ctx *fiber.Ctx) func(*websocket.Conn) {
	// get uid and channel from fiber context
	uid := user.GetID(ctx)
	roomId := ctx.Params("chan")
	thisChannel := roomId

	// prepend channel type to channel name if it exists
	if channelType := ctx.Params("type"); channelType != "" {
		thisChannel = fmt.Sprintf("%s/%s", channelType, thisChannel)
	}

	// ensure room exists for connection. The global TV channel (/socket/tv) is
	// not a room — it is a site-wide read-only stream — so it bypasses this check.
	if !tv.IsTV(roomId) {
		if thisRoom, err := room.Get(roomId); thisRoom == nil {
			util.Error(str.CWS, str.EWSConn, err.Error())
			return func(conn *websocket.Conn) {
				_ = conn.Close()
			}
		}
	}

	util.Info(str.CWS, str.MWSConn, uid, thisChannel, ctx.IP())

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

		// the global TV channel pushes a one-shot grid snapshot on connect so a
		// new viewer immediately sees the current featured games, then receives
		// add/move/remove deltas via the normal broadcast path
		if tv.IsTV(roomId) {
			tv.Connect(socket)
		}

		// UnTrack this socket and stop its writer when the read loop exits
		defer killSocket(socket, thisChannel)

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
				UID:     uid,
				Channel: thisChannel,
				RoomID:  roomId,
				MT:      mt,
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

// okOrigin approves a websocket connection if it comes from an origin we trust
func okOrigin(c *fiber.Ctx) bool {
	if env.IsLocal() {
		return true
	}

	origin := c.Context().Request.Header.Peek("Origin")
	return strings.Contains(config.CorsOrigins(), string(origin))
}
