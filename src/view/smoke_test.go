package view

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/pools"
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
	out := renderSmoke(t, Index(PageMeta("Free Online Octad"), pools.RatingPools))
	mustContain(t, out, "<title>lioctad.org • Free Online Octad</title>")
	mustContain(t, out, "QUICK GAME")
	mustContain(t, out, `id="modalCreateGame"`)
	mustContain(t, out, "getElementById(\"modalCreateGame\")") // inline modal script

	// pool order must be bullet -> blitz -> rapid (sorted, number-prefixed keys)
	order := []string{
		"quarter-zero-blitz", "quarter-one-blitz",
		"half-zero-blitz", "half-one-blitz",
		"one-zero-rapid", "one-two-rapid",
	}
	last := -1
	for _, name := range order {
		i := strings.Index(out, `value="`+name+`"`)
		if i < 0 {
			t.Errorf("missing variant %q", name)
			continue
		}
		if i < last {
			t.Errorf("variant %q out of order", name)
		}
		last = i
	}
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
	mustContain(t, out, `id="gameTitle"`)
	mustContain(t, out, "Half One blitz")
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
}

func TestRenderAboutAndNotFound(t *testing.T) {
	mustContain(t, renderSmoke(t, About(PageMeta("About"), "board")), "Board Layout")
	mustContain(t, renderSmoke(t, About(PageMeta("About"), "misc")), "ppkn/4/4/NKPP w NCFncf - 0 1")
	mustContain(t, renderSmoke(t, NotFound(PageMeta("404"))), "404")
	mustContain(t, renderSmoke(t, DB(PageMeta("Game Database"))), "Game Database")
}
