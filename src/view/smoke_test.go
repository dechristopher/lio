package view

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/news"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/settings"
	"github.com/dechristopher/lio/title"
	"github.com/dechristopher/lio/variant"
)

func renderSmoke(t *testing.T, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := c.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// renderSmokeViewer renders with an explicit Viewer in the context, the way
// view.Render injects the request identity.
func renderSmokeViewer(t *testing.T, v Viewer, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	ctx := context.WithValue(context.Background(), viewerKey{}, v)
	if err := c.Render(ctx, &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("output missing %q", sub)
	}
}

func mustNotContain(t *testing.T, s, sub string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Errorf("output must not contain %q", sub)
	}
}

func TestRenderIndex(t *testing.T) {
	challenges := []message.OpenChallenge{{RoomID: "seek456", Variant: variant.OneTwoRapid, Color: "w"}}
	stats := message.SiteStats{LiveGames: 1, OpenChallenges: 1, Playing: 2}
	out := renderSmoke(t, Index(PageMeta("Free Online Octad"), challenges, stats))
	mustContain(t, out, "<title>octad.gg • Free Online Octad</title>")
	mustContain(t, out, "Quick game")            // home heading (uppercased via CSS)
	mustContain(t, out, `id="createGameButton"`) // modal opener
	mustContain(t, out, `id="modalCreateGame"`)
	mustContain(t, out, "getElementById(\"modalCreateGame\")") // inline modal script

	// new home sections
	mustContain(t, out, `id="home-activity"`)      // polled activity region
	mustContain(t, out, `hx-get="/home/activity"`) // self-poll
	mustContain(t, out, "Open challenges")         // challenges section
	mustContain(t, out, "/seek456")                // joinable challenge link
	mustContain(t, out, "What is Octad?")          // explainer
	// zero Viewer (no session, accounts disabled): the login button renders
	// disabled and the auth modal is omitted entirely
	mustContain(t, out, ">Log in<")
	mustNotContain(t, out, `id="modalAccount"`)
	mustNotContain(t, out, `id="profilePopover"`)

	// "What is Octad?" self-playing demo board: the octadground mount + result
	// pill and its animator script, replacing the old static diagram SVGs
	mustContain(t, out, `id="home-demo-board"`)   // demo board mount
	mustContain(t, out, `id="home-demo-overlay"`) // result pill (reuses .end-annotation)
	mustContain(t, out, "lio-home-demo")          // demo animator script
	mustContain(t, out, "/about/rules")           // learn-more buttons kept
	mustNotContain(t, out, "octad2.svg")          // static diagrams removed
	mustNotContain(t, out, "far-castle.svg")

	// news block: the three newest feed entries plus the link to the full page,
	// and no lingering "alpha" tag in the box (titles are html-escaped in output)
	mustContain(t, out, templ.EscapeString(news.Items[0].Title))    // newest entry rendered
	mustContain(t, out, "All news →")                               // link out to /news
	mustNotContain(t, out, templ.EscapeString(news.Items[3].Title)) // only the top three, not the fourth

	// live-games TV widget: static shell + streaming client (boards stream in)
	mustContain(t, out, `id="tv-widget"`)       // TV card
	mustContain(t, out, `id="tv-grid"`)         // JS-populated grid mount
	mustContain(t, out, "No games in progress") // empty state
	mustContain(t, out, "lio-tv")               // scriptsTV client
	mustContain(t, out, "octadground")          // scriptsTV board renderer

	// create-game modal: opponent toggle, unified POST target, and the hidden
	// field the resolved variant is written into. There is no mode toggle — every
	// game is the blind-deploy variant ("Octad" on the surface).
	mustContain(t, out, `action="/new/game"`)
	mustContain(t, out, `name="opponent" value="human"`)
	mustContain(t, out, `name="opponent" value="computer"`)
	mustContain(t, out, `id="cg-variant"`)
	mustNotContain(t, out, `name="mode"`) // the Classic/Deploy toggle is gone

	// each time-control card carries its (deploy) variant name; order is
	// bullet->blitz->rapid
	order := []string{
		"quarter-zero-bullet-deploy", "half-one-blitz-deploy", "one-two-rapid-deploy",
	}
	for _, name := range order {
		mustContain(t, out, name)
	}
	// bullet card precedes rapid card (data-variant attribute order)
	if strings.Index(out, `data-variant="quarter-zero-bullet-deploy"`) > strings.Index(out, `data-variant="one-two-rapid-deploy"`) {
		t.Error("time-control cards out of order (bullet should precede rapid)")
	}
	// the rated status badge is gated on accounts being enabled; the smoke
	// viewer is anonymous with accounts disabled, so it must not render (and the
	// dangling "Log in" link is never emitted)
	mustNotContain(t, out, `class="cg-rated`)
}

func TestRenderHomeActivityEmpty(t *testing.T) {
	out := renderSmoke(t, HomeActivity(nil, message.SiteStats{}))
	mustContain(t, out, `id="home-activity"`)
	mustContain(t, out, "No open challenges right now")
}

func TestRenderRoomGame(t *testing.T) {
	p := message.RoomTemplatePayload{
		RoomID:        "abc",
		PlayerColor:   "w",
		OpponentColor: "b",
		OpponentIsBot: true,
		BlackIsBot:    true,
		VariantName:   "Half One blitz",
		Variant:       variant.HalfOneBlitz,
	}
	out := renderSmoke(t, Room(RoomMeta(p), p))
	mustContain(t, out, `class="game-room`) // outer game container
	mustContain(t, out, `class="game-grid"`)
	mustContain(t, out, "Half One blitz") // variant/time-control shown in the rail
	mustContain(t, out, `data-bot="true"`)
	mustContain(t, out, "octadground")                     // scriptsRoom loaded
	mustContain(t, out, `id="game"`)                       // board mount
	mustContain(t, out, "Challenge from anonymous player") // room title meta

	// a player's page is not the watch-only variant: interactive controls with
	// their real action tooltips, no spectator flag on the board container
	mustContain(t, out, `data-spectator="false"`)
	mustContain(t, out, `id="btn-resign" class="ctrl-btn play-ctrl" title="Resign the game">`)
	mustContain(t, out, `id="btn-draw" class="ctrl-btn play-ctrl" title="Offer a draw">`)
	mustNotContain(t, out, "Watching as a spectator")

	// the live board ships the eval bar hidden; lio-game.js reveals it only
	// after a bot game ends and analysis begins (requestLiveEvals)
	mustContain(t, out, `id="eval-bar"`)

	// the copy-PGN button carries this room's PGN Event name, so the client's
	// fallback PGN names the situation the same way the archived one does
	mustContain(t, out, `data-event="Unrated Blitz game vs Computer"`)
}

// TestRenderRoomAnonCta locks the anonymous "create account" shim: it renders
// for an anonymous viewer only when accounts are available, carries the
// register + dismiss hooks and rated/username copy, and is absent for a
// logged-in viewer or when accounts are disabled.
func TestRenderRoomAnonCta(t *testing.T) {
	p := message.RoomTemplatePayload{
		RoomID:        "abc",
		PlayerColor:   "w",
		OpponentColor: "b",
		OpponentIsBot: true,
		VariantName:   "Half One blitz",
		Variant:       variant.HalfOneBlitz,
	}
	page := Room(RoomMeta(p), p)

	anon := renderSmokeViewer(t, Viewer{UID: "u", AccountsEnabled: true}, page)
	mustContain(t, anon, `id="roomCta"`)
	mustContain(t, anon, `id="roomCtaCreate"`)
	mustContain(t, anon, `id="roomCtaDismiss"`)
	mustContain(t, anon, "username")
	mustContain(t, anon, "rated")

	loggedIn := renderSmokeViewer(t,
		Viewer{UID: "u", LoggedIn: true, Username: "drew", AccountsEnabled: true}, page)
	mustNotContain(t, loggedIn, `id="roomCta"`)

	// accounts unavailable (PG-less dev): no shim even for an anonymous viewer
	disabled := renderSmokeViewer(t, Viewer{UID: "u"}, page)
	mustNotContain(t, disabled, `id="roomCta"`)
}

