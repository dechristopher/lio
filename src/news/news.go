// Package news holds the site's hand-authored news/changelog feed. Entries are
// a per-process constant curated from notable releases; both the home-page news
// block (top few) and the dedicated /news page (all, paginated) render from the
// same source, so adding an entry here surfaces it in both places.
package news

// Item is a single news/changelog entry. Body is a short plain-text blurb; Date
// is a pre-formatted human display string (e.g. "Jul 5, 2026").
type Item struct {
	Title string
	Date  string
	Body  string
}

// Items is the curated news feed, ordered newest-first. Keep new entries at the
// top. Blurbs are user-facing, so describe the change in terms of what players
// can see or do — not the commit that shipped it.
var Items = []Item{
	{
		Title: "Match play & race to a target",
		Date:  "Jul 5, 2026",
		Body: "Play a series instead of a single game: race to a set number of points and " +
			"follow the match on a live score timeline that stays in sync from board to board.",
	},
	{
		Title: "Sharper post-game analysis",
		Date:  "Jul 4, 2026",
		Body: "The analysis board got a UI pass with larger move controls and clearer endgame " +
			"annotations, plus a fix for board spacing on narrow screens.",
	},
	{
		Title: "See who's around",
		Date:  "Jul 3, 2026",
		Body: "Live player presence and site stats show how many people are online, how many " +
			"games are in progress, and how many seeks are open right now.",
	},
	{
		Title: "Spectator mode overhaul",
		Date:  "Jul 2, 2026",
		Body: "Watching games got a dedicated redesign, joined by live game cards and a running " +
			"live-games count on the home page.",
	},
	{
		Title: "Smarter engine timing",
		Date:  "Jul 2, 2026",
		Body: "The engine now budgets its thinking time against the clock and caches its deploy " +
			"selection, so bot games stay responsive and no longer flag under time pressure.",
	},
	{
		Title: "Deploy: a new way to start",
		Date:  "Jul 1, 2026",
		Body: "A new deploy game mode lets you set up your pieces before the first move, shipped " +
			"alongside WebSocket hardening and rematch sync fixes.",
	},
	{
		Title: "A new custom game builder",
		Date:  "Jul 1, 2026",
		Body: "Create exactly the game you want with a refreshed custom-challenge flow, plus a " +
			"round of concurrency fixes for snappier, more reliable rooms.",
	},
	{
		Title: "Rooms, redesigned",
		Date:  "Jun 26, 2026",
		Body: "Overhauled room design with shareable QR codes for inviting a friend and a smoother " +
			"human rematch experience.",
	},
	{
		Title: "A fresh new look",
		Date:  "Jun 25, 2026",
		Body: "A unified new design language rolled out across the whole site, from the home page " +
			"to the board.",
	},
	{
		Title: "Server-rendered with templ + HTMX",
		Date:  "Jun 24, 2026",
		Body: "Page rendering moved to a-h/templ with HTMX for dynamic updates, dropping the heavy " +
			"single-page app for faster loads.",
	},
	{
		Title: "Engine variety & a thinking indicator",
		Date:  "Jun 24, 2026",
		Body: "The engine now varies its opening moves for more interesting games and shows a live " +
			"thinking indicator while it's on the move.",
	},
	{
		Title: "Engine evaluation fixes",
		Date:  "Jun 23, 2026",
		Body: "Corrected the engine's position evaluation and negamax search, backed by a new " +
			"regression test suite so bot play stays sound.",
	},
}

// PerPage is the number of items shown per page on the /news page.
const PerPage = 6

// Latest returns the n most-recent items (fewer if the feed is shorter, none for
// n <= 0). The returned slice shares backing storage with Items and must not be
// mutated.
func Latest(n int) []Item {
	if n < 0 {
		n = 0
	}
	if n > len(Items) {
		n = len(Items)
	}
	return Items[:n]
}

// Page is one page of the paginated news feed plus the pager metadata the view
// needs to render prev/next controls.
type Page struct {
	Items   []Item
	Number  int // 1-based current page
	Total   int // total number of pages (always >= 1)
	HasPrev bool
	HasNext bool
	Prev    int // page number for the "newer" link (valid when HasPrev)
	Next    int // page number for the "older" link (valid when HasNext)
}

// Paginate returns the requested page of Items. page is 1-based and clamped into
// the valid [1, Total] range, so out-of-range or zero input is safe.
func Paginate(page int) Page {
	total := (len(Items) + PerPage - 1) / PerPage
	if total < 1 {
		total = 1
	}
	if page < 1 {
		page = 1
	}
	if page > total {
		page = total
	}

	start := (page - 1) * PerPage
	end := start + PerPage
	if end > len(Items) {
		end = len(Items)
	}
	var items []Item
	if start < end {
		items = Items[start:end]
	}

	return Page{
		Items:   items,
		Number:  page,
		Total:   total,
		HasPrev: page > 1,
		HasNext: page < total,
		Prev:    page - 1,
		Next:    page + 1,
	}
}
