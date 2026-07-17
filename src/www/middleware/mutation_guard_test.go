package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// guardStatus runs a POST through MutationGuard with the given headers and
// returns the response status (200 = passed through to the handler).
func guardStatus(t *testing.T, origin, secFetchSite string) int {
	t.Helper()
	app := fiber.New()
	app.Use(MutationGuard())
	app.Post("/new/game", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest("POST", "/new/game", nil)
	if origin != "" {
		req.Header.Set(fiber.HeaderOrigin, origin)
	}
	if secFetchSite != "" {
		req.Header.Set(fiber.HeaderSecFetchSite, secFetchSite)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

// TestMutationGuard pins the CSRF-guard policy: outside production the guard
// stands down entirely (LAN devices, tunnels, and test harnesses mutate from
// origins no allowlist can anticipate), while production rejects explicit
// cross-site mutations and unknown Origins, allowing same-origin/same-site/
// user-initiated requests and non-browser clients (no Origin, no Sec-Fetch-Site).
func TestMutationGuard(t *testing.T) {
	t.Run("local allows all", func(t *testing.T) {
		// DEPLOY unset = local
		for _, tc := range [][2]string{
			{"", ""},
			{"http://192.168.1.50:4444", ""},       // LAN device, older browser
			{"https://evil.example", "cross-site"}, // even hostile passes off-prod
			{"http://localhost:5555", "same-origin"},
		} {
			if got := guardStatus(t, tc[0], tc[1]); got != fiber.StatusOK {
				t.Errorf("local: origin=%q sfs=%q -> %d, want 200", tc[0], tc[1], got)
			}
		}
	})

	t.Run("dev allows all", func(t *testing.T) {
		t.Setenv("DEPLOY", "dev")
		if got := guardStatus(t, "http://192.168.1.50:4444", ""); got != fiber.StatusOK {
			t.Errorf("dev: LAN origin -> %d, want 200", got)
		}
		if got := guardStatus(t, "https://evil.example", "cross-site"); got != fiber.StatusOK {
			t.Errorf("dev: cross-site -> %d, want 200", got)
		}
	})

	t.Run("prod", func(t *testing.T) {
		t.Setenv("DEPLOY", "prod")
		cases := []struct {
			origin, sfs string
			want        int
		}{
			{"", "same-origin", fiber.StatusOK},
			{"", "same-site", fiber.StatusOK},
			{"", "none", fiber.StatusOK}, // user-initiated (address bar, bookmark)
			{"", "cross-site", fiber.StatusForbidden},
			{"https://lioctad.org", "", fiber.StatusOK},         // allowlisted Origin fallback
			{"https://evil.example", "", fiber.StatusForbidden}, // unknown Origin fallback
			{"", "", fiber.StatusOK},                            // non-browser client
		}
		for _, tc := range cases {
			if got := guardStatus(t, tc.origin, tc.sfs); got != tc.want {
				t.Errorf("prod: origin=%q sfs=%q -> %d, want %d", tc.origin, tc.sfs, got, tc.want)
			}
		}
	})
}