// TestRenderRoomSpectator locks the watch-only room page: the spectator flag
// lio-game.js keys off, the anchored board orientation and anchor id, identity-
// labeled (the bot's difficulty persona / Anonymous, or usernames) clocks and
// timeline rows, and every game control rendered permanently disabled.
func TestRenderRoomSpectator(t *testing.T) {
	p := message.RoomTemplatePayload{
		RoomID:      "abc",
		PlayerColor: "-", // Lookup returns NoColor for a non-player
		IsSpectator: true,
		WhiteIsBot:  true,     // bot seat may be either color for a spectator
		BotPersona:  "knight", // the bot's chosen difficulty labels its seat
		AnchorColor: "b",      // the human anchors the bottom, currently black
		AnchorID:    "human-uid",
		VariantName: "Half One blitz",
		Variant:     variant.HalfOneBlitz,
	}
	out := renderSmoke(t, Room(RoomMeta(p), p))

	// the flags the client reads once at init: watch-only mode and the anchored
	// player whose seat stays on the bottom across between-game color swaps
	mustContain(t, out, `data-spectator="true"`)
	mustContain(t, out, `data-anchor="human-uid"`)
	// the board is oriented to the anchored player's current color
	mustContain(t, out, `class="gcon b"`)
	mustNotContain(t, out, `class="gcon -"`)

	// clocks and timeline rows are labeled by identity, not You/Opponent or
	// color; the anchor pins the human to the bottom, so the bot marker is
	// always on the top clock whatever color the bot holds. The bot seat shows
	// its difficulty persona name ("Knight"), with the CPU icon plus the piece
	// glyph beside it on both the clock (.clockBotGlyph) and the timeline row
	// (.tl-seat / .tl-seat-glyph); the human seat has no account here, so it
	// reads "Anonymous" (never "You" — the viewer is a spectator, not that
	// player)
	mustContain(t, out, "Knight</span>")
	mustContain(t, out, `class="clockBotGlyph"`)
	mustContain(t, out, `class="tl-seat"`)
	mustContain(t, out, `class="tl-seat-glyph"`)
	mustContain(t, out, ">Anonymous</span>")
	mustNotContain(t, out, ">You</span>")
	mustNotContain(t, out, ">PLAYER</span>")
	mustContain(t, out, `id="clockPlayer" class="clockPlayer ga-you" data-bot="false"`)
	mustContain(t, out, `id="clockOpponent" class="clockOpponent ga-opp" data-bot="true"`)

	// every game control renders, permanently disabled, with the watching tooltip
	mustContain(t, out, `id="btn-resign" class="ctrl-btn play-ctrl" title="Watching as a spectator" disabled>`)
	mustContain(t, out, `id="btn-draw" class="ctrl-btn play-ctrl" title="Watching as a spectator" disabled>`)
	mustContain(t, out, `id="btn-rematch" class="ctrl-btn ctrl-rematch over-ctrl" title="Watching as a spectator" data-rematch-url="" disabled>`)
	mustContain(t, out, `id="result-rematch" type="button" class="result-btn result-rematch" title="Watching as a spectator" data-rematch-url="" disabled>`)

	// crowd label reflects the spectator-only count semantics
	mustContain(t, out, "watching")
}

// TestRenderRoomUsernames locks the Phase-2 username display: a logged-in
// player sees their opponent's username on the top clock and "You" on their
// own (anonymous) bottom clock; a logged-in player's own username shows when
// present. Names are resolved by color through the seat-label helpers.
func TestRenderRoomUsernames(t *testing.T) {
	// viewer is white and logged in as "drewtest"; opponent black is
	// "cdpplayer"
	p := message.RoomTemplatePayload{
		RoomID:        "abc",
		PlayerColor:   "w",
		OpponentColor: "b",
		WhiteName:     "drewtest",
		BlackName:     "cdpplayer",
		CreatorName:   "drewtest",
		VariantName:   "Half One blitz",
		Variant:       variant.HalfOneBlitz,
	}
	out := renderSmoke(t, Room(RoomMeta(p), p))
	// own seat shows the username (not "You") when logged in; opponent's too
	mustContain(t, out, ">drewtest</span>")
	mustContain(t, out, ">cdpplayer</span>")
	mustNotContain(t, out, ">You</span>")
	mustNotContain(t, out, ">Anonymous</span>")

	// a logged-in viewer facing an anonymous opponent: opponent reads
	// "Anonymous", the viewer's own seat their username
	p2 := message.RoomTemplatePayload{
		RoomID: "abc", PlayerColor: "w", OpponentColor: "b",
		WhiteName: "drewtest", Variant: variant.HalfOneBlitz,
		VariantName: "Half One blitz",
	}
	out2 := renderSmoke(t, Room(RoomMeta(p2), p2))
	mustContain(t, out2, ">drewtest</span>")
	mustContain(t, out2, ">Anonymous</span>")

	// the OG/room title carries the challenger's username
	mustContain(t, out, "Challenge from drewtest")
}

// TestRenderRoomRated locks the Phase-5 rating display: in-game each seat's
// rating shows on its clock and the OG/room title carries the creator's rating;
// the pre-game summary carries a "Rated" badge and the creator's rating. A
// casual game shows none of it.
func TestRenderRoomRated(t *testing.T) {
	// in-game render: clocks + OG title
	p := message.RoomTemplatePayload{
		RoomID:        "abc",
		PlayerColor:   "w",
		OpponentColor: "b",
		WhiteName:     "drewtest",
		BlackName:     "cdpplayer",
		CreatorName:   "drewtest",
		WhiteRating:   "1650",
		BlackRating:   "1500?",
		CreatorRating: "1650",
		Rated:         true,
		VariantName:   "Half One blitz",
		Variant:       variant.HalfOneBlitz,
	}
	out := renderSmoke(t, Room(RoomMeta(p), p))
	mustContain(t, out, "1650")  // white clock rating
	mustContain(t, out, "1500?") // black clock rating (provisional)
	mustContain(t, out, "Challenge from drewtest (1650)")
	mustContain(t, out, `id="result-ratings"`)           // game-over delta slot (JS-filled)
	mustContain(t, out, `data-event="Rated Blitz game"`) // copy-PGN fallback Event name

	// pre-game joiner render: the "Rated" badge + creator rating in the summary
	joiner := p
	joiner.IsJoining = true
	joiner.JoinToken = "tok"
	outJ := renderSmoke(t, Room(RoomMeta(joiner), joiner))
	mustContain(t, outJ, ">Rated</span>")
	mustContain(t, outJ, "(1650)") // creator rating beside their name

	// a casual game carries no ratings and no badge
	casual := message.RoomTemplatePayload{
		RoomID: "abc", PlayerColor: "w", OpponentColor: "b",
		WhiteName: "drewtest", BlackName: "cdpplayer",
		VariantName: "Half One blitz", Variant: variant.HalfOneBlitz,
	}
	outC := renderSmoke(t, Room(RoomMeta(casual), casual))
	mustNotContain(t, outC, ">Rated</span>")
	mustNotContain(t, outC, "clockRatingNumber")
}

