package view

import (
	"testing"

	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/variant"
)

// TestBotRematchURL locks the "same settings" fresh-room rematch link a bot
// game's result overlay navigates to: /new/computer with the finished game's
// variant (HTMLName) and the player's own side. Human games get no link (their
// rematch stays the in-room agreement flow).
func TestBotRematchURL(t *testing.T) {
	cases := []struct {
		name    string
		payload message.RoomTemplatePayload
		want    string
	}{
		{
			name: "bot game as white",
			payload: message.RoomTemplatePayload{
				OpponentIsBot: true,
				PlayerColor:   "white",
				Variant:       variant.HalfOneBlitz,
			},
			want: "/new/computer?tc=half-one-blitz&color=w",
		},
		{
			name: "bot game as black preserves side and deploy variant",
			payload: message.RoomTemplatePayload{
				OpponentIsBot: true,
				PlayerColor:   "black",
				Variant:       variant.HalfOneBlitzDeploy,
			},
			want: "/new/computer?tc=half-one-blitz-deploy&color=b",
		},
		{
			name: "human game has no rematch link",
			payload: message.RoomTemplatePayload{
				OpponentIsBot: false,
				PlayerColor:   "white",
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
