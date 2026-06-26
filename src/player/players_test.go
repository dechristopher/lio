package player

import (
	"testing"

	"github.com/dechristopher/octad"
)

func TestAnchorColor(t *testing.T) {
	tests := []struct {
		name string
		p    Players
		want octad.Color
	}{
		{
			name: "bot as black anchors the human at white",
			p:    Players{octad.White: {ID: "human"}, octad.Black: {ID: "bot", IsBot: true}},
			want: octad.White,
		},
		{
			name: "bot as white anchors the human at black",
			p:    Players{octad.White: {ID: "bot", IsBot: true}, octad.Black: {ID: "human"}},
			want: octad.Black,
		},
		{
			name: "human-vs-human anchors the lower id (white lower)",
			p:    Players{octad.White: {ID: "aaa"}, octad.Black: {ID: "bbb"}},
			want: octad.White,
		},
		{
			name: "human-vs-human anchors the lower id (black lower)",
			p:    Players{octad.White: {ID: "bbb"}, octad.Black: {ID: "aaa"}},
			want: octad.Black,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.AnchorColor(); got != tt.want {
				t.Errorf("AnchorColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAnchorColorStableAcrossFlip is the property the TV board flip relies on:
// the same player keeps the anchor as colors swap between games of a match.
func TestAnchorColorStableAcrossFlip(t *testing.T) {
	p := Players{octad.White: {ID: "aaa"}, octad.Black: {ID: "bbb"}}

	before := p.AnchorColor()
	anchored := p[before]

	p.FlipColor()

	after := p.AnchorColor()
	if p[after] != anchored {
		t.Errorf("anchor moved to a different player across flip: before=%v after=%v", before, after)
	}
}
