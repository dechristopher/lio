package view

import (
	"context"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/dechristopher/lio/cache"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/learn"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/news"
	"github.com/dechristopher/lio/prefs"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/settings"
	"github.com/dechristopher/lio/store"
	"github.com/dechristopher/lio/sysinfo"
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

// seatNameRe matches the two places a room page prints a seat's name: the
// clocks (.clockName) and the match-timeline rows (.tl-name).
var seatNameRe = regexp.MustCompile(`<span class="(?:clockName|tl-name)">([^<]*)</span>`)

// seatNames returns every name the page labels a seat with, both clocks and
// both timeline rows. Seat assertions go through this rather than searching the
// whole page for ">Name</span>", which also matches static labels that are not
// seats — the rematch card's "You"/"Opponent" ready chips being the one that
// made two of these tests fail.
func seatNames(out string) []string {
	matches := seatNameRe.FindAllStringSubmatch(out, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

func mustSeatName(t *testing.T, out, name string) {
	t.Helper()
	if names := seatNames(out); !slices.Contains(names, name) {
		t.Errorf("no seat labeled %q; seats are %q", name, names)
	}
}

func mustNotSeatName(t *testing.T, out, name string) {
	t.Helper()
	if names := seatNames(out); slices.Contains(names, name) {
		t.Errorf("a seat is labeled %q; seats are %q", name, names)
	}
}

func TestRenderIndex(t *testing.T) {
	challenges := []message.OpenChallenge{{RoomID: "seek456", Variant: variant.OneTwoRapid, Color: "w"}}
	stats := message.SiteStats{LiveGames: 1, OpenChallenges: 1, Playing: 2}
	out := renderSmoke(t, Index(PageMeta("Free Online Octad"), challenges, stats, message.Community{}))
	mustContain(t, out, "<title>octad.gg • Free Online Octad</title>")
	mustContain(t, out, "Quick game")            // home heading (uppercased via CSS)
	mustContain(t, out, `id="createGameButton"`) // modal opener
	mustContain(t, out, `id="modalCreateGame"`)
	// the dialog's wiring lives in the cached lio-nav.js, not inline
	mustContain(t, out, "lio-nav")
	mustNotContain(t, out, "getElementById(\"modalCreateGame\")")

	// new home sections
	mustContain(t, out, `id="home-activity"`) // streamed activity region
	// the region is server-rendered once and streamed over /socket/home from
	// then on — it must carry no htmx polling of any kind
	// (arch/HOME_ACTIVITY_STREAMING.md)
	mustNotContain(t, out, "/home/activity")
	mustContain(t, out, "Open challenges") // challenges section
	mustContain(t, out, "/seek456")        // joinable challenge link
	mustContain(t, out, "What is Octad?")  // explainer
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

// The "What is Octad?" explainer is dismissible, but only by somebody with an
// account to remember the answer. Three viewers, three outcomes:
//
//   - anonymous: the card renders and carries no ×. It is the one thing on the
//     page that says what this site is, and there is nowhere to store a
//     dismissal anyway.
//   - signed in, never dismissed: the card renders with its ×, and the header
//     popover offers the switch that brings it back.
//   - signed in, dismissed: the card is gone server-side, and so is the demo
//     board's script — a member who put it away does not pay for it.
func TestHomeAboutPreference(t *testing.T) {
	page := func(v Viewer) string {
		return renderSmokeViewer(t, v,
			Index(PageMeta("Free Online Octad"), nil, message.SiteStats{}, message.Community{}))
	}

	anon := page(Viewer{AccountsEnabled: true})
	mustContain(t, anon, "What is Octad?")
	mustContain(t, anon, `id="homeAbout"`)
	mustNotContain(t, anon, "data-pref-off")                  // no dismiss control
	mustNotContain(t, anon, `data-pref="`+prefs.KeyHomeAbout) // and no switch

	member := page(Viewer{AccountsEnabled: true, LoggedIn: true, Username: "drewtest"})
	mustContain(t, member, `id="homeAbout"`)
	mustContain(t, member, `data-pref-off="`+prefs.KeyHomeAbout+`"`)
	// the popover switch, reflecting "shown"
	mustContain(t, member, `data-pref="`+prefs.KeyHomeAbout+`" checked`)
	mustContain(t, member, "lio-home-demo")

	hidden := page(Viewer{AccountsEnabled: true, LoggedIn: true, Username: "drewtest",
		Prefs: prefs.Snapshot{}.With(prefs.KeyHomeAbout, false)})
	mustNotContain(t, hidden, "What is Octad?</h2>")
	mustNotContain(t, hidden, `id="homeAbout"`)
	mustNotContain(t, hidden, `id="home-demo-board"`)
	mustNotContain(t, hidden, "lio-home-demo") // the script goes with the card
	// the switch stays — it is the way back — and reads as off
	mustContain(t, hidden, `data-pref="`+prefs.KeyHomeAbout+`">`)
	mustNotContain(t, hidden, `data-pref="`+prefs.KeyHomeAbout+`" checked`)
	// the rest of the page is untouched
	mustContain(t, hidden, `id="home-activity"`)
	mustContain(t, hidden, "octadground")
}

// An empty region still renders every section's shell, because the stream needs
// somewhere to put the first arrival (arch/HOME_ACTIVITY_STREAMING.md). What an
// empty site must not do is *show* those shells: the card and each section carry
// `hidden`, so a quiet site still looks quiet rather than like a stack of empty
// states.
func TestRenderHomeActivityEmpty(t *testing.T) {
	out := renderSmoke(t, HomeActivity(nil, message.SiteStats{}, message.Community{}))
	mustContain(t, out, `id="home-activity"`)
	mustContain(t, out, "No open challenges right now")
	// the challenge list exists but is hidden; the empty note is the visible one
	mustContain(t, out, `id="home-challenges-list" class="mt-3 flex flex-col gap-2" hidden`)
	mustNotContain(t, out, `id="home-challenges-empty" class="mt-3 text-sm text-fg-subtle" hidden`)
	// the players card and all three of its sections render hidden
	mustContain(t, out, `id="home-players" class="card" hidden`)
	mustContain(t, out, `id="home-following" hidden`)
	mustContain(t, out, `id="home-online" hidden`)
	mustContain(t, out, `id="home-arrivals" hidden`)
	// the client needs the lists themselves to patch into
	mustContain(t, out, `id="home-online-list"`)
	mustContain(t, out, `id="home-arrivals-list"`)
	mustContain(t, out, `id="home-following-list"`)
	// the stat tiles are patched by id, not rebuilt
	mustContain(t, out, `id="home-stat-playing"`)
	mustContain(t, out, `id="home-stat-live"`)
	mustContain(t, out, `id="home-stat-total"`)
}

// A seek leads with the person who created it: title badge, username and (for a
// rated seek) their rating. The time control is demoted to the sub-line.
func TestRenderOpenChallengeNamesCreator(t *testing.T) {
	challenges := []message.OpenChallenge{{
		RoomID:        "seek1",
		Variant:       variant.OneTwoRapid,
		Color:         "w",
		Rated:         true,
		CreatorName:   "nova",
		CreatorTitle:  title.Title{Code: "GM", Name: "Grandmaster"},
		CreatorRating: "1712",
	}}
	out := renderSmoke(t, HomeActivity(challenges, message.SiteStats{}, message.Community{}))
	mustContain(t, out, ">nova<")
	mustContain(t, out, `class="player-title"`)
	mustContain(t, out, ">GM<")
	mustContain(t, out, `>1712<`)
	mustContain(t, out, "/seek1")
	// the variant still shows, on the secondary line
	mustContain(t, out, variant.OneTwoRapid.Name)
}

// An anonymous creator is named "Anonymous" rather than silently rendering a
// nameless row — the contrast with a named seek is the point.
func TestRenderOpenChallengeAnonymousCreator(t *testing.T) {
	challenges := []message.OpenChallenge{{RoomID: "seek2", Variant: variant.OneTwoRapid, Color: "b"}}
	out := renderSmoke(t, HomeActivity(challenges, message.SiteStats{}, message.Community{}))
	mustContain(t, out, ">Anonymous<")
	mustNotContain(t, out, `class="rating-chip`)
}

func TestRenderPlayersCard(t *testing.T) {
	c := message.Community{
		Online: []message.OnlineMember{
			{Username: "zed", Online: true, Playing: true},
			{Username: "nova", Online: true, Title: title.Title{Code: "WFM", Name: "Woman FIDE Master"}},
		},
		Anon:   2,
		Newest: []message.NewMember{{Username: "pawnstar", Joined: time.Now().Add(-48 * time.Hour)}},
	}
	out := renderSmoke(t, HomeActivity(nil, message.SiteStats{}, c))

	mustContain(t, out, ">Active<")
	mustContain(t, out, `href="/@/zed"`)  // chips link to the player page
	mustContain(t, out, `href="/@/nova"`) // ...
	mustContain(t, out, ">playing<")      // seated marker
	mustContain(t, out, ">WFM<")          // title badge rides along
	mustContain(t, out, "Arrivals")       // second section
	mustContain(t, out, `href="/@/pawnstar"`)

	// both rosters are wrapping chip lists, not full-width rows: a name eight
	// characters long must not cost a whole line. Three lists render, not two —
	// the Following shell is always present (and here hidden), because the
	// stream patches into it (arch/HOME_ACTIVITY_STREAMING.md).
	if n := strings.Count(out, `class="chip-list"`); n != 3 {
		t.Errorf("chip lists = %d, want 3 (following + online + newest)", n)
	}
	mustContain(t, out, `id="home-following" hidden`)
	mustNotContain(t, out, `class="roster-row"`) // rows are the leaderboard's shape
	// the seated chip is tinted as well as tagged, so the state is not carried
	// by the tag alone
	mustContain(t, out, "roster-chip is-playing")
	// the join date is a chip-sized token, with the prose form in the tooltip
	mustContain(t, out, ">2d<")
	mustContain(t, out, `title="joined 2 days ago"`)

	// the anonymous footnote counts them and, for a logged-out viewer, says so
	mustContain(t, out, "2 anonymous visitors (including you)")
}

// An arrival who is not on the site carries no presence dot. Most arrivals are
// people who registered and left, so a dot on every chip would be a marker that
// usually says nothing — it is a distinction, exactly as the state tag is.
func TestRenderArrivalOfflineHasNoDot(t *testing.T) {
	c := message.Community{
		Newest: []message.NewMember{{Username: "pawnstar", Joined: time.Now().Add(-48 * time.Hour)}},
	}
	out := renderSmoke(t, HomeActivity(nil, message.SiteStats{}, c))
	mustContain(t, out, `href="/@/pawnstar"`)
	mustContain(t, out, ">2d<")
	// no roster is rendered here, so the only chip on the page is the arrival —
	// and it must carry no dot
	mustNotContain(t, out, `class="roster-dot"`)
	mustNotContain(t, out, "roster-chip-tag")
}

// An arrival who is online gets the full roster treatment: the presence dot, the
// state tag when they cannot play, and the sword when they can. A new player who
// is here and free is the best challenge target the page has.
func TestRenderArrivalOnlineGetsRosterTreatment(t *testing.T) {
	joined := time.Now().Add(-48 * time.Hour)
	c := message.Community{
		Newest: []message.NewMember{
			{Username: "freebie", Joined: joined, Online: true},
			{Username: "atboard", Joined: joined, Online: true, Playing: true, Busy: true},
			{Username: "gone", Joined: joined},
		},
	}
	out := renderSmokeViewer(t, Viewer{
		LoggedIn: true, AccountsEnabled: true, Username: "watcher",
	}, HomeActivity(nil, message.SiteStats{}, c))

	// the two who are here carry dots; the tag marks the one who cannot play
	if n := strings.Count(out, `class="roster-dot"`); n != 2 {
		t.Errorf("presence dots = %d, want 2 (only the online arrivals)", n)
	}
	mustContain(t, out, ">playing<")
	mustContain(t, out, "roster-chip is-playing is-busy")
	// the sword is offered for the free arrival only — never for the one at a
	// board, and never for somebody who is not here at all
	if n := strings.Count(out, `class="roster-challenge"`); n != 1 {
		t.Errorf("swords = %d, want 1 (the online, free arrival)", n)
	}
	mustContain(t, out, `data-challenge="freebie"`)
	mustNotContain(t, out, `data-challenge="atboard"`)
	mustNotContain(t, out, `data-challenge="gone"`)
	// the join token still rides along on every arrival, online or not
	if n := strings.Count(out, ">2d<"); n != 3 {
		t.Errorf("join tokens = %d, want 3 (one per arrival)", n)
	}
}

// The same roster seen by a member drops the "(including you)" aside — they are
// not one of the anonymous visitors.
// TestRosterChallengeGating: the sword is offered only to players who could
// actually accept one right now (arch/NOTIFICATIONS.md Phase 2). A control that
// is present but certain to fail is worse than one that is absent.
func TestRosterChallengeGating(t *testing.T) {
	c := message.Community{
		Online: []message.OnlineMember{
			{Username: "free", Online: true},                             // browsing: challengeable
			{Username: "gamer", Online: true, Playing: true, Busy: true}, // at a board
			{Username: "seeker", Online: true, Busy: true},               // waiting in a room of their own
			{Username: "drewtest", Online: true},                         // the viewer themselves
		},
	}

	out := renderSmokeViewer(t, Viewer{LoggedIn: true, Username: "drewtest", AccountsEnabled: true},
		HomeActivity(nil, message.SiteStats{}, c))
	mustContain(t, out, `data-challenge="free"`)
	// busy in either sense is unavailable — a player waiting in their own
	// challenge is not playing, but they are already committed to a game
	mustNotContain(t, out, `data-challenge="gamer"`)
	mustNotContain(t, out, `data-challenge="seeker"`)
	// nobody is offered the chance to challenge themselves; the comparison is
	// case-insensitive because a username's display case is not its identity
	mustNotContain(t, out, `data-challenge="drewtest"`)

	// an anonymous visitor cannot challenge anyone: there is nobody to address
	// the invitation from
	anon := renderSmokeViewer(t, Viewer{AccountsEnabled: true},
		HomeActivity(nil, message.SiteStats{}, c))
	mustNotContain(t, anon, "data-challenge")
}

// TestRosterBusyState: an unavailable player reads as unavailable three ways at
// once — the sword is gone, the presence dot turns amber, and a tag says which
// kind of busy they are. The tag matters because the state must not rest on the
// dot's color alone, and because it answers the question the missing sword
// raises.
func TestRosterBusyState(t *testing.T) {
	c := message.Community{
		Online: []message.OnlineMember{
			{Username: "free", Online: true},
			{Username: "gamer", Online: true, Playing: true, Busy: true},
			{Username: "seeker", Online: true, Busy: true},
		},
	}
	out := renderSmokeViewer(t, Viewer{LoggedIn: true, Username: "drewtest", AccountsEnabled: true},
		HomeActivity(nil, message.SiteStats{}, c))

	// two of the three are busy, and only those two carry the amber dot
	if n := strings.Count(out, "is-busy"); n != 2 {
		t.Errorf("is-busy chips = %d, want 2", n)
	}
	// a player at a board and one waiting in their own room are both busy, but
	// they are not the same thing and the roster says which
	mustContain(t, out, ">playing<")
	mustContain(t, out, ">waiting<")
	// the available player is the only one offered a sword
	if n := strings.Count(out, "data-challenge="); n != 1 {
		t.Errorf("challenge buttons = %d, want 1", n)
	}
}

func TestRenderPlayersCardAnonNoteForMember(t *testing.T) {
	c := message.Community{Online: []message.OnlineMember{{Username: "nova", Online: true}}, Anon: 1}
	out := renderSmokeViewer(t, Viewer{LoggedIn: true, Username: "nova"},
		HomeActivity(nil, message.SiteStats{}, c))
	mustContain(t, out, "1 anonymous visitor")
	mustNotContain(t, out, "including you")
}

func TestRenderLeaderboard(t *testing.T) {
	// categories are variant HTMLNames, resolved to a speed for display
	c := message.Community{Top: []message.RatedMember{
		{Username: "nova", Rating: 1712, Category: "half-one-blitz-deploy", Games: 40},
		{Username: "zed", Rating: 1604, Category: "one-two-rapid-deploy", Games: 12},
	}}
	out := renderSmoke(t, Index(PageMeta("home"), nil, message.SiteStats{}, c))
	mustContain(t, out, "Top rated")
	mustContain(t, out, "1712 blitz")
	mustContain(t, out, "1604 rapid")
	mustContain(t, out, `href="/@/nova"`)
	// nova ranks above zed
	if strings.Index(out, "/@/nova") > strings.Index(out, "/@/zed") {
		t.Error("leaderboard rows out of rating order")
	}
}

// The leaderboard card is omitted entirely when nothing qualifies (a new site,
// or PG-less local dev) rather than rendering an empty board.
func TestRenderLeaderboardOmittedWhenEmpty(t *testing.T) {
	out := renderSmoke(t, Index(PageMeta("home"), nil, message.SiteStats{}, message.Community{}))
	mustNotContain(t, out, "Top rated")
}

// The home account pitch renders for an anonymous viewer where accounts exist,
// and for nobody else.
func TestRenderHomeWelcomeGating(t *testing.T) {
	page := func(v Viewer) string {
		return renderSmokeViewer(t, v, Index(PageMeta("home"), nil, message.SiteStats{}, message.Community{}))
	}

	anon := page(Viewer{AccountsEnabled: true})
	mustContain(t, anon, `id="homeCta"`)
	mustContain(t, anon, `id="homeCtaCreate"`)
	mustContain(t, anon, "data-open-register") // opens the modal's register tab
	mustContain(t, anon, `id="homeCtaDismiss"`)

	member := page(Viewer{AccountsEnabled: true, LoggedIn: true, Username: "nova"})
	mustNotContain(t, member, `id="homeCta"`)

	// PG-less local dev: no accounts to create
	noAccounts := page(Viewer{})
	mustNotContain(t, noAccounts, `id="homeCta"`)
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
	mustSeatName(t, out, "Knight")
	mustContain(t, out, `class="clockBotGlyph"`)
	mustContain(t, out, `class="tl-seat"`)
	mustContain(t, out, `class="tl-seat-glyph"`)
	mustSeatName(t, out, "Anonymous")
	mustNotSeatName(t, out, "You")
	mustNotSeatName(t, out, "PLAYER")
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
	mustSeatName(t, out, "drewtest")
	mustSeatName(t, out, "cdpplayer")
	mustNotSeatName(t, out, "You")
	mustNotSeatName(t, out, "Anonymous")

	// a logged-in viewer facing an anonymous opponent: opponent reads
	// "Anonymous", the viewer's own seat their username
	p2 := message.RoomTemplatePayload{
		RoomID: "abc", PlayerColor: "w", OpponentColor: "b",
		WhiteName: "drewtest", Variant: variant.HalfOneBlitz,
		VariantName: "Half One blitz",
	}
	out2 := renderSmoke(t, Room(RoomMeta(p2), p2))
	mustSeatName(t, out2, "drewtest")
	mustSeatName(t, out2, "Anonymous")

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
	gain := renderSmoke(t, clock(title.Title{}, "drewtest", "", "1650", 8, ""))
	mustContain(t, gain, ">1650</span>")
	mustContain(t, gain, "clockRatingDelta win")
	mustContain(t, gain, "+8")

	loss := renderSmoke(t, clock(title.Title{}, "cdpplayer", "", "1500?", -8, ""))
	mustContain(t, loss, "1500?")
	mustContain(t, loss, "clockRatingDelta loss")
	mustContain(t, loss, "-8")

	// zero delta (the live clocks): rating shown, no delta span
	none := renderSmoke(t, clock(title.Title{}, "drewtest", "", "1650", 0, ""))
	mustContain(t, none, ">1650</span>")
	mustNotContain(t, none, "clockRatingDelta")

	// no rating (casual/anon/bot): no rating block at all
	empty := renderSmoke(t, clock(title.Title{}, "You", "", "", 0, ""))
	mustNotContain(t, empty, "clockRating")

	// a bot seat: the persona glyph renders as the avatar and the generic CPU
	// icon is not; a human seat (empty glyph) is the reverse
	bot := renderSmoke(t, clock(title.Title{}, "Queen", "♛︎", "", 0, ""))
	mustContain(t, bot, `class="clockBotGlyph"`)
	human := renderSmoke(t, clock(title.Title{}, "drewtest", "", "1650", 0, ""))
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
		title.Title{Code: "GM", Name: "Grandmaster"}, "drewtest", "", "1650", 0, ""))
	mustContain(t, titled, `class="player-title"`)
	mustContain(t, titled, ">GM</span>")
	mustContain(t, titled, `title="Grandmaster"`)

	nameless := renderSmoke(t, clock(
		title.Title{Code: "OG"}, "drewtest", "", "1650", 0, ""))
	mustContain(t, nameless, `title="OG"`)

	untitled := renderSmoke(t, clock(title.Title{}, "drewtest", "", "1650", 0, ""))
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

	// challenger card, anonymous creator: fallback name + the side the
	// challenger plays (joiner takes black → challenger is white)
	mustContain(t, out, `class="challenger-card"`)
	mustContain(t, out, "Anonymous player")
	mustContain(t, out, "Plays White")

	// the joiner decides whether to accept, so the match spec must precede the
	// button that commits them to it (the panels stack in DOM order on a phone)
	if strings.Index(out, `class="spec"`) > strings.Index(out, `class="wait-join-cta"`) {
		t.Error("join CTA rendered before the match spec")
	}

	// named + rated challenger: username + rating chip
	p.CreatorName = "pregametest"
	p.CreatorRating = "1500?"
	named := renderSmoke(t, Room(RoomMeta(p), p))
	mustContain(t, named, "pregametest")
	mustContain(t, named, `class="rating-chip"`)

	// random-color room: the challenger's side is hidden
	p.BlindColor = true
	blind := renderSmoke(t, Room(RoomMeta(p), p))
	mustContain(t, blind, "Plays a random color")
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

// TestArchiveReportControl: a returning player of an archived game gets the
// report control *and* the dialog it opens; everyone else gets neither. The
// server resolves eligibility into ReportTarget, so the page's only job is to
// render both halves together or not at all — a control with no dialog behind
// it would be a dead button, and a dialog with no control is dead weight on
// every archive page on the site.
func TestArchiveReportControl(t *testing.T) {
	m := ArchiveModel{
		RoomID: "abc", VariantName: "½ + 1", N: 1, Count: 1,
		Orientation: "w", TopName: "cdpplayer", BottomName: "drewtest",
		EndedDate: "Jan 1, 2026",
		Data:      ArchiveData{GameID: "g-uuid", N: 1, Count: 1},
	}

	plain := renderSmoke(t, RoomArchive(ArchiveMeta(m), m))
	mustNotContain(t, plain, "data-report-target")
	mustNotContain(t, plain, `id="modalReport"`)
	mustNotContain(t, plain, "lio-report")

	m.ReportTarget = "cdpplayer"
	out := renderSmoke(t, RoomArchive(ArchiveMeta(m), m))
	mustContain(t, out, `data-report-target="cdpplayer"`)
	mustContain(t, out, "Report cdpplayer")
	mustContain(t, out, `id="modalReport"`)
	mustContain(t, out, "lio-report")
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

// TestNoHTMLComments locks the comment convention: notes in .templ files use
// templ's own "//" comments, which the generator drops, not "<!-- -->" markup
// comments, which it copies verbatim into the response. The notes explain
// internals (JS hooks, state classes, why a node exists) and are for readers of
// the source, not for the wire — shipping them cost the room page ~8KB, 15% of
// its HTML. Rendering every page component is impractical here, so this covers
// the pages that pull in the shared header, footer and board markup.
func TestNoHTMLComments(t *testing.T) {
	p := message.RoomTemplatePayload{
		RoomID:      "abc",
		PlayerColor: "w", OpponentColor: "b",
		VariantName: "Half One blitz",
		Variant:     variant.HalfOneBlitz,
	}
	pages := map[string]templ.Component{
		"index": Index(PageMeta("Free Online Octad"), nil, message.SiteStats{}, message.Community{}),
		"room":  Room(RoomMeta(p), p),
		"about": About(PageMeta("About"), "board"),
		"news":  News(PageMeta("News"), 1),
		"db":    DB(PageMeta("Game Database")),
		"learn": Learn(PageMeta("Learn to play"), &learn.Lessons[0]),
		"404":   NotFound(PageMeta("404")),
	}
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			mustNotContain(t, renderSmoke(t, page), "<!--")
		})
	}

	// the logged-in header chrome (profile popover, notifications) renders only
	// with a Viewer in the context, so the anonymous pages above never reach it
	t.Run("viewer", func(t *testing.T) {
		v := Viewer{AccountsEnabled: true, Username: "drewtest"}
		mustNotContain(t, renderSmokeViewer(t, v, NotFound(PageMeta("404"))), "<!--")
	})
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
			// built through the constructor so the test exercises the real
			// category → label resolution, not a hand-filled struct
			NewRatingView("half-one-blitz", "1653", 12),
		},
		Total: NewRecordView(db.Record{Games: 3, Wins: 2, Draws: 0, Losses: 1}),
		Lifetime: NewLifetimeView(
			db.Record{Games: 3},
			db.Lifetime{
				FirstGame: time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC),
				Played:    5 * time.Hour,
			}),
		Variants: []VariantRecordView{{Name: "½ + 1", Group: "blitz",
			RecordView: NewRecordView(db.Record{Games: 3, Wins: 2, Losses: 1})}},
		Bots: []BotRecordView{{Persona: "Queen", Glyph: "♛︎",
			RecordView: NewRecordView(db.Record{Games: 1, Losses: 1}),
			Bar:        NewWDLBar(db.Record{Games: 1, Losses: 1})}},
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
	m.BotLadder = NewBotLadder(m.Bots)
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	mustContain(t, out, ">drewtest</span>")
	mustContain(t, out, `class="player-title"`)
	mustContain(t, out, ">OG</span>")
	mustContain(t, out, "Member since")
	mustContain(t, out, "March 2026")
	mustContain(t, out, "1653") // rating tile
	// the tile shows the resolved time control, never the raw HTMLName key.
	//
	// Asserted against rendered *text* rather than the whole document: the page
	// now carries the create-game dialog too (the Challenge button opens it —
	// arch/NOTIFICATIONS.md Phase 2), and its time-control cards legitimately
	// carry HTMLNames in data-variant, which is how the form submits a choice.
	// The guarantee that matters is that the key is never shown to anybody.
	mustContain(t, out, "½ + 1")
	mustContain(t, out, "blitz")
	mustNotContain(t, out, ">half-one-blitz<")
	// the hero's two figures, as tiles rather than prose
	mustContain(t, out, `class="hero-figure-value"`)
	mustContain(t, out, ">3<")  // games
	mustContain(t, out, ">5h<") // played
	mustNotContain(t, out, "first game")
	mustContain(t, out, "All games") // lifetime record row
	mustContain(t, out, "Versus the computer")
	// the ladder's glyphs are wrapped so their baseline can be corrected —
	// these come from a fallback font that sets them low
	mustContain(t, out, `class="rung-glyph piece-glyph"`)
	mustContain(t, out, "Queen")
	mustContain(t, out, `href="/abc123/1"`) // game links into the archive
	mustContain(t, out, "cdpplayer")
	// the page title carries the title code, as the OG/meta treatment does
	// the title is bracketed in bare-text contexts (tab, search result, share
	// card) where the page's badge styling cannot separate it from the name
	mustContain(t, out, templ.EscapeString("[OG] drewtest"))
	// no moderation UI for an ordinary viewer
	mustNotContain(t, out, `id="modForm"`)
	mustNotContain(t, out, "lio-mod")
}