// TestRenderClockRatingDelta locks the archive clock's "rating + change"
// rendering: a gain is a green +N, a loss a red -N, a zero delta shows only the
// rating (live clocks), and no rating shows nothing at all.
func TestRenderClockRatingDelta(t *testing.T) {
	gain := renderSmoke(t, clock(title.Title{}, "drewtest", "", "1650", 8))
	mustContain(t, gain, ">1650</span>")
	mustContain(t, gain, "clockRatingDelta win")
	mustContain(t, gain, "+8")

	loss := renderSmoke(t, clock(title.Title{}, "cdpplayer", "", "1500?", -8))
	mustContain(t, loss, "1500?")
	mustContain(t, loss, "clockRatingDelta loss")
	mustContain(t, loss, "-8")

	// zero delta (the live clocks): rating shown, no delta span
	none := renderSmoke(t, clock(title.Title{}, "drewtest", "", "1650", 0))
	mustContain(t, none, ">1650</span>")
	mustNotContain(t, none, "clockRatingDelta")

	// no rating (casual/anon/bot): no rating block at all
	empty := renderSmoke(t, clock(title.Title{}, "You", "", "", 0))
	mustNotContain(t, empty, "clockRating")

	// a bot seat: the persona glyph renders as the avatar and the generic CPU
	// icon is not; a human seat (empty glyph) is the reverse
	bot := renderSmoke(t, clock(title.Title{}, "Queen", "♛︎", "", 0))
	mustContain(t, bot, `class="clockBotGlyph"`)
	human := renderSmoke(t, clock(title.Title{}, "drewtest", "", "1650", 0))
	mustNotContain(t, human, "clockBotGlyph")
}

// TestRenderPlayerTitle locks the account title badge: a titled clock renders a
// .player-title span showing the titles row's short code and tooltipping its
// full name; an untitled one renders none. A title carrying no name (an older
// room snapshot, restored code-only) tooltips the code instead of an empty
// string. The badge's accent color comes from CSS (var(--accent)), so it's
// exercised at the DOM level here, not the color.
func TestRenderPlayerTitle(t *testing.T) {
	titled := renderSmoke(t, clock(
		title.Title{Code: "GM", Name: "Grandmaster"}, "drewtest", "", "1650", 0))
	mustContain(t, titled, `class="player-title"`)
	mustContain(t, titled, ">GM</span>")
	mustContain(t, titled, `title="Grandmaster"`)

	nameless := renderSmoke(t, clock(
		title.Title{Code: "OG"}, "drewtest", "", "1650", 0))
	mustContain(t, nameless, `title="OG"`)

	untitled := renderSmoke(t, clock(title.Title{}, "drewtest", "", "1650", 0))
	mustNotContain(t, untitled, "player-title")
}

func TestRenderRoomCreator(t *testing.T) {
	p := message.RoomTemplatePayload{
		RoomID:      "abc",
		PlayerColor: "w",
		VariantName: "Half One blitz",
		Variant:     variant.HalfOneBlitz,
		IsCreator:   true,
		CancelToken: "tok",
	}
	out := renderSmoke(t, Room(RoomMeta(p), p))
	mustContain(t, out, "/abc/cancel")
	mustContain(t, out, "lio-room-create") // creator script
	mustContain(t, out, `id="gameInviteLink"`)
	mustContain(t, out, "Waiting for an opponent") // live waiting status
	mustContain(t, out, `class="invite-qr"`)       // server-rendered QR svg
	mustContain(t, out, "<path d=")                // QR has dark modules
	mustContain(t, out, "You play")                // game summary
	// the live board ships the (hidden) eval bar for the post-bot-game
	// analysis reveal — checked here via the shared Room render path
	// (roomGame's board() carries it; pregame views don't)
	mustNotContain(t, out, `id="eval-bar"`) // creator pregame has no board
	// share-first hero: quiet ghost cancel (never the loud danger button) and
	// the plain-language clock decode in the summary
	mustContain(t, out, "cancel-ghost")
	mustNotContain(t, out, "btn-danger")
	mustContain(t, out, "30 seconds each + 1 second per move")
	// anonymous creator: no identity line
	mustNotContain(t, out, "Playing as")

	// logged-in creator: "Playing as" identity line with the rating chip
	p.CreatorName = "drewtest"
	p.CreatorRating = "1650?"
	named := renderSmoke(t, Room(RoomMeta(p), p))
	mustContain(t, named, "Playing as")
	mustContain(t, named, "drewtest")
	mustContain(t, named, `class="rating-chip"`)
}

func TestRenderRoomJoiner(t *testing.T) {
	p := message.RoomTemplatePayload{
		RoomID:      "abc",
		PlayerColor: "b", // open seat color, set by HandlePreGame
		VariantName: "Half One blitz",
		Variant:     variant.HalfOneBlitz,
		IsJoining:   true,
		JoinToken:   "tok",
	}
	out := renderSmoke(t, Room(RoomMeta(p), p))
	mustContain(t, out, "/abc/join")
	mustContain(t, out, `name="join_token"`)
	mustContain(t, out, "You've been challenged")
	mustContain(t, out, "Black") // open-seat color shown in the summary

	// challenger card, anonymous creator: "?" avatar chip + fallback name +
	// the side the challenger plays (joiner takes black → challenger is white)
	mustContain(t, out, `class="challenger-card"`)
	mustContain(t, out, ">?</span>")
	mustContain(t, out, "Anonymous player")
	mustContain(t, out, "Challenger · plays White")

	// named + rated challenger: initial-letter chip, username, rating chip
	p.CreatorName = "pregametest"
	p.CreatorRating = "1500?"
	named := renderSmoke(t, Room(RoomMeta(p), p))
	mustContain(t, named, ">P</span>") // initial-letter avatar chip
	mustContain(t, named, "pregametest")
	mustContain(t, named, `class="rating-chip"`)

	// random-color room: the challenger's side is hidden
	p.BlindColor = true
	blind := renderSmoke(t, Room(RoomMeta(p), p))
	mustContain(t, blind, "Challenger · plays a random color")
}

// TestRenderRoomArchive locks the archived-game page: the archive board mount
// with its data attributes, the inline #archive-data hydration payload, and
// the engine eval bar (rendered hidden; lio-game.js reveals it only when the
// payload carries cached evals).
func TestRenderRoomArchive(t *testing.T) {
	m := ArchiveModel{
		RoomID:      "abc",
		VariantName: "½ + 1",
		N:           1,
		Count:       1,
		Orientation: "w",
		TopName:     "PLAYER",
		BottomName:  "PLAYER",
		EndedDate:   "Jan 1, 2026",
		Data:        ArchiveData{GameID: "g-uuid", N: 1, Count: 1},
	}
	out := renderSmoke(t, RoomArchive(ArchiveMeta(m), m))
	mustContain(t, out, `data-archive="true"`)
	mustContain(t, out, `id="archive-data"`)
	mustContain(t, out, `id="eval-bar"`)
	mustContain(t, out, `class="eval-fill"`)
	mustContain(t, out, "hidden")      // bar ships hidden until evals hydrate
	mustNotContain(t, out, "eval-num") // pure bar — no numbers

	// free exploration: the archive board carries the promotion picker (for
	// explored promotion pushes) and the hidden explore nudge
	mustContain(t, out, `id="promo-select"`)
	mustContain(t, out, `id="explore-hint"`)
}

// TestRenderNews locks the paginated news page: the full page shell, the first
// page of entries, the htmx pager when the feed spans multiple pages, and the
// oldest entry landing on the last page.
func TestRenderNews(t *testing.T) {
	out := renderSmoke(t, News(PageMeta("News"), 1))
	mustContain(t, out, "<title>octad.gg • News</title>")
	mustContain(t, out, `id="news-content"`)                     // htmx swap region
	mustContain(t, out, templ.EscapeString(news.Items[0].Title)) // newest entry on page 1

	if len(news.Items) > news.PerPage {
		// more than one page: the older-page pager link is present and points on
		mustContain(t, out, "Older →")
		mustContain(t, out, `hx-get="/news?page=2"`)

		// the last page carries the oldest entry and offers no further "older"
		last := news.Paginate(len(news.Items)) // any over-range page clamps to last
		outLast := renderSmoke(t, NewsContent(last.Number))
		mustContain(t, outLast, templ.EscapeString(news.Items[len(news.Items)-1].Title))
		mustNotContain(t, outLast, `hx-get="/news?page=`+strconv.Itoa(last.Number+1)+`"`)
	}
}

