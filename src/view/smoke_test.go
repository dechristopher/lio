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
	mustContain(t, out, `id="result-ratings"`) // game-over delta slot (JS-filled)

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
	gain := renderSmoke(t, clock("drewtest", "", "1650", 8))
	mustContain(t, gain, ">1650</span>")
	mustContain(t, gain, "clockRatingDelta win")
	mustContain(t, gain, "+8")

	loss := renderSmoke(t, clock("cdpplayer", "", "1500?", -8))
	mustContain(t, loss, "1500?")
	mustContain(t, loss, "clockRatingDelta loss")
	mustContain(t, loss, "-8")

	// zero delta (the live clocks): rating shown, no delta span
	none := renderSmoke(t, clock("drewtest", "", "1650", 0))
	mustContain(t, none, ">1650</span>")
	mustNotContain(t, none, "clockRatingDelta")

	// no rating (casual/anon/bot): no rating block at all
	empty := renderSmoke(t, clock("You", "", "", 0))
	mustNotContain(t, empty, "clockRating")

	// a bot seat: the persona glyph renders as the avatar and the generic CPU
	// icon is not; a human seat (empty glyph) is the reverse
	bot := renderSmoke(t, clock("Queen", "♛︎", "", 0))
	mustContain(t, bot, `class="clockBotGlyph"`)
	human := renderSmoke(t, clock("drewtest", "", "1650", 0))
	mustNotContain(t, human, "clockBotGlyph")
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

	// Phase 4 login-time second-factor step + method controls
	mustContain(t, loggedOut, `id="mfaStep"`)
	mustContain(t, loggedOut, `id="mfaCodeForm"`)
	mustContain(t, loggedOut, `id="mfaPasskeyBtn"`)
	mustContain(t, loggedOut, `data-mfa-alt="passkey"`)
	mustContain(t, loggedOut, `data-mfa-alt="recovery"`)

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