// TestRenderProfileFollow covers the follow block's gating (arch/FOLLOWING.md
// Phase 1): the counts render on their own, the control only for a viewer who
// may press it, and the button carries its state in words as well as in a
// class — a control whose only difference is a tint says nothing to somebody
// who cannot see the tint.
func TestRenderProfileFollow(t *testing.T) {
	// counts alone: a visitor who cannot act still sees who follows whom
	m := profileFixture()
	m.Follow = NewFollowView(db.FollowCounts{Followers: 12, Following: 1})
	out := renderSmoke(t, Profile(ProfileMeta(m), m))
	mustContain(t, out, "12 followers")
	mustContain(t, out, "1 following") // "following" does not inflect
	mustNotContain(t, out, "data-follow=")

	// a single follower is not "1 followers"
	m.Follow = NewFollowView(db.FollowCounts{Followers: 1})
	out = renderSmoke(t, Profile(ProfileMeta(m), m))
	mustContain(t, out, "1 follower<")

	// nothing to say, and nobody who could act: the line is absent rather than
	// rendered as a row of zeroes
	m.Follow = NewFollowView(db.FollowCounts{})
	out = renderSmoke(t, Profile(ProfileMeta(m), m))
	mustNotContain(t, out, "0 followers")
	mustNotContain(t, out, `class="hero-social"`)

	// the control renders the counts with it, even at zero: the button's whole
	// effect is on the number beside it
	m.Follow.Control = true
	out = renderSmoke(t, Profile(ProfileMeta(m), m))
	mustContain(t, out, `class="hero-social"`)
	mustContain(t, out, "0 followers")
	mustContain(t, out, `data-follow="drewtest"`)
	mustContain(t, out, `aria-pressed="false"`)
	mustContain(t, out, ">Follow<")
	mustNotContain(t, out, "is-following")

	// already following: the state is on the class, on aria-pressed, and in the
	// visible label. Both labels stay in the DOM so the button cannot resize.
	m.Follow.IsFollowing = true
	out = renderSmoke(t, Profile(ProfileMeta(m), m))
	mustContain(t, out, "is-following")
	mustContain(t, out, `aria-pressed="true"`)
	mustContain(t, out, ">Following<")
	mustContain(t, out, ">Follow<")

	// a closed account publishes nothing, the follow block included
	m.Closed = true
	out = renderSmoke(t, Profile(ProfileMeta(m), m))
	mustNotContain(t, out, `class="hero-social"`)
	mustNotContain(t, out, "data-follow=")
}