func TestRenderAboutAndNotFound(t *testing.T) {
	mustContain(t, renderSmoke(t, About(PageMeta("About"), "board")), "The Board")
	mustContain(t, renderSmoke(t, About(PageMeta("About"), "rules")), `data-castle-demo="far"`)
	mustContain(t, renderSmoke(t, About(PageMeta("About"), "notation")), "ppkn/4/4/NKPP w NCFncf - 0 1")
	mustContain(t, renderSmoke(t, NotFound(PageMeta("404"))), "404")
	mustContain(t, renderSmoke(t, DB(PageMeta("Game Database"))), "Game Database")
}

// TestRenderHeaderViewerStates covers the header's three account states: a
// logged-out viewer with accounts enabled (live login button + auth modal), a
// logged-in viewer (username button + profile popover, no modal), and the
// zero-Viewer disabled state exercised in TestRenderIndex.
func TestRenderHeaderViewerStates(t *testing.T) {
	page := NotFound(PageMeta("404")) // any page carrying the header

	loggedOut := renderSmokeViewer(t,
		Viewer{AccountsEnabled: true}, page)
	mustContain(t, loggedOut, `id="loginButton"`)
	mustContain(t, loggedOut, `id="modalAccount"`)
	mustContain(t, loggedOut, `id="loginForm"`)
	mustContain(t, loggedOut, `id="registerForm"`)
	mustNotContain(t, loggedOut, `id="profilePopover"`)
	// accounts are available, so the login button is live rather than the
	// unavailable placeholder. Asserted on the placeholder's own copy: a bare
	// "disabled" substring matches any inert control anywhere on the page
	// (the signup form goes inert when registration is closed, for one).
	mustNotContain(t, loggedOut, "Accounts are unavailable")

	// Phase 4 login-time second-factor step + method controls
	mustContain(t, loggedOut, `id="mfaStep"`)
	mustContain(t, loggedOut, `id="mfaCodeForm"`)
	mustContain(t, loggedOut, `id="mfaPasskeyBtn"`)
	mustContain(t, loggedOut, `data-mfa-alt="passkey"`)
	mustContain(t, loggedOut, `data-mfa-alt="recovery"`)

	loggedIn := renderSmokeViewer(t,
		Viewer{UID: "uid123", LoggedIn: true, Username: "drew",
			Title:           title.Title{Code: "GM", Name: "Grandmaster"},
			AccountsEnabled: true}, page)
	mustContain(t, loggedIn, `id="profileButton"`)
	mustContain(t, loggedIn, ">drew</span>")
	// the viewer's own account title badges the header button
	mustContain(t, loggedIn, `class="player-title"`)
	mustContain(t, loggedIn, ">GM</span>")
	mustContain(t, loggedIn, `id="profilePopover"`)
	mustContain(t, loggedIn, `id="logoutButton"`)
	mustContain(t, loggedIn, `content="uid123"`) // lio-uid meta
	mustNotContain(t, loggedIn, `id="modalAccount"`)
	mustNotContain(t, loggedIn, `id="loginButton"`)

	// Phase 3 account-admin sections + actions live in the popover
	mustContain(t, loggedIn, `id="passwordForm"`)
	mustContain(t, loggedIn, `id="sessionsDetails"`)
	mustContain(t, loggedIn, `id="sessionsBody"`)
	mustContain(t, loggedIn, `id="logoutAllButton"`)

	// Phase 4 security surface: the popover button opens the (logged-in-only)
	// two-factor & passkey modal
	mustContain(t, loggedIn, `id="securityButton"`)
	mustContain(t, loggedIn, `id="modalSecurity"`)
	mustContain(t, loggedIn, `id="securityModalBody"`)
	mustNotContain(t, loggedIn, "arrive soon") // old Phase-3 placeholder gone

	// polish-pass Edit Profile surface: the popover pencil opens the
	// (logged-in-only) modal with the email + one-time username-change forms,
	// and the username is prefilled with the viewer's current display name
	mustContain(t, loggedIn, `id="editProfileButton"`)
	mustContain(t, loggedIn, `id="modalEditProfile"`)
	mustContain(t, loggedIn, `id="usernameForm"`)
	mustContain(t, loggedIn, `id="emailForm"`)
	mustContain(t, loggedIn, `value="drew"`) // username prefill
	mustNotContain(t, loggedOut, `id="modalEditProfile"`)
}

// TestRenderSessionList covers the active-sessions fragment: the current
// session is labeled and has no revoke button; other sessions carry a revoke
// button keyed by id.
func TestRenderSessionList(t *testing.T) {
	out := renderSmoke(t, SessionList([]SessionView{
		{ID: 1, Device: "Chrome on macOS", LastSeen: "just now", Current: true},
		{ID: 2, Device: "Safari on iOS", LastSeen: "2 hours ago", Current: false},
	}))
	mustContain(t, out, "Chrome on macOS")
	mustContain(t, out, "Safari on iOS")
	mustContain(t, out, "This device")
	// the current session (id 1) is not revocable; the other (id 2) is
	mustContain(t, out, `data-session-id="2"`)
	mustNotContain(t, out, `data-session-id="1"`)

	// empty state
	mustContain(t, renderSmoke(t, SessionList(nil)), "No active sessions")
}

// profileFixture is a populated, unbanned player page model.
func profileFixture() ProfileModel {
	return ProfileModel{
		UserID:   7,
		Username: "drewtest",
		Title:    title.Title{Code: "OG", Name: "Original Gamer"},
		Joined:   "March 2026",
		Ratings: []RatingView{
			{Category: "half-one-blitz", Rating: "1653", Games: 12},
		},
		Total: NewRecordView(db.Record{Games: 3, Wins: 2, Draws: 0, Losses: 1}),
		Variants: []VariantRecordView{{Name: "½ + 1", Group: "blitz",
			RecordView: NewRecordView(db.Record{Games: 3, Wins: 2, Losses: 1})}},
		Bots: []BotRecordView{{Persona: "Queen", Glyph: "♛︎",
			RecordView: NewRecordView(db.Record{Games: 1, Losses: 1})}},
		Games: []ProfileGameView{{
			URL: "/abc123/1", When: "2 days ago", Variant: "½ + 1 blitz",
			Mode: "Rated", Result: "Won", Class: "win", Opponent: "cdpplayer",
			OppTitle: "GM",
		}},
	}
}

// TestRenderProfile covers the public player page: identity with the title
// badge, ratings, records and the games list all present for a normal account.
func TestRenderProfile(t *testing.T) {
	m := profileFixture()
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	mustContain(t, out, ">drewtest</span>")
	mustContain(t, out, `class="player-title"`)
	mustContain(t, out, ">OG</span>")
	mustContain(t, out, "Member since")
	mustContain(t, out, "March 2026")
	mustContain(t, out, "1653")           // rating tile
	mustContain(t, out, "half-one-blitz") // its category
	mustContain(t, out, "All games")      // lifetime record row
	mustContain(t, out, "Versus the computer")
	mustContain(t, out, "♛︎ Queen")         // bot record label
	mustContain(t, out, `href="/abc123/1"`) // game links into the archive
	mustContain(t, out, "cdpplayer")
	// the page title carries the title code, as the OG/meta treatment does
	mustContain(t, out, templ.EscapeString("OG drewtest"))
	// no moderation UI for an ordinary viewer
	mustNotContain(t, out, `id="modForm"`)
	mustNotContain(t, out, "lio-mod")
}

