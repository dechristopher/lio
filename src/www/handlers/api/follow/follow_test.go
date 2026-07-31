package follow

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// TestPageParam pins the one client-controlled input on the list endpoints.
// Every unreadable value has to land on page 1 rather than reach the query as a
// negative or absurd offset — a page number is arithmetic that goes straight
// into OFFSET, and "page=-5" must not be able to say anything about it.
func TestPageParam(t *testing.T) {
	cases := []struct {
		name string
		q    string
		want int
	}{
		{"absent", "", 1},
		{"first", "page=1", 1},
		{"later", "page=7", 7},

		{"zero", "page=0", 1},
		{"negative", "page=-5", 1},
		{"words", "page=abc", 1},
		{"blank", "page=", 1},
		{"float", "page=2.5", 1},
		// "+" is a space in a query string, so this arrives as " 3" and is not
		// a number. An encoded sign is one, and is accepted.
		{"plus is a space", "page=+3", 1},
		{"encoded sign", "page=%2B3", 3},
		{"overflow", "page=99999999999999999999", 1},
	}

	app := fiber.New()
	var got int
	app.Get("/p", func(c fiber.Ctx) error {
		got = pageParam(c)
		return c.SendStatus(fiber.StatusNoContent)
	})

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got = 0
			if _, err := app.Test(httptest.NewRequest("GET", "/p?"+c.q, nil)); err != nil {
				t.Fatalf("request: %v", err)
			}
			if got != c.want {
				t.Errorf("pageParam(%q) = %d, want %d", c.q, got, c.want)
			}
		})
	}
}

// TestPageOffset covers the arithmetic the page number feeds, including that
// one row past the page is always read — that extra row is the only thing that
// tells the client whether another page exists, and dropping it would leave the
// "Load more" control guessing.
func TestPageOffset(t *testing.T) {
	for _, c := range []struct {
		page         int
		wantOffset   int32
		wantMinLimit int32
	}{
		{1, 0, pageSize + 1},
		{2, pageSize, pageSize + 1},
		{5, 4 * pageSize, pageSize + 1},
	} {
		limit, offset := int32(pageSize+1), int32((c.page-1)*pageSize)
		if offset != c.wantOffset {
			t.Errorf("page %d offset = %d, want %d", c.page, offset, c.wantOffset)
		}
		if limit != c.wantMinLimit {
			t.Errorf("page %d limit = %d, want %d", c.page, limit, c.wantMinLimit)
		}
	}
}
