package ws

import (
	"fmt"
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
	// Map[channel] -> SockMap (map[string]Socket)
	chanMap = make(map[string]common.SockMap)
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

	// Keep track of all chanMap for off-rpc broadcasts
	// Create a new SockMap and track it under the channel key
	if chanMap[channel].C == nil {
		chanMap[channel] = common.NewSockMap(channel)
		go crowdHandler(channel)
	}

	// track this socket in the corresponding SockMap
	lock := &sync.Mutex{}
	chanMap[channel].Track(bid, common.Socket{
		Connection: c,
		Mutex:      lock,
	})

	// UnTrack this socket when it disconnects
	defer killSocket(c, channel, bid)

	for {
		// read raw incoming messages from socket
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
			Sockets: chanMap,
			BID:     bid,
			Channel: channel,
			MT:      mt,
		})

		// print response to debug out
		util.Debug(str.CWS, str.DWSSend, string(resp))

		lock.Lock()
		// acquire socket lock, write bytes, and release lock
		if err = c.WriteMessage(mt, resp); err != nil {
			util.Error(str.CWS, str.EWSWrite, resp, err.Error())
			break
		}
		lock.Unlock()
	}
}

// crowdHandler monitors chanMap on a channel and emits crowd message
// broadcasts to everyone in the channel
func crowdHandler(channel string) {
	meta := common.SocketMeta{
		Sockets: chanMap,
		Channel: channel,
		MT:      1,
	}
	for {
		e := <-chanMap[channel].C
		msg := fmt.Sprintf(`{"t":"c","d":{"s":%d}}`, e)
		common.Broadcast([]byte(msg), meta)
	}
}

// killSocket closes the websocket connection and removes the socket
// reference from the chanMap map
func killSocket(conn *websocket.Conn, channel string, bid string) {
	chanMap[channel].UnTrack(bid)
	// free up memory in chanMap if the SockMap is empty
	if chanMap[channel].Empty() {
		delete(chanMap, channel)
	}
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
