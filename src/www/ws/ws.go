package ws

import (
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"

	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/lioctad/www/ws/common"
	"github.com/dechristopher/lioctad/www/ws/proto"
	"github.com/dechristopher/lioctad/www/ws/routes"

	"github.com/valyala/fastjson"
)

var (
	// Map[channel][userID] -> websocket connection
	sockets = make(map[string]map[string]common.Socket)
)

// UpgradeHandler catches anything under /ws/** and allows
// the websocket connection through the "allowed" local
func UpgradeHandler(c *fiber.Ctx) error {
	bid := c.Cookies("bid")
	if bid == "" {
		c.Status(403)
		util.Error(str.CWS, str.EWSNoBid, c.String())
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

// ConnHandler is the global websocket connection handler
// for various websocket use-cases across the site
func ConnHandler(c *websocket.Conn) {
	// websocket.Conn bindings
	// https://pkg.go.dev/github.com/fasthttp/websocket?tab=doc#pkg-index
	var (
		mt  int
		b   []byte
		err error
	)

	bid := c.Cookies("bid")
	channel := c.Params("chan")

	// Keep track of all sockets for off-rpc broadcasts
	if sockets[channel] == nil {
		sockets[channel] = make(map[string]common.Socket)
	}

	lock := &sync.Mutex{}
	sockets[channel][c.Cookies("bid")] = common.Socket{
		Connection: c,
		Mutex:      lock,
	}

	defer killSocket(c, channel, bid)

	util.Debug(str.CWS, "ref: %s", c.Locals("ref"))

	for {
		if mt, b, err = c.ReadMessage(); err != nil {
			util.Error(str.CWS, str.EWSRead, err.Error())
			break
		}

		// TODO improve safety of heartbeats to prevent DoS
		if len(b) == 4 {
			// write heartbeat ack asap and continue
			_ = c.WriteMessage(mt, []byte("0"))
			continue
		}

		util.Debug(str.CWS, str.DWSRecv, string(b))

		// pull message tag for routing decision
		tag := fastjson.GetString(b, "t")

		// ignore if no route
		if !validTag(tag) {
			continue
		}

		// route message to proper handler and await response
		resp := routes.Map[proto.PayloadTag(tag)](b, common.SocketMeta{
			Sockets: sockets,
			BID:     bid,
			Channel: channel,
			MT:      mt,
		})

		// print response to debug out
		util.Debug(str.CWS, str.DWSSend, string(resp))

		lock.Lock()
		// acquire socket lock, write bytes, and release lock
		if err = c.WriteMessage(mt, resp); err != nil {
			util.Error(str.CWS, str.EWSWrite, err.Error())
			break
		}
		lock.Unlock()
	}
}

// killSocket closes the websocket connection and removes the socket
// reference from the sockets map
func killSocket(conn *websocket.Conn, channel string, bid string) {
	delete(sockets[channel], bid)
	_ = conn.Close()
}

// validTag returns true if the message tag has a valid handler route
func validTag(tag string) bool {
	_, ok := routes.Map[proto.PayloadTag(tag)]
	return ok
}

// okOrigin approves a websocket connection if it comes from an origin we trust
func okOrigin(c *fiber.Ctx) bool {
	origin := c.Context().Request.Header.Peek("Origin")
	return strings.Contains(util.CorsOrigins(), string(origin))
}
