package settings

import "testing"

// A cleared banner must leave nothing behind that can affect the next one.
// The handler deletes both overrides when the text is cleared; these lock the
// resolution rules that make that the right thing to do — an orphaned level
// would otherwise sit in the table waiting to restyle an unrelated notice.
func TestNoticeLevelWithoutTextRendersNothing(t *testing.T) {
	// a level with no text is not a state the site has: nothing renders it
	s := resolve(map[string]string{KeyNoticeLevel: LevelWarn})
	if s.NoticeText != "" {
		t.Fatalf("notice text = %q, want empty", s.NoticeText)
	}

	// and once both overrides are gone, the next notice starts from the
	// default styling rather than inheriting the previous one's
	cleared := resolve(map[string]string{})
	if cleared.NoticeLevel != LevelInfo {
		t.Errorf("level after clearing = %q, want %q", cleared.NoticeLevel, LevelInfo)
	}
	if cleared.NoticeText != "" {
		t.Errorf("text after clearing = %q, want empty", cleared.NoticeText)
	}
}