// TestRenderProfileClosed locks the public treatment of a banned account: a
// neutral closed card, and *no* reason, expiry, ratings, record or games. The
// reason is moderator-only, so leaking it here would be a real disclosure bug.
func TestRenderProfileClosed(t *testing.T) {
	m := profileFixture()
	m.Closed = true
	m.Ratings, m.Variants, m.Bots, m.Games = nil, nil, nil, nil
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	mustContain(t, out, "This account is closed.")
	mustContain(t, out, ">drewtest</span>") // the name still renders
	mustNotContain(t, out, "1653")          // ratings suppressed
	mustNotContain(t, out, "All games")     // record suppressed
	mustNotContain(t, out, "Recent games")  // games suppressed
}

// TestRenderProfileModBar covers the moderator view: the action controls
// render, the role picker is admin-only, and a reason field is always present
// (every action is required to carry one).
func TestRenderProfileModBar(t *testing.T) {
	m := profileFixture()
	m.ShowMod = true
	m.Mod = ModBarView{
		CanSetRole:     false,
		CanBan:         true,
		CurrentRole:    "player",
		CurrentTitleID: "1",
		Titles:         []TitleOptionView{{ID: "1", Code: "OG", Name: "Original Gamer"}},
		Roles:          AssignableRoles(),
		Actions: []ModFeedEntry{
			{When: "yesterday", Actor: "cdpplayer", Action: "title", Reason: "won the open",
				Details: DetailChipsOf(map[string]any{"from": "none", "to": "OG"})},
		},
		ActionsTotal: 1,
	}
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	mustContain(t, out, `id="modForm"`)
	mustContain(t, out, `data-user-id="7"`)
	mustContain(t, out, `name="reason"`)
	mustContain(t, out, `data-mod-action="ban"`)
	mustContain(t, out, `data-mod-action="title"`)
	mustContain(t, out, `data-mod-action="rename"`)
	mustContain(t, out, "lio-mod") // the bar's script only loads for moderators
	mustContain(t, out, "won the open")
	// a mod is not an admin: no role control, even though Roles was populated
	mustNotContain(t, out, `data-mod-action="role"`)

	// an admin gets it
	m.Mod.CanSetRole = true
	admin := renderSmoke(t, Profile(ProfileMeta(m), m))
	mustContain(t, admin, `data-mod-action="role"`)

	// a banned target swaps the ban control for the lift control
	m.Mod.Banned = true
	m.Mod.BanUntil = "permanently"
	m.Mod.BanReason = "engine assistance"
	banned := renderSmoke(t, Profile(ProfileMeta(m), m))
	mustContain(t, banned, `data-mod-action="unban"`)
	mustContain(t, banned, "engine assistance") // moderators DO see the reason
	mustNotContain(t, banned, `data-mod-action="ban"`)
}

// systemFixture is a console model with a small first page of audit entries.
func systemFixture() SystemModel {
	return SystemModel{
		IsAdmin:  true,
		Settings: settings.Snapshot{RegistrationOpen: true, RatedEnabled: true},
		Feed: AuditFeed{
			Actions: []ModFeedEntry{
				{When: "just now", WhenExact: "2026-07-25 12:00:00 UTC",
					Actor: "drewtest", Action: "ban", Target: "spammer99", Reason: "ads",
					Details: DetailChipsOf(map[string]any{
						"permanent": true, "duration": "permanent", "forfeited": float64(1),
					})},
				{When: "yesterday", WhenExact: "2026-07-24 09:00:00 UTC",
					Actor: "drewtest", Action: "setting", Reason: "deploy window",
					Details: DetailChipsOf(map[string]any{"maintenance": true})},
			},
			ActionKinds: ModActionKinds,
			Page:        1,
			Pages:       1,
			Total:       2,
		},
	}
}

// TestRenderSystemConsole covers /system: an admin sees the site controls, a
// plain moderator does not (they affect every visitor at once), and both see
// the audit log.
func TestRenderSystemConsole(t *testing.T) {
	m := systemFixture()
	admin := renderSmoke(t, System(SystemMeta(), m))
	mustContain(t, admin, `id="settingsForm"`)
	mustContain(t, admin, `data-setting="maintenance"`)
	mustContain(t, admin, `data-setting="registrationOpen"`)
	mustContain(t, admin, `data-setting="notice"`)
	mustContain(t, admin, `name="reason"`) // every change carries one
	mustContain(t, admin, "Audit log")
	mustContain(t, admin, "2 entries on record")
	mustContain(t, admin, "spammer99")
	mustContain(t, admin, `href="/@/spammer99"`) // targets link to their page
	mustContain(t, admin, "site-wide")           // site-level entry, no target

	// a moderator who is not an admin gets the log but no site controls
	m.IsAdmin = false
	mod := renderSmoke(t, System(SystemMeta(), m))
	mustNotContain(t, mod, `id="settingsForm"`)
	mustContain(t, mod, "Audit log")

	// empty log distinguishes "nothing yet" from "nothing matches"
	mustContain(t, renderSmoke(t, System(SystemMeta(), SystemModel{})),
		"Nothing has been actioned yet.")
	empty := AuditFeed{Filtered: true, Query: "nobody", Page: 1, Pages: 1}
	mustContain(t, renderSmoke(t, AuditFeedBody(empty)), "No actions match that search.")
}

// TestRenderAuditPager: the pager appears only past one page, disables the end
// it is at, and both controls carry a real href plus its htmx fragment.
func TestRenderAuditPager(t *testing.T) {
	one := systemFixture().Feed
	mustNotContain(t, renderSmoke(t, AuditFeedBody(one)), "audit-pager")

	mid := one
	mid.Page, mid.Pages, mid.Total = 2, 3, 120
	mid.PrevURL = AuditURL(ModActionQuery{Page: 1})
	mid.NextURL = AuditURL(ModActionQuery{Page: 3})
	out := renderSmoke(t, AuditFeedBody(mid))
	mustContain(t, out, "Page 2 of 3")
	mustContain(t, out, `href="/system"`)                  // page 1 is the bare path
	mustContain(t, out, `href="/system?page=3"`)           // next page
	mustContain(t, out, `hx-get="/system/actions?page=3"`) // its fragment
	mustNotContain(t, out, "is-disabled")

	last := mid
	last.Page, last.NextURL = 3, ""
	end := renderSmoke(t, AuditFeedBody(last))
	mustContain(t, end, "is-disabled") // "Older" is inert at the end
}

// TestAuditURL locks the link builder the pager and the player page share: an
// unfiltered first page is the bare path, and filters survive paging.
func TestAuditURL(t *testing.T) {
	cases := []struct {
		q    ModActionQuery
		want string
	}{
		{ModActionQuery{Page: 1}, "/system"},
		{ModActionQuery{Page: 0}, "/system"},
		{ModActionQuery{Page: 2}, "/system?page=2"},
		{ModActionQuery{Query: "drewtest", Page: 1}, "/system?q=drewtest"},
		{ModActionQuery{Query: "drewtest", Action: "ban", Page: 3},
			"/system?action=ban&page=3&q=drewtest"},
	}
	for _, tc := range cases {
		if got := AuditURL(tc.q); got != tc.want {
			t.Errorf("AuditURL(%+v) = %q, want %q", tc.q, got, tc.want)
		}
	}
}

// TestNormalizeAction drops unknown verbs so a hand-typed query string cannot
// silently produce an empty feed.
func TestNormalizeAction(t *testing.T) {
	for _, kind := range ModActionKinds {
		if got := NormalizeAction(kind); got != kind {
			t.Errorf("NormalizeAction(%q) = %q", kind, got)
		}
	}
	for _, bad := range []string{"", "BAN", "drop", "'; --"} {
		if got := NormalizeAction(bad); got != "" {
			t.Errorf("NormalizeAction(%q) = %q, want empty", bad, got)
		}
	}
}

