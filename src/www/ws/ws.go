package ws

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/valyala/fastjson"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
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

	// ensure room exists for connection
	if thisRoom, err := room.Get(roomId); thisRoom == nil {
		util.Error(str.CWS, str.EWSConn, err.Error())
		return func(conn *websocket.Conn) {
			_ = conn.Close()
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

		// websocket.Conn bindings
		// https://pkg.go.dev/github.com/fasthttp/websocket?tab=doc#pkg-index
		var (
			mt  int
			b   []byte
			err error
		)

		// track this socket in the corresponding SockMap
		lock := &sync.Mutex{}
		channel.Map.GetSockMap(thisChannel).Track(uid, &channel.Socket{
			Connection: c,
			Mutex:      lock,
			Type:       c.Params("type"),
		})

		// UnTrack this socket when it disconnects
		defer killSocket(c, thisChannel, uid)

		for {
			// read raw incoming messages from socket
			if mt, b, err = c.ReadMessage(); err != nil {
				// don't log clean websocket close messages
				if !websocket.IsCloseError(err, websocket.CloseGoingAway) {
					util.Info(str.CWS, str.EWSRead, err.Error())
				}
				break
			}

			if fastjson.GetInt(b, "pi") == 1 {
				// write pong message asap and continue
				lock.Lock()
				_ = c.WriteMessage(mt, proto.Pong())
				lock.Unlock()
				continue
			}

			// TODO improve safety of heartbeats to prevent DoS
			//if len(b) == 4 {
			//fmt.Println(mt, b)
			// write heartbeat ack asap and continue
			//lock.Lock()
			//_ = c.WriteMessage(mt, []byte("0"))
			//lock.Unlock()
			//	continue
			//}

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

			// return immediate response if any given
			// print response to debug out
			util.DebugFlag("ws", str.CWS, str.DWSSend, string(resp))

			lock.Lock()
			// acquire socket lock, write bytes, and release lock
			if err = c.WriteMessage(mt, resp); err != nil {
				util.Error(str.CWS, str.EWSWrite, resp, err.Error())
				break
			}
			lock.Unlock()
		}
	}
}

// killSocket closes the websocket connection and removes the socket
// reference from the ChanMap map
func killSocket(conn *websocket.Conn, thisChannel string, uid string) {
	util.Info(str.CWS, str.MWSDisc, uid, thisChannel, conn.RemoteAddr())
	channel.Map.GetSockMap(thisChannel).UnTrack(uid)
	_ = conn.Close()
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
