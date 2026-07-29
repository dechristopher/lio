package room

import (
	"testing"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/engine"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/title"
)

// TestTVSeat locks the identity the home-page TV grid shows for each kind of
// seat. The rule that matters is that nothing here is viewer-relative: the TV
// stream broadcasts one card to everyone, so an anonymous seat must resolve to
// a name a stranger can read rather than the room view's "You" (which no
// viewer of the grid could interpret).
func TestTVSeat(t *testing.T) {
	pawn := engine.PersonaByKey("pawn")

	tests := []struct {
		name  string
		seat  *player.Player
		perso string
		want  string
		bot   bool
		title string
		glyph string
	}{
		{
			name: "logged-in account carries its username and title badge",
			seat: &player.Player{
				ID:       "u1",
				Username: "drewtest",
				Title:    title.Title{Code: "GM", Name: "Grandmaster"},
			},
			want:  "drewtest",
			title: "GM",
		},
		{
			name: "untitled account shows a name and no badge",
			seat: &player.Player{ID: "u2", Username: "cdpplayer"},
			want: "cdpplayer",
		},
		{
			name: "anonymous human is spelled out, never left blank",
			seat: &player.Player{ID: "u3"},
			want: "Anonymous",
		},
		{
			name:  "bot seat is named by its difficulty persona",
			seat:  &player.Player{IsBot: true},
			perso: "pawn",
			want:  pawn.Name,
			bot:   true,
			glyph: pawn.Glyph,
		},
		{
			// every pre-persona room stamped no key; PersonaByKey resolves those
			// to the strongest persona, so the seat still gets a real label
			name:  "bot seat with no persona key falls back to a named persona",
			seat:  &player.Player{IsBot: true},
			perso: "",
			want:  engine.PersonaByKey("").Name,
			bot:   true,
			glyph: engine.PersonaByKey("").Glyph,
		},
		{
			name: "missing seat degrades to Anonymous rather than an empty card",
			seat: nil,
			want: "Anonymous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestInstance(t, "wp", "bp")
			r.players[octad.White] = tt.seat
			r.params.BotPersona = tt.perso

			got := r.tvSeatLocked(octad.White)
			if got.Name != tt.want {
				t.Errorf("Name = %q, want %q", got.Name, tt.want)
			}
			if got.Bot != tt.bot {
				t.Errorf("Bot = %t, want %t", got.Bot, tt.bot)
			}
			if got.Title != tt.title {
				t.Errorf("Title = %q, want %q", got.Title, tt.title)
			}
			if got.Glyph != tt.glyph {
				t.Errorf("Glyph = %q, want %q", got.Glyph, tt.glyph)
			}
		})
	}
}

// TestTVEventCarriesBothSeats guards the wiring rather than the resolution: a
// TV event must describe both seats, since the grid renders a name above and
// below every board.
func TestTVEventCarriesBothSeats(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.players[octad.White] = &player.Player{ID: "wp", Username: "drewtest"}
	r.players[octad.Black] = &player.Player{IsBot: true}
	r.params.BotPersona = "knight"

	ev := r.tvEvent(0)
	if ev.WhiteSeat.Name != "drewtest" {
		t.Errorf("white seat = %q, want drewtest", ev.WhiteSeat.Name)
	}
	if !ev.BlackSeat.Bot || ev.BlackSeat.Name != engine.PersonaByKey("knight").Name {
		t.Errorf("black seat = %+v, want the knight persona", ev.BlackSeat)
	}
}
