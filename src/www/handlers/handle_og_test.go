package handlers

import (
	"image/png"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/dechristopher/octad/v2"
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/og"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/variant"
)

// ogTestApp registers the OG card routes exactly as www.Serve does, loading
// the card renderer's board/piece art from the on-disk static tree (the same
// files the embedded FS serves in production).
func ogTestApp(t *testing.T) *fiber.App {
	t.Helper()
	if err := og.LoadAssets(os.DirFS("../../cmd/lio/static")); err != nil {
		t.Fatalf("load og assets: %v", err)
	}
	app := fiber.New()
	app.Get("/og/default.png", OGDefaultHandler)
	app.Get("/og/room/:id", OGRoomHandler)
	return app
}

// fetchCard requests path and asserts a decodable PNG of the standard
// OpenGraph card dimensions comes back.
func fetchCard(t *testing.T, app *fiber.App, path string) {
	t.Helper()

	resp, err := app.Test(httptest.NewRequest("GET", path, nil))
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET %s: status %d, want 200", path, resp.StatusCode)
	}
	if ct := resp.Header.Get(fiber.HeaderContentType); ct != "image/png" {
		t.Fatalf("GET %s: content-type %q, want image/png", path, ct)
	}

	img, err := png.Decode(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: decode png: %v", path, err)
	}
	b := img.Bounds()
	if b.Dx() != og.Width || b.Dy() != og.Height {
		t.Fatalf("GET %s: card is %dx%d, want %dx%d",
			path, b.Dx(), b.Dy(), og.Width, og.Height)
	}
}

func TestOGDefaultCard(t *testing.T) {
	fetchCard(t, ogTestApp(t), "/og/default.png")
}

// TestOGRoomCard exercises the room card against a real registered room, and
// the default-card fallback for an unknown room id.
func TestOGRoomCard(t *testing.T) {
	params := room.NewParams("og-test-user", variant.HalfOneBlitz)
	params.Players[octad.White] = &player.Player{ID: "og-test-user"}
	toJoin := player.ToJoin
	params.Players[octad.Black] = &toJoin

	instance, err := room.Create(params)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	app := ogTestApp(t)
	fetchCard(t, app, "/og/room/"+instance.ID+".png")
	// unknown rooms serve the default card, never a 404 image
	fetchCard(t, app, "/og/room/zzzzzzz.png")
}