// TestRenderProfileFollowLists covers the counts as list openers and the dialog
// they open (arch/FOLLOWING.md Phase 2). The rows are not asserted here — they
// are rendered in the client, on purpose, so there is nothing in the server's
// output to check beyond the frame.
func TestRenderProfileFollowLists(t *testing.T) {
	m := profileFixture()
	m.Follow = NewFollowView(db.FollowCounts{Followers: 31, Following: 3})
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	// each count opens its own list
	mustContain(t, out, `data-follow-open="followers"`)
	mustContain(t, out, `data-follow-open="following"`)
	// the dialog is mounted with the account it belongs to; the viewer is empty
	// in a smoke render, which is the signed-out case
	mustContain(t, out, `id="modalFollow"`)
	mustContain(t, out, `data-follow-owner="drewtest"`)
	mustContain(t, out, "Followers")
	mustContain(t, out, "Load more")
	mustContain(t, out, "lio-follow.")

	// A count with nothing behind it is disabled rather than swapped for plain
	// text, so a first follower does not change the shape of the line.
	m.Follow = NewFollowView(db.FollowCounts{Followers: 4})
	out = renderSmoke(t, Profile(ProfileMeta(m), m))
	if strings.Count(out, `data-follow-open`) != 2 {
		t.Fatal("both counts must render as controls, populated or not")
	}
	mustContain(t, out, `data-following-count disabled`)
	mustNotContain(t, out, `data-follower-count disabled`)

	// a closed account has no counts, so it needs no dialog either
	m.Closed = true
	out = renderSmoke(t, Profile(ProfileMeta(m), m))
	mustNotContain(t, out, `id="modalFollow"`)
	mustNotContain(t, out, "lio-follow.")
}