// TestSettingToggleLabelsAction locks a small but load-bearing UX rule: the
// button says what it will DO, not what the state IS. A control labelled with
// its own state is how an operator turns the wrong thing off mid-incident.
func TestSettingToggleLabelsAction(t *testing.T) {
	on := renderSmoke(t, settingToggle("maintenance", "Maintenance mode", "help", true))
	mustContain(t, on, "Turn off")
	mustContain(t, on, `data-value="0"`)
	mustContain(t, on, "setting-on")

	off := renderSmoke(t, settingToggle("maintenance", "Maintenance mode", "help", false))
	mustContain(t, off, "Turn on")
	mustContain(t, off, `data-value="1"`)
	mustContain(t, off, "setting-off")
}

// withSettings runs fn against a temporary settings override, restoring the
// real snapshot afterward. The settings package caches from Postgres, which the
// view tests do not have, so these poke the resolved snapshot directly.
func withSettings(t *testing.T, s settings.Snapshot, fn func()) {
	t.Helper()
	restore := settings.OverrideForTest(s)
	defer restore()
	fn()
}

// TestRenderRegistrationClosed covers the signup form while registration is
// closed: the visitor is told why, and every control goes inert. The form is
// still rendered — removing it would shift the modal and leave the Sign up tab
// pointing at nothing.
func TestRenderRegistrationClosed(t *testing.T) {
	page := Index(PageMeta("home"), nil, message.SiteStats{})
	viewer := Viewer{UID: "u", AccountsEnabled: true}

	withSettings(t, settings.Snapshot{RegistrationOpen: true, RatedEnabled: true}, func() {
		open := renderSmokeViewer(t, viewer, page)
		mustContain(t, open, `id="registerForm"`)
		mustNotContain(t, open, "sign-ups are temporarily closed")
		mustNotContain(t, open, `<fieldset class="contents" disabled>`)
	})

	withSettings(t, settings.Snapshot{RegistrationOpen: false, RatedEnabled: true}, func() {
		closed := renderSmokeViewer(t, viewer, page)
		mustContain(t, closed, `id="registerForm"`) // still present, just inert
		mustContain(t, closed, "sign-ups are temporarily closed")
		mustContain(t, closed, `<fieldset class="contents" disabled>`)
		// the login form is untouched: closing signups is not a lockout
		mustContain(t, closed, `id="loginForm"`)
	})
}

// TestRenderRatedPaused covers the create-game modal while ratings are paused:
// the live rated badge is replaced by an inert unrated one, and the player's
// other choices are left alone. Pausing ratings must not take clocks away, so
// the casual toggle stays free and the time controls stay selectable — the
// server forces Rated off regardless of what the form submits.
func TestRenderRatedPaused(t *testing.T) {
	page := Index(PageMeta("home"), nil, message.SiteStats{})
	viewer := Viewer{UID: "u", LoggedIn: true, Username: "drew", AccountsEnabled: true}
	// the casual checkbox, exactly as it renders when nothing has touched it
	freeCasual := `<input type="checkbox" class="cg-toggle-box cg-casual-box" name="casual" value="true">`

	withSettings(t, settings.Snapshot{RegistrationOpen: true, RatedEnabled: true}, func() {
		on := renderSmokeViewer(t, viewer, page)
		mustNotContain(t, on, "Rated games are temporarily disabled")
		mustContain(t, on, "Counts toward your rating") // live badge
		mustContain(t, on, freeCasual)
	})

	withSettings(t, settings.Snapshot{RegistrationOpen: true, RatedEnabled: false}, func() {
		off := renderSmokeViewer(t, viewer, page)
		mustContain(t, off, "cg-rated-paused")
		mustContain(t, off, "Rated games are temporarily disabled")
		mustNotContain(t, off, "Counts toward your rating") // live badge replaced
		// the casual toggle is untouched: not checked, not disabled
		mustContain(t, off, freeCasual)
		mustNotContain(t, off, `<input type="hidden" name="casual"`)
		// who-can-join is a separate question and stays available
		mustContain(t, off, `name="allow_anon"`)
	})

	// an anonymous visitor is not invited to log in "for rated play" while
	// there is no rated play to log in for
	withSettings(t, settings.Snapshot{RegistrationOpen: true, RatedEnabled: false}, func() {
		anon := renderSmokeViewer(t, Viewer{UID: "u", AccountsEnabled: true}, page)
		mustContain(t, anon, "cg-rated-paused")
		mustNotContain(t, anon, "to play rated games")
	})
}

// TestRenderMaintenanceBanner: the maintenance bar is its own element, on every
// page, independent of whether an admin notice happens to be set. The two stack
// when both are active.
func TestRenderMaintenanceBanner(t *testing.T) {
	page := About(PageMeta("about"), "main")

	withSettings(t, settings.Snapshot{RegistrationOpen: true, RatedEnabled: true}, func() {
		mustNotContain(t, renderSmoke(t, page), "maintenance-notice")
	})

	withSettings(t, settings.Snapshot{Maintenance: true, RegistrationOpen: true, RatedEnabled: true}, func() {
		out := renderSmoke(t, page)
		mustContain(t, out, "maintenance-notice")
		mustContain(t, out, "New games are paused")
		mustNotContain(t, out, "site-notice ") // no admin notice set
	})

	withSettings(t, settings.Snapshot{
		Maintenance: true, NoticeText: "back at 21:00", NoticeLevel: settings.LevelWarn,
		RegistrationOpen: true, RatedEnabled: true,
	}, func() {
		both := renderSmoke(t, page)
		mustContain(t, both, "maintenance-notice")
		mustContain(t, both, "back at 21:00")
	})
}

// TestActiveNoticesOf derives the /system stand-down list from a snapshot. The
// clear value for each row must be the setting change that returns it to the
// default — a stand-down button that re-posts the current state is worse than
// no button.
func TestActiveNoticesOf(t *testing.T) {
	if got := ActiveNoticesOf(settings.Snapshot{
		RegistrationOpen: true, RatedEnabled: true,
	}); len(got) != 0 {
		t.Fatalf("default site has %d active notices: %+v", len(got), got)
	}

	all := ActiveNoticesOf(settings.Snapshot{
		Maintenance: true, RegistrationOpen: false, RatedEnabled: false,
		NoticeText: "hello",
	})
	if len(all) != 4 {
		t.Fatalf("got %d notices, want 4: %+v", len(all), all)
	}
	// ordered by how much a visitor is affected
	wantOrder := []string{"Maintenance mode", "Registration closed", "Rated games paused", "Site notice"}
	for i, want := range wantOrder {
		if all[i].Title != want {
			t.Errorf("notice %d = %q, want %q", i, all[i].Title, want)
		}
	}
	wantClear := map[string]string{
		"maintenance": "0", "registrationOpen": "1", "ratedEnabled": "1", "notice": "",
	}
	for _, n := range all {
		if got, ok := wantClear[n.Setting]; !ok || got != n.ClearValue {
			t.Errorf("%s clears with %q, want %q", n.Setting, n.ClearValue, got)
		}
	}
	// the site notice carries its own text so an admin can see what they are
	// taking down without going hunting for it
	if all[3].Detail != "hello" {
		t.Errorf("site notice detail = %q, want the notice text", all[3].Detail)
	}
}

// TestRenderStaffLinks: the console shortcuts appear in the profile popover for
// a moderator and nobody else.
func TestRenderStaffLinks(t *testing.T) {
	page := Index(PageMeta("home"), nil, message.SiteStats{})

	plain := renderSmokeViewer(t, Viewer{
		UID: "u", LoggedIn: true, Username: "drew", AccountsEnabled: true,
	}, page)
	mustNotContain(t, plain, `href="/system"`)
	mustNotContain(t, plain, `href="/moderation"`)

	staff := renderSmokeViewer(t, Viewer{
		UID: "u", LoggedIn: true, Username: "drew", AccountsEnabled: true, Role: role.Mod,
	}, page)
	mustContain(t, staff, `href="/system"`)
	mustContain(t, staff, `href="/moderation"`)
	// renamed security section
	mustContain(t, staff, "Account Security")
	mustNotContain(t, staff, "Two-factor &amp; passkeys")
}

