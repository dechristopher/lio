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
		Title: "Games with your name on them",
		Date:  "Jul 20, 2026",
		Body: "You can now sign up and play under a username, which shows on your clocks, match " +
			"timelines, and shared game links. A new profile menu keeps your account in one place — " +
			"change your password, and review or sign out the devices you're logged in on. " +
			"Anonymous play is unchanged; an account is entirely optional.",
	},
	{
		Title: "It's that time already?",
		Date:  "Jul 19, 2026",
		Body: "Finished games can now be replayed at their original pace — a new play button in the " +
			"archive steps through the moves with the real timing between them, and the move list " +
			"shows how long each move took.",
	},
	{
		Title: "Matches that outlive the room",
		Date:  "Jul 19, 2026",
		Body: "Game links no longer go dead when a room closes — every finished match lives on at " +
			"its original link, ready to replay in full, and each game of a match gets its own " +
			"permanent address to share.",
	},
	{
		Title: "A timeline you can click through",
		Date:  "Jul 19, 2026",
		Body: "The match score timeline got a clean new look — one compact score column per game, " +
			"showing who had which color and how each game was decided. It's clickable now, too: " +
			"once a game ends, tap any earlier game to replay it right there on the board while " +
			"the rematch clock keeps ticking, then tap the latest game to jump back. Archived " +
			"matches link between their games the same way.",
	},
	{
		Title: "Quick games go blind",
		Date:  "Jul 18, 2026",
		Body: "The quick-game buttons now deal deploy games by default: arrange your home rank in " +
			"secret, then meet your opponent's setup at the reveal. Classic starts are still a " +
			"click away in the custom game builder. Also fixed a brief flash of the standard " +
			"starting position before the blind setup covered the board.",
	},
	{
		Title: "The reveal gets a countdown",
		Date:  "Jul 17, 2026",
		Body: "In deploy games, a ten-second countdown now rings the center of the board the " +
			"moment both secret arrangements are revealed. Take it in, then make your first move " +
			"whenever you're ready — or let the timer lapse and white's clock starts on its own. " +
			"The computer holds its opening move for a beat too, so the reveal is never a blur. " +
			"And a player who never moves now loses on time like anyone else, instead of the " +
			"game quietly closing.",
	},
	{
		Title: "Sound for spectators",
		Date:  "Jul 17, 2026",
		Body: "Browsers keep a page silent until it's been tapped at least once — so watching a " +
			"game from a fresh tab used to mean no sounds at all, with no hint why. Spectators " +
			"now see a small muted icon mid-board while sound is locked: tap it (or anywhere) " +
			"and the game comes to life.",
	},
	{
		Title: "Little touches",
		Date:  "Jul 17, 2026",
		Body: "The preferences menu now highlights your chosen theme — light, dark, or system — " +
			"in your accent color, matching the board and piece pickers. The home page counts " +
			"every game ever played on the site. A clock that runs out now reads an honest " +
			"0:00.0. And gradient backgrounds on Android lost their color banding.",
	},
	{
		Title: "Your games, on the record",
		Date:  "Jul 14, 2026",
		Body: "Under the hood, every finished game is now recorded to a durable database, each " +
			"move and position preserved alongside the result. It's the groundwork for game " +
			"history, personal stats, and post-game analysis on the way.",
	},
	{
		Title: "Sounds you won't miss",
		Date:  "Jul 12, 2026",
		Body: "Move sounds now play reliably across phones and older browsers that used to fall " +
			"silent mid-game. And draw offers and rematch requests each get their own soft chime " +
			"heard by both players so a proposal never slips by unnoticed.",
	},
	{
		Title: "Games that outlive the server",
		Date:  "Jul 12, 2026",
		Body: "Server updates no longer end your game: live games are now preserved through " +
			"restarts, and your board reconnects on its own with a brief \"updating\" status. " +
			"When a new version ships, a small notice offers a refresh on your schedule, " +
			"not ours.",
	},
	{
		Title: "Who's ahead, at a glance",
		Date:  "Jul 11, 2026",
		Body: "Captured pieces now stack up beside each player's clock with a running point " +
			"score. The icons follow the position you're viewing, so stepping back through " +
			"the moves replays the material story too.",
	},
	{
		Title: "Take all the time you want",
		Date:  "Jul 11, 2026",
		Body: "Casual mode now drops the clock entirely: play untimed games against the " +
			"computer or a friend and think as long as you like. For those who do want a " +
			"ticking clock, a 3+5 rapid control joins the lineup.",
	},
	{
		Title: "Game links that show the game",
		Date:  "Jul 11, 2026",
		Body: "Share a lioctad.org link and it now unfurls (in most link previews) with a real " +
			"picture of the board alongside game info.",
	},
	{
		Title: "Snappier page loads",
		Date:  "Jul 10, 2026",
		Body: "Pages load quicker and lighter: scripts no longer hold up the first render, the " +
			"site's fonts and all eight board themes now arrive in far fewer requests, and tabs " +
			"left in the background stop refreshing on their own to spare your battery and data.",
	},
	{
		Title: "Sound, ready the moment you need it",
		Date:  "Jul 10, 2026",
		Body: "Move, capture, and check sounds are now served straight from lioctad and loaded " +
			"ahead of time, so audio is primed to play the instant it's needed. No waiting on an " +
			"CDN or outside service.",
	},
	{
		Title: "A more secure lioctad",
		Date:  "Jul 10, 2026",
		Body: "Under the hood, your session is now sealed with tamper-proof encryption, and the " +
			"site gained browser-level protections against clickjacking and other cross-site " +
			"tricks, plus rate limits that keep abuse in check.",
	},
	{
		Title: "Spectating, straightened out",
		Date:  "Jul 9, 2026",
		Body: "Watching a match no longer plays musical chairs: each player keeps their side " +
			"of the board and scoreboard for the whole match while the colors swap between " +
			"games, and every clock now wears a stripe showing who has white and black — " +
			"for players too. The score timeline also scrolls as one.",
	},
	{
		Title: "Live boards with more context",
		Date:  "Jul 9, 2026",
		Body: "The home page's live mini-boards now name each game's time control and mode " +
			"and show how many people are watching, so you can pick the best action before " +
			"clicking in.",
	},
	{
		Title: "Castling, demonstrated",
		Date:  "Jul 9, 2026",
		Body: "The about page got an overhaul: octad's three castle types now play out on " +
			"looping demo boards, so the swap-and-cross mechanics can be watched instead " +
			"of deciphered from notation.",
	},
	{
		Title: "Polish across the board",
		Date:  "Jul 8, 2026",
		Body: "Game results now land with an animated result card, the move list keeps the " +
			"latest move in view, and the home page fits phones better with a full-width " +
			"demo board.",
	},
	{
		Title: "The engine finishes the job",
		Date:  "Jul 8, 2026",
		Body: "The engine now sees threefold repetition coming: it converts winning endgames " +
			"instead of shuffling into a draw, and steers toward repetition when it's the " +
			"one behind.",
	},
	{
		Title: "Take your games with you",
		Date:  "Jul 8, 2026",
		Body: "A new copy button in analysis mode puts the full PGN of your game on the " +
			"clipboard, ready to share or study elsewhere.",
	},
	{
		Title: "Rock-solid connections",
		Date:  "Jul 8, 2026",
		Body: "Squashed a long-standing bug that could silently drop moves mid-game — most " +
			"often on iPhones — alongside smoother reconnects and game-state re-sync.",
	},
	{
		Title: "The board plays itself",
		Date:  "Jul 7, 2026",
		Body: "New to Octad? The home page now features a self-playing demo board that runs " +
			"through full games so you can see the variant in motion.",
	},
	{
		Title: "Make the board yours",
		Date:  "Jul 6, 2026",
		Body: "Pick from eight board themes and three piece sets in settings — and your board " +
			"choice retints the whole site's accent color to match.",
	},
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

	// ---- the historical record: entries below were written retroactively from
	// git history when the news feed was introduced in 2026 ----
	{
		Title: "A stir in hibernation",
		Date:  "Feb 16, 2024",
		Body: "A brief awakening between eras: fixed an engine bug that could skew its move " +
			"selection, and brought the whole stack up to Go 1.22 before the long nap resumed.",
	},
	{
		Title: "Sounds and small touches",
		Date:  "Sep 17, 2022",
		Body: "A check sound effect joins the move and capture sounds, and colorful pawn icons " +
			"stand in as placeholder profile pictures.",
	},
	{
		Title: "Proper waiting rooms",
		Date:  "Sep 9, 2022",
		Body: "Creating a challenge now opens a real waiting room with a cancel button, and " +
			"shared game links unfurl into rich previews in chat apps.",
	},
	{
		Title: "Custom challenges",
		Date:  "Aug 20, 2022",
		Body: "A new create-a-game dialog lets you set up the exact game you want: pick the " +
			"time control and choose which color you'll play.",
	},
	{
		Title: "Clocks worth trusting",
		Date:  "Aug 18, 2022",
		Body: "Rebuilt game clocks tick smoothly, compensate for network lag on every move, and " +
			"carry the running match score. Rematches landed too — same room, same opponent, " +
			"swapped colors.",
	},
	{
		Title: "The multiplayer update",
		Date:  "Aug 11, 2022",
		Body: "The biggest release yet: create a room, share the link, and play a friend live. " +
			"A new homepage, proper game rooms, and an asynchronous engine dispatcher came along " +
			"with it.",
	},
	{
		Title: "Back from a lull",
		Date:  "Jul 6, 2022",
		Body: "Development picks back up with a real server-side game clock implementation and " +
			"an engine that sizes its search depth more intelligently.",
	},
	{
		Title: "The engine gets serious",
		Date:  "Nov 5, 2021",
		Body: "The computer now searches all of its candidate moves in parallel, looks seven " +
			"plies ahead, and factors checks into its evaluation — a meaningfully stronger " +
			"opponent.",
	},
	{
		Title: "The engine opens its eyes",
		Date:  "Mar 11, 2021",
		Body: "The computer stops playing randomly: a minimax search with alpha-beta pruning and " +
			"hand-tuned piece-square tables now picks its moves.",
	},
	{
		Title: "Every game on the record",
		Date:  "Mar 7, 2021",
		Body: "Finished games are now archived as PGNs in object storage, laying the groundwork " +
			"for a public game database.",
	},
	{
		Title: "First moves",
		Date:  "Mar 1, 2021",
		Body: "The first playable games: a live board over websockets against a random-moving " +
			"opponent, complete with move and capture sounds and check highlighting.",
	},
	{
		Title: "Hello, world",
		Date:  "Feb 21, 2021",
		Body: "lioctad.org is born: a fresh Go server, the first pages, and a mission: a free, " +
			"libre home for octad, the 4x4 chess variant, in the spirit of lichess.",
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
