package room

import "testing"

// TestStateLive locks which room states count as a live board. The predicate
// backs three things that must agree: the home page's "Live" counter, the
// live-games list behind it (and the /system console's room list), and the
// "playing" marker on a member in the online roster.
//
// The case worth guarding is StateDeploy. It used to be excluded, so a room
// whose players were mid blind-deploy showed on the TV grid while the counter
// beside it said the site had no live games, and the members at that board read
// as merely browsing. The deploy is a phase of the game with both players
// present and a clock running on the arrangement — any active room is live.
func TestStateLive(t *testing.T) {
	tests := []struct {
		state State
		want  bool
		why   string
	}{
		{StateGameReady, true, "both players seated, about to begin"},
		{StateDeploy, true, "blind deploy is a phase of the game, not a lobby"},
		{StateGameOngoing, true, "moves being traded"},

		{StateInit, false, "not yet a room"},
		{StateWaitingForPlayers, false, "a seek nobody has taken; listed as a challenge"},
		{StateGameOver, false, "position frozen; a rematch re-enters game_ready"},
		{StateRoomOver, false, "room torn down"},
	}

	for _, tt := range tests {
		if got := tt.state.Live(); got != tt.want {
			t.Errorf("State(%q).Live() = %v, want %v — %s", tt.state, got, tt.want, tt.why)
		}
	}
}
