package card

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/view"
)

// rated builds a rating tile view for a real category key, so the canonical
// ordering under test is the same lookup the profile page uses.
func rated(category string, games int) view.RatingView {
	return view.NewRatingView(category, "1500", games)
}

// The card leads with what somebody actually plays, but must not reorder itself
// as they play: selection is by volume, display is canonical.
func TestPickRatingsTakesMostPlayedAndOrdersCanonically(t *testing.T) {
	rows := pickRatings([]view.RatingView{
		rated("three-five-rapid-deploy", 5),
		rated("quarter-zero-bullet-deploy", 40),
		rated("one-two-rapid-deploy", 20),
		rated("half-one-blitz-deploy", 30),
	})

	if len(rows) != ratingsShown {
		t.Fatalf("expected %d tiles, got %d", ratingsShown, len(rows))
	}
	// the 5-game category is the one dropped
	for _, r := range rows {
		if r.Games == 5 {
			t.Fatalf("least-played category should have been dropped: %#v", rows)
		}
	}
	// and what survives reads bullet -> blitz -> rapid, not 40/30/20
	want := []int{40, 30, 20}
	got := []int{rows[0].Games, rows[1].Games, rows[2].Games}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tiles out of canonical order: got %v, want %v", got, want)
		}
	}
}

func TestPickRatingsKeepsEverythingUnderTheLimit(t *testing.T) {
	rows := pickRatings([]view.RatingView{
		rated("one-two-rapid-deploy", 3),
		rated("half-one-blitz-deploy", 9),
	})
	if len(rows) != 2 {
		t.Fatalf("expected both tiles, got %d", len(rows))
	}
}

func TestPickRatingsOnNothing(t *testing.T) {
	if rows := pickRatings(nil); len(rows) != 0 {
		t.Fatalf("expected no tiles, got %#v", rows)
	}
}

// The status line answers the most specific true thing first. Playing outranks
// merely holding a seat, which outranks merely being connected.
func TestStatusLinePrecedence(t *testing.T) {
	cases := []struct {
		name                  string
		playing, online, busy bool
		want                  string
	}{
		{"at a board", true, true, true, "Playing now"},
		{"playing wins even if presence lags", true, false, false, "Playing now"},
		{"seated but not started", false, true, true, "Waiting for a game"},
		{"just here", false, true, false, "Online"},
		{"away with no stamp", false, false, false, "Offline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// user id 0 is never online and has no departure stamp, so the
			// offline branch falls through to its durable answer
			if got := statusLine(tc.playing, tc.online, 0, tc.busy); got != tc.want {
				t.Fatalf("statusLine = %q, want %q", got, tc.want)
			}
		})
	}
}

// An unknown or empty name is a 404, which the client renders as nothing at all
// — the username simply stays a plain link. This is also the whole response on
// a build with no database, which is what a bare checkout runs.
func TestHandlerRejectsAnEmptyName(t *testing.T) {
	app := fiber.New()
	Wire(app.Group("/api/card"))

	// a trailing-slash request never binds :username, so it 404s at the router;
	// the handler's own guard is exercised by the space-only name below
	req := httptest.NewRequest("GET", "/api/card/%20", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusNotFound)
	}
}
