package view

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/dechristopher/lio/message"
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

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("output missing %q", sub)
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
	mustContain(t, out, "Accounts are coming")     // login stub modal
	mustContain(t, out, ">Log in<")                // nav stub

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

func TestRenderAboutAndNotFound(t *testing.T) {
	mustContain(t, renderSmoke(t, About(PageMeta("About"), "board")), "Board Layout")
	mustContain(t, renderSmoke(t, About(PageMeta("About"), "misc")), "ppkn/4/4/NKPP w NCFncf - 0 1")
	mustContain(t, renderSmoke(t, NotFound(PageMeta("404"))), "404")
	mustContain(t, renderSmoke(t, DB(PageMeta("Game Database"))), "Game Database")
}
