package ws

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// checkOrigin runs okOrigin against a request carrying the given Origin
// header (empty = no Origin header, like a non-browser client).
func checkOrigin(t *testing.T, origin string) bool {
	t.Helper()
	app := fiber.New()
	var got bool
	app.Get("/socket/tv", func(c fiber.Ctx) error {
		got = okOrigin(c)
		return nil
	})
	req := httptest.NewRequest("GET", "/socket/tv", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if _, err := app.Test(req); err != nil {
		t.Fatal(err)
	}
	return got
}

// TestOkOrigin pins the upgrade origin policy: outside production every
// origin is trusted (LAN devices, tunnels, and test harnesses reach non-prod
// servers from origins no allowlist can anticipate); production allows exact
// matches against the configured origin list only — no substring near-misses
// — with absent Origin allowed (non-browser clients).
func TestOkOrigin(t *testing.T) {
	t.Run("dev allows all", func(t *testing.T) {
		t.Setenv("DEPLOY", "dev")
		for _, origin := range []string{
			"",
			"https://dev.lioctad.org",
			"http://192.168.1.50:4444", // LAN device against the dev env
			"https://evil.example",
		} {
			if !checkOrigin(t, origin) {
				t.Errorf("okOrigin(%q) = false, want true in dev env", origin)
			}
		}
	})

	t.Run("prod", func(t *testing.T) {
		t.Setenv("DEPLOY", "prod")
		cases := []struct {
			origin string
			want   bool
		}{
			{"https://lioctad.org", true},
			{"https://lioctad.or", false}, // registerable near-miss, substring of the real origin
			{"http://localhost:4444", false},
		}
		for _, tc := range cases {
			if got := checkOrigin(t, tc.origin); got != tc.want {
				t.Errorf("okOrigin(%q) = %t, want %t", tc.origin, got, tc.want)
			}
		}
	})

	t.Run("local", func(t *testing.T) {
		// DEPLOY unset = local: every origin is trusted
		for _, origin := range []string{"", "http://192.168.1.50:4444", "https://evil.example"} {
			if !checkOrigin(t, origin) {
				t.Errorf("okOrigin(%q) = false, want true in local env", origin)
			}
		}
	})
}
