package ws

import (
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"

	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/lioctad/www/ws/proto"
	"github.com/dechristopher/lioctad/www/ws/routes"
)

var (
	// Map[channel][userID] -> websocket connection
	sockets = make(map[string]map[string]Socket)
)

// Socket is a struct combining a websocket connection and a mutex lock
// for best practice, protected synchronous reads and writes to websockets
type Socket struct {
	Connection *websocket.Conn
	Mutex      *sync.Mutex
}

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
	// requested upgrade to the WebSocket protocol.
	if websocket.IsWebSocketUpgrade(c) {
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
		msg proto.Message
		err error
	)

	// Keep track of all sockets for off-rpc broadcasts
	if sockets["test"] == nil {
		sockets["test"] = make(map[string]Socket)
	}

	bid := c.Cookies("bid")
	channel := c.Params("chan")

	lock := &sync.Mutex{}
	sockets[channel][c.Cookies("bid")] = Socket{
		Connection: c,
		Mutex:      lock,
	}

	defer killSocket(c, channel, bid)

	for {
		if err = c.ReadJSON(&msg); err != nil {
			util.Debug(str.CWS, str.EWSRead, err.Error())
			break
		}

		msg.Channel = channel
		util.Debug(str.CWS, str.DWSRecv, msg)

		// route message to proper handler and await response
		resp := routes.Map[msg.Command](msg)

		util.Debug(str.CWS, str.DWSSend, resp)

		lock.Lock()
		if err = c.WriteJSON(resp); err != nil {
			util.Debug(str.CWS, str.EWSWrite, err.Error())
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
