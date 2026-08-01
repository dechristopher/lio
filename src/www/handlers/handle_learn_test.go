package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/bus"
	"github.com/dechristopher/lio/learn"
)

// learnTestApp registers the tutorial routes as www.Serve does, minus the rate
// limiter — these tests exercise the handlers. The graduation lesson's replies
// run an engine search, which publishes to the in-process bus and blocks until
// it is up, so bring it up here (the established pattern, see
// handle_analysis_test.go and room/main_test.go).
func learnTestApp() *fiber.App {
	bus.Up()
	app := fiber.New()
	app.Get("/learn", LearnHandler)
	app.Get("/learn/:slug", LearnLessonHandler)
	app.Post("/api/learn", LearnAPIHandler)
	return app
}

// postLearn runs one request through the tutorial API. The timeout is generous
// enough for the graduation game's budgeted engine reply.
func postLearn(t *testing.T, app *fiber.App, body string) (int, learn.Response) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/learn", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("learn request: %v", err)
	}
	var out learn.Response
	if resp.StatusCode == fiber.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return resp.StatusCode, out
}

// TestLearnAPIDescribe covers the call the client makes on entering a step: no
// move, just the position and the destinations the board needs to accept input.
func TestLearnAPIDescribe(t *testing.T) {
	app := learnTestApp()
	status, out := postLearn(t, app, `{"lesson":"pieces","step":2}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if out.Move != nil {
		t.Error("describe-only response carries a move")
	}
	if len(out.Dests) == 0 {
		t.Error("no legal destinations returned")
	}
	if out.Turn != "white" {
		t.Errorf("turn = %q, want white", out.Turn)
	}
}

// TestLearnAPISolvesAStep covers the ordinary path end to end: the pawn-capture
// step, answered correctly, comes back completed with the coach's line and the
// capture flagged for the client's sound.
func TestLearnAPISolvesAStep(t *testing.T) {
	app := learnTestApp()
	lesson, _ := learn.BySlug("capture")
	body := `{"lesson":"capture","step":0,"ofen":"` + lesson.Steps[0].Start() + `","uoi":"c2b3"}`
	status, out := postLearn(t, app, body)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !out.Done {
		t.Fatal("the correct capture did not complete the step")
	}
	if out.Move == nil || !out.Move.Capture {
		t.Error("the capture was not flagged on the move")
	}
	if out.Say == "" {
		t.Error("a completed step said nothing")
	}
}

// TestLearnAPIRejectsBadInput covers the guards: a request has to name a real
// lesson and step, and a move has to be legal in the position given.
func TestLearnAPIRejectsBadInput(t *testing.T) {
	app := learnTestApp()
	cases := map[string]string{
		"unknown lesson":    `{"lesson":"nope"}`,
		"step out of range": `{"lesson":"board","step":99}`,
		"illegal move":      `{"lesson":"pieces","step":2,"uoi":"a1a4"}`,
		"malformed ofen":    `{"lesson":"pieces","step":2,"ofen":"garbage"}`,
		"bad deploy":        `{"lesson":"deploy","deploy":"kkkk"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			status, _ := postLearn(t, app, body)
			if status != fiber.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", status)
			}
		})
	}

	t.Run("oversized field", func(t *testing.T) {
		long := make([]byte, 200)
		for i := range long {
			long[i] = 'a'
		}
		status, _ := postLearn(t, app, `{"lesson":"board","ofen":"`+string(long)+`"}`)
		if status != fiber.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
	})
}

// TestLearnAPIGraduationGameReplies covers the one lesson that uses the engine:
// a move in the graduation game must come back with the bot's answer, so the
// game is actually playable.
func TestLearnAPIGraduationGameReplies(t *testing.T) {
	app := learnTestApp()
	lesson, _ := learn.BySlug("play")
	body := `{"lesson":"play","step":0,"ofen":"` + lesson.Steps[0].Start() + `","uoi":"c1c2"}`
	status, out := postLearn(t, app, body)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if out.Move == nil {
		t.Fatal("the learner's move is missing from the response")
	}
	if out.Reply == nil {
		t.Fatal("the bot did not reply in the graduation game")
	}
	if out.Turn != "white" {
		t.Errorf("turn = %q after the bot's reply, want white", out.Turn)
	}
	if len(out.Dests) == 0 {
		t.Error("no destinations to continue the game with")
	}
}

// TestLearnPageRoutes covers the page routes: the course start, a deep link,
// and an unknown lesson falling back to the start rather than a 404.
func TestLearnPageRoutes(t *testing.T) {
	app := learnTestApp()
	cases := []struct {
		path string
		want int
	}{
		{"/learn", fiber.StatusOK},
		{"/learn/castling", fiber.StatusOK},
		{"/learn/not-a-lesson", fiber.StatusFound},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", tc.path, nil),
				fiber.TestConfig{Timeout: 10 * time.Second})
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