// TestRenderProfileRatingHistory covers the rating curve on the page: the SVG
// renders server-side, the tiles become selectors, exactly one panel opens, and
// every plotted value is also reachable without hovering.
func TestRenderProfileRatingHistory(t *testing.T) {
	m := profileFixture()
	hist := []db.RatingSeries{{
		Category: "half-one-blitz",
		Points: []db.RatingPoint{
			{Day: chartNow.AddDate(0, 0, -3), Rating: 1500, Provisional: true},
			{Day: chartNow.AddDate(0, 0, -2), Rating: 1610, Provisional: false},
		},
	}}
	m.Charts = NewRatingCharts(hist, m.Ratings, chartNow)
	m.HasCharts = true
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	mustContain(t, out, "Rating history")
	mustContain(t, out, `class="chart-svg"`)
	mustContain(t, out, `data-chart-panel="half-one-blitz"`)
	// the tile is the selector, so it must carry the category the panel keys on
	mustContain(t, out, `data-chart-tab="half-one-blitz"`)
	mustContain(t, out, "is-selectable")
	// a provisional prefix means two strokes, one of them dashed by CSS
	mustContain(t, out, "chart-line-prov")
	mustContain(t, out, "dashed while provisional")
	// values are never hover-only: a table carries every point. sr-only sits on
	// a wrapping div, never on the table itself — a <table> ignores the width,
	// height and overflow that class relies on, which left the whole table in
	// the page with only clip-path hiding it.
	mustContain(t, out, "1610")
	mustContain(t, out, `<div class="sr-only">`)
	mustNotContain(t, out, `<table class="sr-only">`)
	// the page ships the behaviour that drives all of it
	mustContain(t, out, "lio-profile")

	// exactly one panel opens, so the section is readable with JS disabled
	if n := strings.Count(out, "chart-panel is-active"); n != 1 {
		t.Errorf("got %d active panels, want exactly 1", n)
	}
}

// TestRenderProfileEmpty locks the placeholder behaviour for a brand-new
// account: every statistics section still renders its heading, and each shows a
// placeholder describing what will appear there instead of collapsing to
// nothing. A section that silently vanishes reads as a broken page to exactly
// the players who see this state.
func TestRenderProfileEmpty(t *testing.T) {
	m := profileFixture()
	m.Ratings, m.Variants, m.Bots, m.Games = nil, nil, nil, nil
	m.Total = NewRecordView(db.Record{})
	m.Lifetime = NewLifetimeView(db.Record{}, db.Lifetime{})
	out := renderSmoke(t, Profile(ProfileMeta(m), m))

	// headings survive with no data behind them
	mustContain(t, out, "Record")
	mustContain(t, out, "Recent games")
	// each is a placeholder that says what is coming
	mustContain(t, out, `class="stat-empty"`)
	mustContain(t, out, "Wins, draws and losses appear here")
	mustContain(t, out, "Finished games land here")
	// the hero states the unrated case in place of a rating row
	mustContain(t, out, "No rating yet")
	// no zero-filled record rows masquerading as a real tally
	mustNotContain(t, out, "All games")
	// an account with no games claims no lifetime facts. Asserted on the exact
	// phrases NewLifetimeView emits rather than the bare word "played", which
	// also appears in placeholder prose and made this a trap for future copy.
	mustNotContain(t, out, "hero-figure-value")
}

// TestStatPlaceholderMeter covers the progress meter a threshold-gated section
// shows: it appears only while short of the threshold, and the fill never
// exceeds full or vanishes entirely at zero.
func TestStatPlaceholderMeter(t *testing.T) {
	short := StatPlaceholder{Copy: "c", Have: 2, Need: 5, Unit: "rated games"}
	if !short.Meter() {
		t.Error("meter should show while short of the threshold")
	}
	if got := short.Progress(); got != "2 of 5 rated games" {
		t.Errorf("Progress() = %q", got)
	}
	if got := short.Width(); got != "40%" {
		t.Errorf("Width() = %q, want 40%%", got)
	}
	// met, and over-met, both stop showing the meter
	for _, have := range []int64{5, 9} {
		p := StatPlaceholder{Have: have, Need: 5}
		if p.Meter() {
			t.Errorf("meter should be hidden at have=%d need=5", have)
		}
	}
	// an empty meter still renders a sliver, so it reads as real-and-empty
	if got := (StatPlaceholder{Have: 0, Need: 5}).Width(); got != "2%" {
		t.Errorf("Width() at zero = %q, want 2%%", got)
	}
	// no threshold means no meter at all
	if (StatPlaceholder{Copy: "c"}).Meter() {
		t.Error("meter should be hidden with no threshold")
	}
}