// TestRenderModBarSelf covers an admin looking at their own player page: the
// harmless edits are offered, ban and role are not, and the bar says why rather
// than silently showing a shorter set of controls.
func TestRenderModBarSelf(t *testing.T) {
	m := profileFixture()
	m.ShowMod = true
	m.Mod = ModBarView{
		IsSelf:      true,
		CanSetRole:  false, // no self-demotion
		CanBan:      false, // no self-ban
		CurrentRole: "admin",
		Titles:      []TitleOptionView{{ID: "1", Code: "OG", Name: "Original Gamer"}},
		Roles:       AssignableRoles(),
	}
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	mustContain(t, out, `id="modForm"`)
	mustContain(t, out, "Your account")
	mustContain(t, out, "not available on your own account")
	mustContain(t, out, `data-mod-action="title"`) // still yours to change
	mustContain(t, out, `data-mod-action="rename"`)
	mustNotContain(t, out, `data-mod-action="ban"`)
	mustNotContain(t, out, `data-mod-action="role"`)
	// the ban blurb goes with the ban control
	mustNotContain(t, out, "ends any game in progress")
}

// TestRenderModHistoryLink: a truncated history says so and links to the full
// record on /system rather than growing a second pager here.
func TestRenderModHistoryLink(t *testing.T) {
	m := profileFixture()
	m.ShowMod = true
	m.Mod = ModBarView{
		CanBan: true,
		Actions: []ModFeedEntry{
			{When: "yesterday", Actor: "cdpplayer", Action: "ban", Reason: "cheating"},
		},
		ActionsTotal: 37,
		HistoryURL:   AuditURL(ModActionQuery{Query: "drewtest", Page: 1}),
	}
	out := renderSmoke(t, Profile(ProfileMeta(m), m))
	mustContain(t, out, "latest 1 of 37")
	mustContain(t, out, `href="/system?q=drewtest"`)
	mustContain(t, out, "See everything involving this account")

	// a complete history needs no link out
	m.Mod.ActionsTotal = 1
	m.Mod.HistoryURL = ""
	whole := renderSmoke(t, Profile(ProfileMeta(m), m))
	mustContain(t, whole, "1 entry")
	mustNotContain(t, whole, "See everything involving this account")
}

// TestDetailChipsOf covers the audit payload rendering: stable ordering with
// the before/after pair first, JSON's float64 numbers printed without a decimal
// tail, booleans as words, and every key carrying an explanation.
func TestDetailChipsOf(t *testing.T) {
	if got := DetailChipsOf(nil); got != nil {
		t.Errorf("nil detail produced %+v", got)
	}

	chips := DetailChipsOf(map[string]any{
		"forfeited": float64(2),
		"to":        "admin",
		"permanent": false,
		"from":      "mod",
	})
	var keys []string
	for _, c := range chips {
		keys = append(keys, c.Key)
		if c.Help == "" {
			t.Errorf("chip %q has no tooltip", c.Key)
		}
	}
	// from/to lead — a before/after pair reads backwards any other way — then
	// the rest alphabetically
	want := []string{"from", "to", "forfeited", "permanent"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("chip order = %v, want %v", keys, want)
	}

	byKey := map[string]string{}
	for _, c := range chips {
		byKey[c.Key] = c.Value
	}
	if byKey["forfeited"] != "2" {
		t.Errorf("float64 rendered as %q, want \"2\"", byKey["forfeited"])
	}
	if byKey["permanent"] != "no" {
		t.Errorf("false rendered as %q, want \"no\"", byKey["permanent"])
	}
	// an empty string is an em dash, never a blank that reads as a bug
	if got := DetailChipsOf(map[string]any{"from": ""})[0].Value; got != "—" {
		t.Errorf("empty value rendered as %q, want an em dash", got)
	}
}

// TestActionClassAndHelp: every verb the log can record has its own tint and a
// human explanation, and an unknown verb still renders rather than blanking.
func TestActionClassAndHelp(t *testing.T) {
	seen := map[string]bool{}
	for _, kind := range ModActionKinds {
		class, help := ActionClass(kind), ActionHelp(kind)
		if class == "" || help == "" {
			t.Errorf("%q: class=%q help=%q", kind, class, help)
		}
		if help == "Moderation action" {
			t.Errorf("%q falls through to the generic help text", kind)
		}
		seen[class] = true
	}
	// sanctions and their reversal must not share a colour
	if ActionClass("ban") == ActionClass("unban") {
		t.Error("ban and unban render identically")
	}
	if got := ActionClass("something-new"); got == "" {
		t.Error("unknown action has no class")
	}
}

// TestSettingEffect: every switch states its consequence in both directions,
// and the two directions differ.
func TestSettingEffect(t *testing.T) {
	for _, key := range []string{"maintenance", "registrationOpen", "ratedEnabled"} {
		on, off := SettingEffect(key, true), SettingEffect(key, false)
		if on == "" || off == "" {
			t.Errorf("%q: on=%q off=%q", key, on, off)
		}
		if on == off {
			t.Errorf("%q reads the same in both directions", key)
		}
	}
}

// TestRenderAuditRowFields locks the per-field rendering: each part is its own
// tinted element, the timestamp and every chip carry a tooltip, and a
// site-level entry says so instead of showing an empty target.
func TestRenderAuditRowFields(t *testing.T) {
	f := AuditFeed{Page: 1, Pages: 1, Actions: []ModFeedEntry{{
		When: "2 days ago", WhenExact: "2026-07-23 11:22:33 UTC",
		Actor: "drewtest", Action: "ban", Target: "spammer99",
		Reason: "engine assistance",
		Details: DetailChipsOf(map[string]any{
			"duration": "permanent", "forfeited": float64(1),
		}),
	}}}
	out := renderSmoke(t, AuditFeedBody(f))

	mustContain(t, out, `class="audit-action act-ban"`)
	mustContain(t, out, `title="2026-07-23 11:22:33 UTC"`) // exact time on hover
	mustContain(t, out, `class="audit-actor"`)
	mustContain(t, out, `class="audit-target"`)
	mustContain(t, out, `href="/@/drewtest"`) // both parties link to their pages
	mustContain(t, out, `href="/@/spammer99"`)
	mustContain(t, out, `class="audit-chip-key"`)
	mustContain(t, out, "forfeited")
	mustContain(t, out, templ.EscapeString("Live games this ban ended as a forfeit"))
	mustContain(t, out, "engine assistance")

	// a site-level entry names itself rather than rendering an empty target
	site := AuditFeed{Page: 1, Pages: 1, Actions: []ModFeedEntry{{
		When: "just now", Actor: "drewtest", Action: "setting", Reason: "window",
		Details: DetailChipsOf(map[string]any{"maintenance": true}),
	}}}
	siteOut := renderSmoke(t, AuditFeedBody(site))
	mustContain(t, siteOut, "site-wide")
	mustContain(t, siteOut, `class="audit-action act-setting"`)
	mustNotContain(t, siteOut, `class="audit-target"`)
}

// TestRenderConfirmModal: the site controls carry the change summary and its
// consequence for the confirmation, and the reason lives in the modal rather
// than at the top of the form.
func TestRenderConfirmModal(t *testing.T) {
	m := systemFixture()
	out := renderSmoke(t, System(SystemMeta(), m))

	mustContain(t, out, `id="modalConfirmChange"`)
	mustContain(t, out, `id="confirmReason"`)
	mustContain(t, out, `id="confirmSummary"`)
	// each control describes its own change + effect for the modal to show
	mustContain(t, out, `data-confirm="Turn on: Maintenance mode"`)
	mustContain(t, out, templ.EscapeString(SettingEffect("maintenance", true)))
	mustContain(t, out, `data-confirm="Set the site notice"`)
	// the form no longer carries a reason field of its own
	if strings.Contains(out, `<form id="settingsForm"`) {
		formPart := out[strings.Index(out, `<form id="settingsForm"`):]
		formPart = formPart[:strings.Index(formPart, "</form>")]
		if strings.Contains(formPart, `name="reason"`) {
			t.Error("settings form still carries its own reason field")
		}
	}

	// a non-admin moderator gets neither the controls nor the modal
	m.IsAdmin = false
	mod := renderSmoke(t, System(SystemMeta(), m))
	mustNotContain(t, mod, `id="modalConfirmChange"`)
}

