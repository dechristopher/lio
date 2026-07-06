package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/view"
)

// NewsHandler renders the paginated news page, returning just the swappable
// #news-content fragment for htmx pager requests and the full page otherwise.
// The page number is 1-based; view.News/NewsContent (via news.Paginate) clamp
// out-of-range values, so a missing or garbage ?page is safe.
func NewsHandler(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page"))
	if view.IsHTMXFragment(c) {
		return view.Render(c, 200, view.NewsContent(page))
	}
	return view.Render(c, 200, view.News(view.PageMeta("News"), page))
}
