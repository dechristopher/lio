package www

import (
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"

	"github.com/dechristopher/lioctad/www/handlers"
	"github.com/dechristopher/lioctad/www/middleware"
)

// WireHandlers builds all of the websocket and http routes
// into the fiber app context
func WireHandlers(r *fiber.App, staticFs http.FileSystem) {

	// websocket upgrade intermediate route
	// catches anything under /ws/** and allows the
	// websocket connection through "allowed" local
	r.Use("/ws", func(c *fiber.Ctx) error {
		// IsWebSocketUpgrade returns true if the client
		// requested upgrade to the WebSocket protocol.
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// websocket connection listener
	r.Get("/ws/:id", websocket.New(func(c *websocket.Conn) {
		// c.Locals is added to the *websocket.Conn
		log.Println(c.Locals("allowed"))  // true
		log.Println(c.Params("id"))       // 123
		log.Println(c.Query("v"))         // 1.0
		log.Println(c.Cookies("session")) // ""

		// websocket.Conn bindings
		// https://pkg.go.dev/github.com/fasthttp/websocket?tab=doc#pkg-index
		var (
			mt  int
			msg []byte
			err error
		)
		for {
			if mt, msg, err = c.ReadMessage(); err != nil {
				log.Println("read:", err)
				break
			}
			log.Printf("recv: %s", msg)

			if err = c.WriteMessage(mt, msg); err != nil {
				log.Println("write:", err)
				break
			}
		}
	}))

	// sub-router with compression enabled
	sub := r.Group("/")

	// wire up all middleware components
	middleware.WireMiddleware(sub, staticFs)

	// home handler
	r.Get("/", handlers.IndexHandler)

	// Custom 404 page
	middleware.NotFound(r)
}
