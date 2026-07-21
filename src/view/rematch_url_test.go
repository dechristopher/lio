package view

import (
	"testing"

	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/variant"
)

// TestBotRematchURL locks the "same settings" fresh-room rematch link a bot
// game's result overlay navigates to: /new/computer with the finished game's
// variant (HTMLName), the player's own side, and the bot's difficulty persona
// (empty resolves to the full-strength Queen, so the fresh room keeps the same
// opponent). Human games get no link (their rematch stays the in-room
// agreement flow).
func TestBotRematchURL(t *testing.T) {
	cases := []struct {
		name    string
		payload message.RoomTemplatePayload
		want    string
	}{
		// PlayerColor carries octad.Color.String() values ("w"/"b", or "-" for
		// a non-player), not full color names — the same tokens the board's
		// orientation class check keys off.
		{
			name: "bot game as white defaults to queen when persona unset",
			payload: message.RoomTemplatePayload{
				OpponentIsBot: true,
				PlayerColor:   "w",
				Variant:       variant.HalfOneBlitz,
			},
			want: "/new/computer?tc=half-one-blitz&color=w&bot=queen",
		},
		{
			name: "bot game as black preserves side, deploy variant, and persona",
			payload: message.RoomTemplatePayload{
				OpponentIsBot: true,
				PlayerColor:   "b",
				Variant:       variant.HalfOneBlitzDeploy,
				BotPersona:    "pawn",
			},
			want: "/new/computer?tc=half-one-blitz-deploy&color=b&bot=pawn",
		},
		{
			name: "human game has no rematch link",
			payload: message.RoomTemplatePayload{
				OpponentIsBot: false,
				PlayerColor:   "w",
				Variant:       variant.HalfOneBlitz,
			},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := botRematchURL(tc.payload); got != tc.want {
				t.Fatalf("botRematchURL = %q, want %q", got, tc.want)
			}
		})
	}
}