// TestSortRatings locks the rating tile order: canonical time-control order
// from pools, with categories no longer curated sorted last rather than dropped.
func TestSortRatings(t *testing.T) {
	rs := []RatingView{
		NewRatingView("legacy-retired-pool", "1500", 1),
		NewRatingView("three-five-rapid-deploy", "1600", 5),
		NewRatingView("quarter-zero-bullet-deploy", "1700", 9),
	}
	SortRatings(rs)

	want := []string{"quarter-zero-bullet-deploy", "three-five-rapid-deploy", "legacy-retired-pool"}
	for i, w := range want {
		if rs[i].Category != w {
			t.Errorf("position %d = %q, want %q", i, rs[i].Category, w)
		}
	}
	// an unknown category keeps its raw key visible rather than rendering blank
	if rs[2].Label != "legacy-retired-pool" {
		t.Errorf("unknown category label = %q", rs[2].Label)
	}
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
//
// The console is four pages now, so a test picks the one it is about by setting
// Tab; the fixture carries every tab's data at once, which no real handler
// does — each of those loads only what its own page shows.
func systemFixture() SystemModel {
	return SystemModel{
		Tab:      TabOverview,
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

// systemTab renders one page of the console from the shared fixture.
func systemTab(t *testing.T, tab SystemTab, isAdmin bool) string {
	t.Helper()
	m := systemFixture()
	m.Tab, m.IsAdmin = tab, isAdmin
	return renderSmoke(t, System(SystemMeta(), m))
}

// TestRenderSystemConsole covers the overview: an admin sees the controls
// beside the live picture, and a moderator sees neither them nor the modal.
func TestRenderSystemConsole(t *testing.T) {
	admin := systemTab(t, TabOverview, true)
	mustContain(t, admin, `id="settingsForm"`)
	mustContain(t, admin, `data-setting="maintenance"`)
	mustContain(t, admin, `data-setting="registrationOpen"`)
	mustContain(t, admin, `data-setting="notice"`)
	mustContain(t, admin, `name="reason"`) // every change carries one

	// The log has the page to itself. Its rows are scanned across rather than
	// read down, which is why it is the one tab with no second column.
	log := systemTab(t, TabLog, true)
	mustContain(t, log, "Audit log")
	mustContain(t, log, "2 entries on record")
	mustContain(t, log, "spammer99")
	mustContain(t, log, `href="/@/spammer99"`) // targets link to their page
	mustContain(t, log, "site-wide")           // site-level entry, no target
	mustNotContain(t, log, `id="settingsForm"`)

	// A moderator gets the live picture and the log, and none of the controls.
	modOverview := systemTab(t, TabOverview, false)
	mustContain(t, modOverview, "Right now")
	mustNotContain(t, modOverview, `id="settingsForm"`)
	mustNotContain(t, modOverview, `id="broadcastForm"`)
	mustContain(t, systemTab(t, TabLog, false), "Audit log")

	// The operator message composer is a moderator tool, not an admin one: it
	// writes to one account, like every other action a moderator takes, rather
	// than changing something every visitor sees (arch/NOTIFICATIONS.md
	// Phase 3). It ships to both, and it is a search box rather than a list of
	// accounts — the site has more players than any picker could hold.
	for name, isAdmin := range map[string]bool{"admin": true, "moderator": false} {
		out := systemTab(t, TabPeople, isAdmin)
		if !strings.Contains(out, `id="msgSearch"`) {
			t.Errorf("%s is missing the message composer", name)
		}
		if !strings.Contains(out, `id="msgBody"`) {
			t.Errorf("%s is missing the message body field", name)
		}
		if !strings.Contains(out, "Message a player") {
			t.Errorf("%s is missing the composer heading", name)
		}
	}

	// empty log distinguishes "nothing yet" from "nothing matches"
	mustContain(t, renderSmoke(t, System(SystemMeta(), SystemModel{Tab: TabLog})),
		"Nothing has been actioned yet.")
	empty := AuditFeed{Filtered: true, Query: "nobody", Page: 1, Pages: 1}
	mustContain(t, renderSmoke(t, AuditFeedBody(empty)), "No actions match that search.")
}

// TestSystemTabs: the strip names every page this viewer may open and no page
// they may not, and marks the current one on two channels rather than colour
// alone.
func TestSystemTabs(t *testing.T) {
	// Every moderator gets the same three pages. The admin-only cards are
	// unrendered within a page rather than hidden behind a tab of their own.
	for name, isAdmin := range map[string]bool{"admin": true, "moderator": false} {
		out := systemTab(t, TabOverview, isAdmin)
		for _, path := range []string{"/system", "/system/people", "/system/log"} {
			if !strings.Contains(out, `href="`+path+`"`) {
				t.Errorf("%s is missing the %s tab", name, path)
			}
		}
		if strings.Contains(out, `href="/system/site"`) {
			t.Errorf("%s still links the folded-in Site tab", name)
		}
	}
	mustContain(t, systemTab(t, TabOverview, true),
		`class="sys-tab is-active" href="/system" aria-current="page"`)

	if len(SystemTabs) != 3 {
		t.Errorf("SystemTabs = %v, want three pages", SystemTabs)
	}
	if got := TabOverview.Path(); got != "/system" {
		t.Errorf("the overview must stay the bare path, got %q", got)
	}
}

// TestSystemTwoColumns: above md the cards sit two abreast, and each column is
// an explicit stack so the single-column order on a telephone is a decision
// rather than whatever the wrap produced.
func TestSystemTwoColumns(t *testing.T) {
	overview := systemTab(t, TabOverview, true)
	if n := strings.Count(overview, `class="sys-col"`); n != 2 {
		t.Errorf("the overview has %d columns, want 2", n)
	}

	// Active notices is the page's alert line and runs full width above the
	// grid, so it is first at every size rather than first in whichever column
	// somebody happens to read.
	if strings.Index(overview, "Active notices") > strings.Index(overview, `class="sys-grid"`) {
		t.Error("active notices must sit above the columns, not inside one")
	}

	// The two columns are one reading order folded in half: below md the left
	// is read out in full before the right, so the split has to produce the
	// order an operator wants. Anything that reorders these cards changes what
	// somebody sees first on a telephone.
	order := []string{
		"Active notices",                                     // full width, above both
		"Right now", `id="systemStats"`, `id="settingsForm"`, // this instance and its switches
		`id="broadcastForm"`, "Sent broadcasts", // everything outbound
	}
	for i := 1; i < len(order); i++ {
		if strings.Index(overview, order[i]) < strings.Index(overview, order[i-1]) {
			t.Errorf("%s renders before %s, which changes the single-column order",
				order[i], order[i-1])
		}
	}

	// The feedback inbox and the composer that answers it stay adjacent, in that
	// order, in one column: feedback has no reply thread, and writing to the
	// player is the nearest thing to an answer.
	people := systemTab(t, TabPeople, true)
	if strings.Index(people, `id="msgSearch"`) < strings.Index(people, "Feedback") {
		t.Error("the message composer must stay under the feedback inbox")
	}

	// A moderator's overview has one card, so it is not put in a half-width
	// column with nothing beside it.
	if n := strings.Count(systemTab(t, TabOverview, false), `class="sys-col"`); n != 0 {
		t.Error("a moderator's overview should render as one column, not a grid")
	}
}

// TestRenderInstancePanel covers the perf panel: admins get it, moderators do
// not (it names infrastructure, which is not their business), and the polling
// contract that keeps it live without collapsing the disclosure holds.
func TestRenderInstancePanel(t *testing.T) {
	m := systemFixture()
	m.Stats = SystemStatsOf(
		sysinfo.Runtime{Version: "v9.9.9+abc", Env: "local", GoVer: "go1.22.0",
			Platform: "darwin/arm64", NumCPU: 8, GOMAXPROCS: 8, Goroutines: 42},
		db.Stats{Configured: true, Reachable: true, MaxConns: 4},
		cache.Stats{Configured: true, Reachable: true, Addr: "localhost:6379"},
		store.Stats{Configured: true, Reachable: true, Endpoint: "obj.example", Bucket: "pgn"},
	)

	admin := renderSmoke(t, System(SystemMeta(), m))
	mustContain(t, admin, `id="systemStats"`)
	mustContain(t, admin, "v9.9.9+abc")
	mustContain(t, admin, "Goroutines")
	mustContain(t, admin, "Postgres")
	mustContain(t, admin, "localhost:6379")
	mustContain(t, admin, "obj.example / pgn")
	// the poll refreshes the sample but pauses in a background tab
	mustContain(t, admin, `hx-get="/system/stats"`)
	mustNotContain(t, admin, "document.visibilityState") // CSP forbids htmx filter eval; gating lives in lio-mod.js
	// the disclosure sits outside the swapped region, so a refresh cannot
	// collapse it under the operator
	before, _, ok := strings.Cut(admin, `id="systemStats"`)
	if !ok || !strings.Contains(before, `id="sysDetail"`) {
		t.Error("the detail toggle must precede the polled region, or every refresh closes it")
	}

	// a moderator who is not an admin does not get the panel at all
	m.IsAdmin = false
	mod := renderSmoke(t, System(SystemMeta(), m))
	mustNotContain(t, mod, `id="systemStats"`)
	mustNotContain(t, mod, "localhost:6379")

	// the fragment renders standalone, which is how the poll receives it
	mustContain(t, renderSmoke(t, SystemStatsBody(m.Stats)), "Postgres")
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
	mustContain(t, out, `href="/system/log"`)              // page 1 is the bare path
	mustContain(t, out, `href="/system/log?page=3"`)       // next page
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
		{ModActionQuery{Page: 1}, "/system/log"},
		{ModActionQuery{Page: 0}, "/system/log"},
		{ModActionQuery{Page: 2}, "/system/log?page=2"},
		{ModActionQuery{Query: "drewtest", Page: 1}, "/system/log?q=drewtest"},
		{ModActionQuery{Query: "drewtest", Action: "ban", Page: 3},
			"/system/log?action=ban&page=3&q=drewtest"},
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
	page := Index(PageMeta("home"), nil, message.SiteStats{}, message.Community{})
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
	page := Index(PageMeta("home"), nil, message.SiteStats{}, message.Community{})
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
	page := Index(PageMeta("home"), nil, message.SiteStats{}, message.Community{})

	plain := renderSmokeViewer(t, Viewer{
		UID: "u", LoggedIn: true, Username: "drew", AccountsEnabled: true,
	}, page)
	mustNotContain(t, plain, `href="/system/people"`)
	mustNotContain(t, plain, `href="/moderation"`)

	staff := renderSmokeViewer(t, Viewer{
		UID: "u", LoggedIn: true, Username: "drew", AccountsEnabled: true, Role: role.Mod,
	}, page)
	// the console link points at the page the inbox is on, not its front page
	mustContain(t, staff, `href="/system/people"`)
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
	mustContain(t, out, `href="/system/log?q=drewtest"`)
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
	// the overview: where the controls that need confirming live
	out := systemTab(t, TabOverview, true)

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
	mustNotContain(t, systemTab(t, TabOverview, false), `id="modalConfirmChange"`)
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

// TestProfileReportControl: a logged-in visitor on someone else's open account
// gets the report control and the dialog behind it. ShowReport is resolved
// server-side — including its mutual exclusion with the mod bar, since a
// moderator who can act on this account has no reason to file a report about it
// — so the page renders both halves off that one flag.
func TestProfileReportControl(t *testing.T) {
	m := profileFixture()

	plain := renderSmoke(t, Profile(ProfileMeta(m), m))
	mustNotContain(t, plain, "data-report-target")
	mustNotContain(t, plain, `id="modalReport"`)
	mustNotContain(t, plain, "lio-report")

	m.ShowReport = true
	out := renderSmoke(t, Profile(ProfileMeta(m), m))
	mustContain(t, out, `data-report-target="drewtest"`)
	mustContain(t, out, "Report drewtest")
	mustContain(t, out, `id="modalReport"`)
	mustContain(t, out, "lio-report")
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

// TestClockSeatLinksToProfile covers the username-linking convention: a seat
// backed by a real account reaches that account's page, while "You",
// "Anonymous" and "BOT" are labels with no page to point at and stay plain text.
func TestClockSeatLinksToProfile(t *testing.T) {
	linked := renderSmoke(t, clock(
		title.Title{Code: "GM"}, "drewtest", "", "1650", 0, "/@/drewtest"))
	mustContain(t, linked, `href="/@/drewtest"`)
	mustContain(t, linked, `class="player-link"`)
	// the title badge travels inside the link, so the whole identity is one target
	mustContain(t, linked, ">GM</span>")

	// a bot seat has no account behind it
	bot := renderSmoke(t, clock(title.Title{}, "BOT", "♛︎", "", 0, ""))
	mustNotContain(t, bot, "player-link")
	mustNotContain(t, bot, "/@/")
}

// TestProfilePopoverLinksToOwnProfile covers the account popover's username: it
// is the viewer's own name, and it should reach their page like any other
// username on the site.
func TestProfilePopoverLinksToOwnProfile(t *testing.T) {
	out := renderSmoke(t, profilePopover("drewtest", title.Title{Code: "OG", Name: "Original Gamer"}))
	mustContain(t, out, `href="/@/drewtest"`)
	mustContain(t, out, `class="player-link"`)
	// the title badge travels inside the link with the name
	mustContain(t, out, ">OG</span>")
	mustContain(t, out, ">drewtest</span>")
}

// TestTimelineNameLinks covers the match timeline's seat identity: a real
// account reaches its page, while an anonymous or bot seat has none to reach.
// The timeline renders in both the live room and the archive, so the rule has
// to be one component or the two drift apart.
func TestTimelineNameLinks(t *testing.T) {
	linked := renderSmoke(t, timelineName(
		title.Title{Code: "GM"}, "drewtest", "/@/drewtest"))
	mustContain(t, linked, `href="/@/drewtest"`)
	mustContain(t, linked, `class="player-link"`)
	mustContain(t, linked, `class="tl-name"`)
	mustContain(t, linked, ">GM</span>")

	// anonymous / bot seats: a label, not an account
	plain := renderSmoke(t, timelineName(title.Title{}, "Anonymous", ""))
	mustNotContain(t, plain, "player-link")
	mustNotContain(t, plain, "/@/")
	mustContain(t, plain, "Anonymous")
}

// feedbackFixture is one unread and one read submission, covering both row
// shapes the inbox renders.
func feedbackFixture() FeedbackInbox {
	return FeedbackInbox{
		Unread: 1,
		Total:  2,
		Shown:  2,
		Items: []FeedbackView{
			{ID: "7", When: "just now", WhenExact: "2026-07-26 12:00:00 UTC",
				Kind: "problem", Class: FeedbackKindClass("problem"),
				Label: FeedbackKindLabel("problem"),
				Body:  "the clock jumps on my phone", Author: "drewtest",
				Path: "/Ab3xY9", Unread: true},
			{ID: "6", When: "yesterday", WhenExact: "2026-07-25 09:00:00 UTC",
				Kind: "praise", Class: FeedbackKindClass("praise"),
				Label: FeedbackKindLabel("praise"),
				Body:  "the new board themes are lovely", Author: "cdpplayer",
				Reader: "drewtest"},
		},
	}
}

// TestRenderFeedbackInbox covers the /system feedback section: both row states,
// the kind tint, the page-path link back into the site, and the mark-read
// controls that only exist while something is unread.
func TestRenderFeedbackInbox(t *testing.T) {
	out := renderSmoke(t, FeedbackInboxSection(feedbackFixture()))
	mustContain(t, out, "1 unread")
	mustContain(t, out, "the clock jumps on my phone")
	mustContain(t, out, "the new board themes are lovely")
	mustContain(t, out, `href="/@/drewtest"`) // the author reaches their page
	mustContain(t, out, `href="/Ab3xY9"`)     // where they were when they wrote it
	mustContain(t, out, "fb-problem")         // tinted by kind, not by severity
	mustContain(t, out, "fb-praise")
	mustContain(t, out, "is-unread")
	mustContain(t, out, `data-feedback-read="7"`) // the unread row offers the action
	mustContain(t, out, "data-feedback-read-all")
	mustContain(t, out, "read by drewtest") // the read row states who read it
	mustNotContain(t, out, `data-feedback-read="6"`)

	// nothing unread: no mark-read controls anywhere, and no dot
	allRead := feedbackFixture()
	allRead.Unread = 0
	allRead.Items[0].Unread = false
	allRead.Items[0].Reader = "drewtest"
	read := renderSmoke(t, FeedbackInboxSection(allRead))
	mustContain(t, read, "all read")
	mustNotContain(t, read, "data-feedback-read-all")
	mustNotContain(t, read, "unread-dot")

	// an empty inbox says where feedback comes from rather than just "none"
	empty := renderSmoke(t, FeedbackInboxSection(FeedbackInbox{}))
	mustContain(t, empty, "Nothing yet.")
	mustNotContain(t, empty, "unread-dot")
}

// TestFeedbackInboxMore: a bounded page states what it is leaving out, so a
// long history never looks like the whole of it.
func TestFeedbackInboxMore(t *testing.T) {
	f := feedbackFixture()
	if got := f.More(); got != 0 {
		t.Errorf("More() = %d with everything shown, want 0", got)
	}
	f.Total = 40
	if got := f.More(); got != 38 {
		t.Errorf("More() = %d, want 38", got)
	}
	mustContain(t, renderSmoke(t, FeedbackInboxSection(f)), "38 older not shown.")
}

// TestUnreadDotOnlyWhenUnread: the red marker is a signal, and a marker that is
// always present is one nobody looks at.
func TestUnreadDotOnlyWhenUnread(t *testing.T) {
	mustNotContain(t, renderSmoke(t, unreadDot(0)), "unread-dot")
	on := renderSmoke(t, unreadDot(3))
	mustContain(t, on, "unread-dot")
	// the count travels with the dot: "there is something" is not the same
	// answer as "there are three things"
	mustContain(t, on, "3 unread feedback messages")
	mustContain(t, renderSmoke(t, unreadDot(1)), "1 unread feedback message")
}

// TestFeedbackPromptAndModal covers the player-facing half: the popover prompt
// names both halves of what we want to hear about, and the dialog offers every
// kind the database accepts plus the honeypot the server checks.
func TestFeedbackPromptAndModal(t *testing.T) {
	prompt := renderSmoke(t, feedbackPrompt())
	mustContain(t, prompt, `id="feedbackButton"`)
	// the invitation has to cover praise as well as problems, or it only ever
	// collects complaints
	mustContain(t, prompt, "Tell us how it's going")
	mustContain(t, prompt, "We read all of it.")

	modal := renderSmoke(t, feedbackModal())
	mustContain(t, modal, `id="modalFeedback"`)
	mustContain(t, modal, `id="feedbackForm"`)
	// every kind the CHECK constraint accepts is offered, so the picker cannot
	// drift from what the database will store. The prompts are compared escaped
	// because they contain apostrophes, which is what templ writes out.
	for _, k := range FeedbackKinds() {
		mustContain(t, modal, `value="`+k+`"`)
		mustContain(t, modal, html.EscapeString(FeedbackKindPrompt(k)))
	}
	// the honeypot is present and empty; a filled one is how the server spots a
	// form-filling bot
	mustContain(t, modal, `name="website"`)
	mustContain(t, modal, "feedback-hp")
}

// TestFeedbackPromptShownToEveryAccount: the prompt is not a staff control. It
// renders for an ordinary account, while the moderator-only staff links and the
// unread dot do not.
func TestFeedbackPromptShownToEveryAccount(t *testing.T) {
	player := renderSmokeViewer(t, Viewer{LoggedIn: true, Username: "drewtest"},
		profilePopover("drewtest", title.Title{}))
	mustContain(t, player, `id="feedbackButton"`)
	mustNotContain(t, player, `href="/system/people"`)
	mustNotContain(t, player, "unread-dot")

	// a moderator with unread feedback gets the dot on the System link, which
	// is the step between noticing and reading
	mod := renderSmokeViewer(t,
		Viewer{LoggedIn: true, Username: "drewtest", Role: role.Mod, UnreadFeedback: 4},
		profilePopover("drewtest", title.Title{}))
	mustContain(t, mod, `href="/system/people"`)
	mustContain(t, mod, "unread-dot")
	mustContain(t, mod, "4 unread feedback messages")
}

// TestNotifyClientGating: the notification client ships to every signed-in
// account, not just to a moderator (arch/NOTIFICATIONS.md). It owns the bell for
// everyone, and on a page with no other socket it is what keeps the reader
// reachable at all — so gating it on a role would leave ordinary accounts unable
// to receive anything.
//
// It is asserted against the *header*, not a page-script bundle: the bell is in
// the header on every page, while scriptsBase covers only some of them.
func TestNotifyClientGating(t *testing.T) {
	anon := renderSmokeViewer(t, Viewer{}, header(""))
	mustNotContain(t, anon, "lio-notify")

	player := renderSmokeViewer(t,
		Viewer{LoggedIn: true, Username: "drewtest"}, header(""))
	mustContain(t, player, "lio-notify")

	mod := renderSmokeViewer(t,
		Viewer{LoggedIn: true, Username: "drewtest", Role: role.Mod}, header(""))
	mustContain(t, mod, "lio-notify")

	// a moderator with nothing unread still gets the anchor: the client needs
	// somewhere to put the dot when the count goes above zero
	quiet := renderSmokeViewer(t,
		Viewer{LoggedIn: true, Username: "drewtest", Role: role.Mod},
		profilePopover("drewtest", title.Title{}))
	mustContain(t, quiet, "data-unread-anchor")
	mustNotContain(t, quiet, "unread-dot")
}

// TestNotifyBell: the bell renders for a signed-in account and carries the
// badge only when something is unread. A dot that is always there, and only
// sometimes means something, is a dot nobody reads.
func TestNotifyBell(t *testing.T) {
	quiet := renderSmokeViewer(t,
		Viewer{LoggedIn: true, Username: "drewtest"}, header(""))
	mustContain(t, quiet, `id="notifyButton"`)
	mustContain(t, quiet, `id="notifyPanel"`)
	mustNotContain(t, quiet, "notify-dot")

	loud := renderSmokeViewer(t,
		Viewer{LoggedIn: true, Username: "drewtest", UnreadNotifications: 3}, header(""))
	mustContain(t, loud, "notify-dot")
	mustContain(t, loud, "3 unread notifications")

	// an anonymous visitor has no name, so nothing can be addressed to them and
	// the header shows no bell at all
	anon := renderSmokeViewer(t, Viewer{}, header(""))
	mustNotContain(t, anon, `id="notifyButton"`)
}

// TestNotifyBadgeCountsFeedback: a moderator has one bell, and unread feedback
// counts into it alongside their own notifications. Feedback is not stored as a
// notification — its read state is site-wide on purpose — so the badge is the
// one place the two are added together.
func TestNotifyBadgeCountsFeedback(t *testing.T) {
	out := renderSmokeViewer(t, Viewer{
		LoggedIn:            true,
		Username:            "drewtest",
		Role:                role.Mod,
		UnreadFeedback:      4,
		UnreadNotifications: 2,
	}, header(""))
	mustContain(t, out, "6 unread notifications")
}

// A member who has left keeps their place in the roster and says so three ways:
// no presence dot, no sword, and a token saying how long ago. This is the whole
// visible difference the 15-minute window introduced, and it is the same
// treatment an offline arrival has always had — one component draws all of it.
func TestRosterDepartedMember(t *testing.T) {
	c := message.Community{
		Online: []message.OnlineMember{
			{Username: "here", Online: true},
			{Username: "left", Left: time.Now().Add(-4 * time.Minute)},
		},
	}
	out := renderSmokeViewer(t, Viewer{LoggedIn: true, Username: "drewtest", AccountsEnabled: true},
		HomeActivity(nil, message.SiteStats{}, c))

	// both are listed
	mustContain(t, out, `href="/@/here"`)
	mustContain(t, out, `href="/@/left"`)
	// only the connected one carries the dot
	if n := strings.Count(out, `class="roster-dot"`); n != 1 {
		t.Errorf("presence dots = %d, want 1 (only the member who is here)", n)
	}
	// ...and only they can be challenged: an invitation has to reach somebody
	mustContain(t, out, `data-challenge="here"`)
	mustNotContain(t, out, `data-challenge="left"`)
	// the departed row says when, in the chip and in its tooltip
	mustContain(t, out, ">4m<")
	mustContain(t, out, `title="left 4 minutes ago"`)
}

// The footnote accounts for everybody the chips above it do not: the anonymous
// visitors, the members the display cap dropped, and the window that explains
// why somebody with no dot is listed at all.
func TestRosterNoteStatesWindowAndOverflow(t *testing.T) {
	c := message.Community{
		Online: []message.OnlineMember{{Username: "nova", Online: true}},
		Anon:   2,
		More:   12,
	}
	out := renderSmokeViewer(t, Viewer{LoggedIn: true, Username: "drewtest"},
		HomeActivity(nil, message.SiteStats{}, c))

	mustContain(t, out, "2 anonymous visitors")
	mustContain(t, out, "12 more not shown")
	mustContain(t, out, "active in the last 15 minutes")
}

// With no roster there is no window to explain, so the footnote does not claim
// one. It still counts the anonymous visitors, who are there either way.
func TestRosterNoteOmitsWindowWithoutChips(t *testing.T) {
	c := message.Community{
		Anon:   1,
		Newest: []message.NewMember{{Username: "pawnstar", Joined: time.Now()}},
	}
	out := renderSmokeViewer(t, Viewer{LoggedIn: true, Username: "drewtest"},
		HomeActivity(nil, message.SiteStats{}, c))

	mustContain(t, out, "1 anonymous visitor")
	mustNotContain(t, out, "active in the last")
}

// The chip token and its tooltip must agree about where a minute begins. They
// once did not: a member gone 30 seconds read "1m" in the chip and "left just
// now" in its own title attribute.
func TestDepartedTokenAgreesWithTooltip(t *testing.T) {
	for _, tc := range []struct {
		ago         time.Duration
		token, hint string
	}{
		{30 * time.Second, "now", "left just now"},
		{90 * time.Second, "1m", "left 1 minute ago"},
		{4 * time.Minute, "4m", "left 4 minutes ago"},
	} {
		at := time.Now().Add(-tc.ago)
		if got := shortSince(at); got != tc.token {
			t.Errorf("shortSince(%s) = %q, want %q", tc.ago, got, tc.token)
		}
		if got := leftPhrase(at); got != tc.hint {
			t.Errorf("leftPhrase(%s) = %q, want %q", tc.ago, got, tc.hint)
		}
	}
	// an online member carries neither
	if got := shortSince(time.Time{}); got != "" {
		t.Errorf("shortSince(zero) = %q, want empty", got)
	}
	if got := leftPhrase(time.Time{}); got != "" {
		t.Errorf("leftPhrase(zero) = %q, want empty", got)
	}
}

// A card whose only rows are the viewer's follows still explains the window:
// those chips carry the same departed state the site-wide roster's do.
func TestRosterNoteWindowWithFollowingOnly(t *testing.T) {
	c := message.Community{
		Following: []message.OnlineMember{{Username: "friend", Online: true}},
	}
	out := renderSmokeViewer(t, Viewer{LoggedIn: true, Username: "drewtest"},
		HomeActivity(nil, message.SiteStats{}, c))
	mustContain(t, out, "active in the last 15 minutes")
}

// staffFixture is a site with one bootstrapped admin, one appointed admin and
// one moderator — the three states the panel has to tell apart.
func staffFixture() []db.StaffMember {
	return []db.StaffMember{
		{
			ID: 1, Username: "drewtest", Role: role.Admin,
			TitleCode: "DEV", TitleName: "Developer",
			Joined: time.Now().Add(-400 * 24 * time.Hour),
			// no grantor: promoted by hand in SQL
		},
		{
			ID: 2, Username: "secondadmin", Role: role.Admin,
			Joined:    time.Now().Add(-90 * 24 * time.Hour),
			GrantedBy: "drewtest", GrantedAt: time.Now().Add(-30 * 24 * time.Hour),
		},
		{
			ID: 3, Username: "helpfulmod", Role: role.Mod,
			Joined:    time.Now().Add(-60 * 24 * time.Hour),
			GrantedBy: "drewtest", GrantedAt: time.Now().Add(-10 * 24 * time.Hour),
		},
	}
}

// TestRenderStaffPage: the public page names who holds the tools, grouped by
// what they may do, and carries none of the staff-only accountability trail.
//
// The last part is the one worth guarding. The appointment comes out of the
// audit log, which is not public, and a field that reaches the model of a
// public page is one render mistake away from being on it — so StaffListOf
// refuses to populate it at all when detailed is false.
func TestRenderStaffPage(t *testing.T) {
	out := renderSmoke(t, Staff(StaffMeta(), StaffListOf(staffFixture(), false)))

	mustContain(t, out, "Admins")
	mustContain(t, out, "Moderators")
	mustContain(t, out, `href="/@/drewtest"`)
	mustContain(t, out, `href="/@/secondadmin"`)
	mustContain(t, out, `href="/@/helpfulmod"`)
	// a staff account's title renders like it does everywhere else
	mustContain(t, out, "DEV")

	// none of the audit trail
	mustNotContain(t, out, "bootstrapped")
	mustNotContain(t, out, "by drewtest")
}

// TestStaffListSplitsByRole: two groups, because what separates a moderator
// from an admin is what they may do — which is the question the page answers.
func TestStaffListSplitsByRole(t *testing.T) {
	list := StaffListOf(staffFixture(), true)
	if len(list.Admins) != 2 || len(list.Mods) != 1 {
		t.Fatalf("admins=%d mods=%d, want 2 and 1", len(list.Admins), len(list.Mods))
	}
	if list.Total() != 3 {
		t.Errorf("Total = %d, want 3", list.Total())
	}
	if list.Empty() {
		t.Error("a site with three staff reported itself empty")
	}
	if !list.Admins[0].Bootstrapped() {
		t.Error("the hand-promoted admin should have no grantor on record")
	}
	if list.Admins[1].Bootstrapped() {
		t.Error("an appointed admin was reported as bootstrapped")
	}
	if StaffCountLabel(StaffListOf(staffFixture()[:1], false)) != "1 person" {
		t.Error("the count label does not singularize")
	}
}

// TestRenderStaffPanel: the /system panel is the same list plus the
// accountability half — who granted each role, and the account that was
// promoted outside the app and therefore cannot be demoted through the UI.
//
// It renders for a plain moderator as well as an admin. The tools are
// accountable to the people who hold them first, and there is no version of
// this page where a moderator should have to ask who else can act.
func TestRenderStaffPanel(t *testing.T) {
	m := systemFixture()
	m.Tab = TabPeople
	m.Staff = StaffListOf(staffFixture(), true)

	admin := renderSmoke(t, System(SystemMeta(), m))
	m.IsAdmin = false
	mod := renderSmoke(t, System(SystemMeta(), m))

	for name, out := range map[string]string{"admin": admin, "moderator": mod} {
		if !strings.Contains(out, `href="/@/helpfulmod"`) {
			t.Errorf("%s cannot see the staff list", name)
		}
		if !strings.Contains(out, "bootstrapped") {
			t.Errorf("%s: the hand-promoted admin is not marked, so its missing "+
				"grantor reads as a gap in the data", name)
		}
		if !strings.Contains(out, "by drewtest") {
			t.Errorf("%s: the appointment trail is missing", name)
		}
	}

	// a sanctioned staff account should never exist; if one does, it is loud
	sanctioned := StaffListOf([]db.StaffMember{
		{ID: 4, Username: "compromised", Role: role.Mod, Sanctioned: true},
	}, true)
	m.Staff = sanctioned
	mustContain(t, renderSmoke(t, System(SystemMeta(), m)), "staff-sanctioned")

	// an empty site says so rather than rendering an empty box
	m.Staff = StaffList{Detailed: true}
	mustContain(t, renderSmoke(t, System(SystemMeta(), m)), "Nobody holds a staff role yet.")
}

// TestRenderBroadcastComposer: sending to every account is admin work, and the
// line is the one /system already draws — writing to one player is a
// moderator's, anything every visitor sees is an admin's.
func TestRenderBroadcastComposer(t *testing.T) {
	admin := systemTab(t, TabOverview, true)
	mustContain(t, admin, `id="broadcastForm"`)
	mustContain(t, admin, "data-broadcast")
	mustContain(t, admin, "Sent broadcasts")
	// it goes through the confirmation modal, unlike the one-player composer:
	// a broadcast reaches everybody and cannot be unsent
	mustContain(t, admin, `data-confirm="Broadcast to every account"`)

	// A moderator sees no broadcast composer on the same page.
	mustNotContain(t, systemTab(t, TabOverview, false), `id="broadcastForm"`)
	// ...but the one-player composer still ships to them, on People
	mustContain(t, systemTab(t, TabPeople, false), `id="msgSearch"`)
}

// TestRenderAnswerOptions: the control that turns a message into a question is
// offered by both composers, because the flag belongs to notifications rather
// than to broadcasts.
func TestRenderAnswerOptions(t *testing.T) {
	people := systemTab(t, TabPeople, true)
	site := systemTab(t, TabOverview, true)
	mustContain(t, people, `id="msgChoices"`)
	mustContain(t, site, `id="bcChoices"`)
	// both carry the form name their controller reads: the broadcast composer
	// is a form and reads form.elements, the one-player composer is not and
	// reads by id
	for name, out := range map[string]string{"people": people, "site": site} {
		if n := strings.Count(out, `name="choices"`); n != 1 {
			t.Errorf(`%s: name="choices" appears %d times, want 1`, name, n)
		}
	}
}

// TestRenderSentBroadcasts: a live message can be retired, an ended one cannot,
// and a question shows what it asked even before anybody answers — the options
// are the half a zero count cannot state.
func TestRenderSentBroadcasts(t *testing.T) {
	m := systemFixture()
	m.Tab = TabOverview
	m.Broadcasts = []BroadcastView{
		{
			ID: "1", Body: "the server restarts at 9pm", When: "2 hours ago",
			Actor: "drewtest", Live: true,
			Asks: []string{"OK"},
		},
		{
			ID: "2", Body: "do you want rated blitz?", When: "yesterday",
			Actor: "drewtest", Live: false,
			Asks: []string{"Yes", "No"}, Answers: 12,
			Tally: []BroadcastTallyView{{Choice: "Yes", Count: "9"}, {Choice: "No", Count: "3"}},
		},
	}
	out := renderSmoke(t, System(SystemMeta(), m))

	// only the live one offers a retire
	if n := strings.Count(out, "data-retire-broadcast"); n != 1 {
		t.Errorf("retire controls = %d, want 1 (only the live message)", n)
	}
	mustContain(t, out, `data-retire-broadcast="1"`)
	mustContain(t, out, "the server restarts at 9pm")
	// the ended one keeps its answers: the tally is why it was sent
	mustContain(t, out, "12 answers")
	mustContain(t, out, "bc-chip")
	// an unanswered question still names its options
	mustContain(t, out, "bc-chip-empty")

	mustContain(t, systemTab(t, TabOverview, true), "Nothing has been broadcast yet.")
}

// TestRenderLearn covers the tutorial page's server-rendered shell: the whole
// course in the rail (so the page is navigable and crawlable with no
// JavaScript), the opening lesson marked current with its first prompt already
// on screen, the board mount, and the curriculum inlined for lio-learn.js.
func TestRenderLearn(t *testing.T) {
	out := renderSmoke(t, Learn(PageMeta("Learn to play"), &learn.Lessons[0]))

	// every lesson is a real link in the rail
	for _, l := range learn.Lessons {
		mustContain(t, out, `href="/learn/`+l.Slug+`"`)
		mustContain(t, out, html.EscapeString(l.Title))
	}
	// ...grouped under every chapter
	for _, c := range learn.Chapters() {
		mustContain(t, out, html.EscapeString(c.Title))
	}

	// the opening lesson is current, and its first prompt is already rendered
	mustContain(t, out, `data-lesson="board"`)
	mustContain(t, out, "learn-lesson current")
	mustContain(t, out, html.EscapeString(learn.Lessons[0].Steps[0].Prompt))

	// the board mount and the curriculum payload the client drives from
	mustContain(t, out, `id="learn-board"`)
	mustContain(t, out, `id="learn-data"`)
	mustContain(t, out, `src="/lio-learn.`)

	// a step dot per step of the open lesson
	if n := strings.Count(out, "learn-dot"); n < len(learn.Lessons[0].Steps) {
		t.Errorf("step dots = %d, want at least %d", n, len(learn.Lessons[0].Steps))
	}
}

// TestRenderLearnDeepLink checks a lesson-specific URL renders opened at that
// lesson rather than at the start of the course.
func TestRenderLearnDeepLink(t *testing.T) {
	target, ok := learn.BySlug("castling")
	if !ok {
		t.Fatal("castling lesson missing")
	}
	out := renderSmoke(t, Learn(PageMeta("Learn to play"), target))
	mustContain(t, out, html.EscapeString(target.Steps[0].Prompt))
	// the rail marks the opened lesson, not the first one
	mustContain(t, out, `class="learn-lesson current" data-lesson="castling"`)
	mustNotContain(t, out, `class="learn-lesson current" data-lesson="board"`)
}

// TestLearnSuccessLinesStayServerSide checks where each step's success line
// lives. The server is what judges a move and says so in its reply, so a
// playable step's success line must not be in the page — it would give the
// answer away and put the verdict somewhere the client could fake to itself.
// The click-a-square step is the deliberate exception: it makes no move, so
// there is no request to answer and nothing but the client could ever say it.
func TestLearnSuccessLinesStayServerSide(t *testing.T) {
	out := renderSmoke(t, Learn(PageMeta("Learn to play"), &learn.Lessons[0]))
	var checked, exempt int
	for _, l := range learn.Lessons {
		for _, s := range l.Steps {
			if s.Goal == learn.GoalSelect {
				// it rides in the JSON payload, where the text is literal
				mustContain(t, out, s.Success)
				exempt++
				continue
			}
			// absent in both the JSON payload and the rendered markup
			mustNotContain(t, out, s.Success)
			mustNotContain(t, out, html.EscapeString(s.Success))
			checked++
		}
	}
	// guard the guard: if the curriculum ever stopped having both kinds of
	// step, this test would silently assert nothing
	if checked == 0 || exempt == 0 {
		t.Fatalf("expected both judged and click-only steps, got %d and %d", checked, exempt)
	}
}
