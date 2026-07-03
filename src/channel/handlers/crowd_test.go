package handlers

import (
	"testing"

	"github.com/dechristopher/lio/channel"
)

// TestCrowdPayload locks the crowd broadcast semantics: White/Black report
// seat presence, and Spec counts connected spectators only — seated players
// are excluded, extra tabs of one uid count once, and a bot seat (whose uid
// never holds a socket) reads not-present without distorting the count.
func TestCrowdPayload(t *testing.T) {
	seats := func() (string, string) { return "wp", "bp" }

	cases := []struct {
		name      string
		uids      []string // one socket per uid, plus extras via repeats
		wantWhite bool
		wantBlack bool
		wantSpec  int
	}{
		{name: "empty room", uids: nil, wantSpec: 0},
		{name: "players only", uids: []string{"wp", "bp"}, wantWhite: true, wantBlack: true, wantSpec: 0},
		{name: "one player one spectator", uids: []string{"wp", "s1"}, wantWhite: true, wantSpec: 1},
		{name: "players and spectators", uids: []string{"wp", "bp", "s1", "s2"}, wantWhite: true, wantBlack: true, wantSpec: 2},
		{name: "spectators only", uids: []string{"s1", "s2", "s3"}, wantSpec: 3},
		{name: "multi-tab spectator counts once", uids: []string{"wp", "s1", "s1"}, wantWhite: true, wantSpec: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := channel.NewSockMap("crowd-test")
			defer sm.Cleanup()
			for i, uid := range tc.uids {
				sm.Track(channel.NewSocket(nil, uid, string(rune('a'+i)), ""))
			}

			got := crowdPayload(sm, seats)
			if got.White != tc.wantWhite || got.Black != tc.wantBlack || got.Spec != tc.wantSpec {
				t.Fatalf("crowdPayload = {w:%t b:%t s:%d}, want {w:%t b:%t s:%d}",
					got.White, got.Black, got.Spec, tc.wantWhite, tc.wantBlack, tc.wantSpec)
			}
		})
	}
}

// TestCrowdPayloadBotSeat verifies a bot seat (uid never connected) reads
// not-present while the human's presence and spectators still count correctly.
func TestCrowdPayloadBotSeat(t *testing.T) {
	sm := channel.NewSockMap("crowd-test-bot")
	defer sm.Cleanup()
	seats := func() (string, string) { return "human", "bot-uid" }

	sm.Track(channel.NewSocket(nil, "human", "c1", ""))
	sm.Track(channel.NewSocket(nil, "spec", "c1", ""))

	got := crowdPayload(sm, seats)
	if !got.White || got.Black || got.Spec != 1 {
		t.Fatalf("crowdPayload = {w:%t b:%t s:%d}, want {w:true b:false s:1}",
			got.White, got.Black, got.Spec)
	}
}
