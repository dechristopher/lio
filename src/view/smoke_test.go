package view

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/news"
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
	mustContain(t, out, "<title>lioctad.org • Free Online Octad</title>")
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

	// create-game modal: opponent + Classic/Deploy toggles, unified POST target,
	// and the hidden field the resolved variant is written into
	mustContain(t, out, `action="/new/game"`)
	mustContain(t, out, `name="opponent" value="human"`)
	mustContain(t, out, `name="opponent" value="computer"`)
	mustContain(t, out, `name="mode" value="classic"`)
	mustContain(t, out, `name="mode" value="deploy"`)
	mustContain(t, out, `id="cg-variant"`)

	// each time-control card carries both its classic and deploy variant name so
	// the mode toggle can resolve one from the other; order is bullet->blitz->rapid
	order := []string{
		"quarter-zero-blitz", "half-one-blitz", "one-two-rapid", // classic
		"quarter-zero-bullet-deploy", "half-one-blitz-deploy", "one-two-rapid-deploy", // deploy
	}
	for _, name := range order {
		mustContain(t, out, name)
	}
	// bullet card precedes rapid card (data-classic attribute order)
	if strings.Index(out, `data-classic="quarter-zero-blitz"`) > strings.Index(out, `data-classic="one-two-rapid"`) {
		t.Error("time-control cards out of order (bullet should precede rapid)")
	}
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
}

// TestRenderRoomSpectator locks the watch-only room page: the spectator flag
// lio-game.js keys off, the anchored board orientation and anchor id, identity-
// labeled (BOT/PLAYER) clocks and timeline rows, and every game control
// rendered permanently disabled.
func TestRenderRoomSpectator(t *testing.T) {
	p := message.RoomTemplatePayload{
		RoomID:      "abc",
		PlayerColor: "-", // Lookup returns NoColor for a non-player
		IsSpectator: true,
		WhiteIsBot:  true, // bot seat may be either color for a spectator
		AnchorColor: "b",  // the human anchors the bottom, currently black
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
	// always on the top clock whatever color the bot holds
	mustContain(t, out, ">BOT</span>")
	mustContain(t, out, ">PLAYER</span>")
	mustNotContain(t, out, ">You</span>")
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
}

// TestRenderNews locks the paginated news page: the full page shell, the first
// page of entries, the htmx pager when the feed spans multiple pages, and the
// oldest entry landing on the last page.
func TestRenderNews(t *testing.T) {
	out := renderSmoke(t, News(PageMeta("News"), 1))
	mustContain(t, out, "<title>lioctad.org • News</title>")
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
	mustNotContain(t, loggedOut, "disabled")

	loggedIn := renderSmokeViewer(t,
		Viewer{UID: "uid123", LoggedIn: true, Username: "drew",
			AccountsEnabled: true}, page)
	mustContain(t, loggedIn, `id="profileButton"`)
	mustContain(t, loggedIn, ">drew</button>")
	mustContain(t, loggedIn, `id="profilePopover"`)
	mustContain(t, loggedIn, `id="logoutButton"`)
	mustContain(t, loggedIn, `content="uid123"`) // lio-uid meta
	mustNotContain(t, loggedIn, `id="modalAccount"`)
	mustNotContain(t, loggedIn, `id="loginButton"`)
}