// TestRenderModBarConfirmFlow: the player-page mod bar no longer carries its own
// reason field, ships the shared confirmation modal, and each action states its
// consequence for that modal to show. The account name rides on the form so the
// confirmation can name who is being acted on.
func TestRenderModBarConfirmFlow(t *testing.T) {
	m := profileFixture()
	m.ShowMod = true
	m.Mod = ModBarView{
		Username:    "drewtest",
		CanBan:      true,
		CanSetRole:  true,
		CurrentRole: "player",
		Titles:      []TitleOptionView{{ID: "1", Code: "OG", Name: "Original Gamer"}},
		Roles:       AssignableRoles(),
	}
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	mustContain(t, out, `id="modalConfirmChange"`)
	mustContain(t, out, `id="confirmReason"`)
	mustContain(t, out, `data-username="drewtest"`)
	// every action carries its consequence
	for _, act := range []string{"ban", "title", "role", "rename"} {
		mustContain(t, out, `data-mod-action="`+act+`"`)
	}
	mustContain(t, out, "Ends any game in progress as a forfeit")
	mustContain(t, out, "signs it out so the new role takes effect immediately")

	// the bar's own reason field is gone — it lives in the modal now
	form := out[strings.Index(out, `<form id="modForm"`):]
	form = form[:strings.Index(form, "</form>")]
	if strings.Contains(form, `name="reason"`) {
		t.Error("mod bar still carries its own reason field")
	}
	// and the standing ban blurb moved into the confirmation, where it is read
	// at the moment it matters
	mustNotContain(t, form, "A ban ends any game in progress")

	// no modal for a page without the bar
	plain := profileFixture()
	mustNotContain(t, renderSmoke(t, Profile(ProfileMeta(plain), plain)), `id="modalConfirmChange"`)
}

// TestRenderModHistoryChips: the player page renders the same audit row the
// /system feed does — payload chips and tooltips included — minus the target,
// which on that page is the page itself.
func TestRenderModHistoryChips(t *testing.T) {
	m := profileFixture()
	m.ShowMod = true
	m.Mod = ModBarView{
		Username: "drewtest",
		CanBan:   true,
		Actions: []ModFeedEntry{{
			When: "yesterday", WhenExact: "2026-07-24 08:00:00 UTC",
			Actor: "cdpplayer", Action: "ban", Reason: "engine assistance",
			Details: DetailChipsOf(map[string]any{
				"duration": "7 days", "forfeited": float64(1),
			}),
		}},
		ActionsTotal: 1,
	}
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	mustContain(t, out, `class="audit-action act-ban"`)
	mustContain(t, out, `title="2026-07-24 08:00:00 UTC"`)
	mustContain(t, out, `class="audit-chip-key"`)
	mustContain(t, out, "forfeited")
	mustContain(t, out, templ.EscapeString("Live games this ban ended as a forfeit"))
	// the target is this page, so it is not repeated on every row
	mustNotContain(t, out, `class="audit-target"`)
	mustNotContain(t, out, "site-wide")
}

// TestRenderReportQueue covers /moderation: open reports render with their
// category tint and links out to the account and the evidence, and the queue
// carries no sanction controls of its own — acting happens on the player page,
// where the record is visible.
func TestRenderReportQueue(t *testing.T) {
	m := ModerationModel{
		OpenCount: 2,
		Open: []ReportView{{
			ID: "7", When: "2 hours ago", WhenExact: "2026-07-26 09:00:00 UTC",
			Category: "cheating", Class: ReportCategoryClass("cheating"),
			Help: ReportCategoryHelp("cheating"), Note: "instant moves from a lost position",
			Reporter: "drewtest", Target: "spammer99",
			GameURL: "/game/abc-123",
		}},
		Closed: []ReportView{{
			ID: "6", Category: "stalling", Class: ReportCategoryClass("stalling"),
			Help: ReportCategoryHelp("stalling"), Target: "someone",
			Resolved: "yesterday", Resolver: "drewtest", Resolution: "warned",
		}},
	}
	out := renderSmoke(t, Moderation(ModerationMeta(), m))

	mustContain(t, out, "2 reports waiting")
	mustContain(t, out, `class="report-cat rep-severe"`)
	mustContain(t, out, "spammer99")
	mustContain(t, out, `href="/@/spammer99"`)  // straight to the account
	mustContain(t, out, `href="/game/abc-123"`) // the evidence
	mustContain(t, out, "instant moves from a lost position")
	mustContain(t, out, `data-resolve-report="7"`)
	mustContain(t, out, "Recently resolved")
	mustContain(t, out, "warned")
	// the queue does not sanction: no ban/role controls live here
	mustNotContain(t, out, `data-mod-action="ban"`)
	mustNotContain(t, out, `data-mod-action="role"`)

	// empty queue explains where reports come from
	empty := renderSmoke(t, Moderation(ModerationMeta(), ModerationModel{}))
	mustContain(t, empty, "Nothing waiting")
	mustNotContain(t, empty, "Recently resolved")
}

// TestReportCategoryMapping: every category the database accepts has a tint,
// an explanation and a picker label, so a new one cannot be added to the CHECK
// constraint and render as a bare slug.
func TestReportCategoryMapping(t *testing.T) {
	for _, c := range ReportCategoriesForPicker() {
		if ReportCategoryClass(c) == "" {
			t.Errorf("%q has no tint", c)
		}
		if help := ReportCategoryHelp(c); help == "" || help == "Reported behaviour" {
			t.Errorf("%q falls through to the generic help", c)
		}
		if label := ReportCategoryLabel(c); label == "" || label == c {
			t.Errorf("%q has no picker label", c)
		}
	}
	// severity is distinguishable
	if ReportCategoryClass("cheating") == ReportCategoryClass("other") {
		t.Error("a cheating report reads the same as an unspecified one")
	}
}

// TestRenderLiveOps covers the /system ops section: the counters, a room list
// that links through to spectate, and the admin-only force-close.
func TestRenderLiveOps(t *testing.T) {
	m := systemFixture()
	m.Live = LiveOps{
		Online: 12, LiveGames: 3, OpenChallenges: 1,
		Rooms: []LiveRoomView{
			{RoomID: "abc123", URL: "/abc123", Variant: "½ + 1 blitz",
				Moves: "14 moves", Kind: "playing"},
			{RoomID: "bot456", URL: "/bot456", Variant: "1 + 2 rapid",
				Moves: "2 moves", VsBot: true, Kind: "playing"},
		},
		Truncated: 5,
	}
	admin := renderSmoke(t, System(SystemMeta(), m))
	mustContain(t, admin, "Right now")
	mustContain(t, admin, ">12<") // online
	mustContain(t, admin, `href="/abc123"`)
	mustContain(t, admin, "½ + 1 blitz")
	mustContain(t, admin, "14 moves")
	mustContain(t, admin, ">bot<") // the bot marker
	mustContain(t, admin, "5 more not shown")
	mustContain(t, admin, `data-close-room="abc123"`)

	// a moderator sees the picture but cannot force-close
	m.IsAdmin = false
	mod := renderSmoke(t, System(SystemMeta(), m))
	mustContain(t, mod, "Right now")
	mustContain(t, mod, `href="/abc123"`)
	mustNotContain(t, mod, "data-close-room")

	// empty
	quiet := systemFixture()
	quiet.Live = LiveOps{}
	mustContain(t, renderSmoke(t, System(SystemMeta(), quiet)), "No rooms are live.")
}
