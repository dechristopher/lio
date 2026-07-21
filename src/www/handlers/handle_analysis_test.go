package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/bus"
)

// startOFEN is octad's standard starting position.
const startOFEN = "ppkn/4/4/NKPP w NCFncf - 0 1"

// analysisTestApp registers the analysis route as www.Serve does (sans the
// rate limiter — these tests exercise the handler, not the limiter). The
// engine's search publishes to the in-process event bus and blocks until it
// is up (production starts it in systems.Run), so bring it up here — the
// established test pattern (see room/main_test.go).
func analysisTestApp() *fiber.App {
	bus.Up()
	app := fiber.New()
	app.Post("/api/analysis", AnalysisHandler)
	return app
}

// postAnalysis runs one request through the handler, returning the status and
// decoded body. The generous timeout covers the budgeted engine search a
// cache-missing eval performs (PG is unconfigured in unit tests, so every
// eval is a live search).
func postAnalysis(t *testing.T, app *fiber.App, body string) (int, analysisResponse) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/analysis",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("analysis request: %v", err)
	}
	var out analysisResponse
	if resp.StatusCode == fiber.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return resp.StatusCode, out
}

// TestAnalysisDescribePosition covers the no-move form: describing the start
// position yields its legal-move map and a (searched) eval, no SAN.
func TestAnalysisDescribePosition(t *testing.T) {
	app := analysisTestApp()
	status, out := postAnalysis(t, app, `{"ofen":"`+startOFEN+`"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if out.OFEN != startOFEN {
		t.Errorf("ofen = %q, want the start position back", out.OFEN)
	}
	if out.SAN != "" {
		t.Errorf("san = %q, want empty without a move", out.SAN)
	}
	if out.Over != "" {
		t.Errorf("over = %q, want playable", out.Over)
	}
	// white to move: the c1/d1 pawns and b1 king/a1 knight have moves
	if len(out.Dests) == 0 || len(out.Dests["c1"]) == 0 {
		t.Errorf("dests = %v, want legal moves incl. c1", out.Dests)
	}
}

// TestAnalysisApplyMove covers the move form: applying c1c2 to the start
// position returns the SAN, the advanced position, and black's moves.
func TestAnalysisApplyMove(t *testing.T) {
	app := analysisTestApp()
	status, out := postAnalysis(t, app,
		`{"ofen":"`+startOFEN+`","uoi":"c1c2"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if out.SAN != "c2" {
		t.Errorf("san = %q, want %q", out.SAN, "c2")
	}
	if out.OFEN == startOFEN || out.OFEN == "" {
		t.Errorf("ofen = %q, want an advanced position", out.OFEN)
	}
	// black to move now
	if len(out.Dests) == 0 || len(out.Dests["b4"]) == 0 {
		t.Errorf("dests = %v, want black's legal moves incl. b4", out.Dests)
	}
}

// TestAnalysisRejections covers the failure modes: malformed request, invalid
// position, malformed and illegal moves.
func TestAnalysisRejections(t *testing.T) {
	app := analysisTestApp()
	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty body", `{}`, fiber.StatusBadRequest},
		{"garbage ofen", `{"ofen":"not-a-position"}`, fiber.StatusUnprocessableEntity},
		{"malformed uoi", `{"ofen":"` + startOFEN + `","uoi":"e2e4"}`, fiber.StatusUnprocessableEntity},
		{"illegal move", `{"ofen":"` + startOFEN + `","uoi":"a1a2"}`, fiber.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		if status, _ := postAnalysis(t, app, tc.body); status != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, status, tc.want)
		}
	}
}
