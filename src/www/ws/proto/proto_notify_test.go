package proto

import (
	"strings"
	"testing"
)

// The notification frames carry only the counts they actually know, and the
// difference between "absent" and "zero" is load-bearing on both sides:
//
//   - A staff frame goes to every moderator at once. It cannot know any one of
//     their personal counts, so it must omit "n" — a 0 there would blank each
//     moderator's own badge.
//   - A count of zero must still be sent when it is known. It is the value that
//     clears a badge after the last message is read, and an omitted 0 would
//     leave the badge lit until the next navigation.
//
// A plain int64 field cannot express both. These tests are what stops the
// pointers being "simplified" back into values.

func TestNotifyCountMessageSendsZeroCounts(t *testing.T) {
	out := string(NotifyCountMessage(0, 0))
	if !strings.Contains(out, `"n":0`) {
		t.Errorf("connect frame dropped a zero unread count: %s", out)
	}
	if !strings.Contains(out, `"s":0`) {
		t.Errorf("connect frame dropped a zero staff count: %s", out)
	}
}

func TestNotifyStaffMessageOmitsPersonalCount(t *testing.T) {
	out := string(NotifyStaffMessage(3))
	if strings.Contains(out, `"n"`) {
		t.Errorf("staff frame carried a personal count and would overwrite it: %s", out)
	}
	if !strings.Contains(out, `"s":3`) {
		t.Errorf("staff frame missing the staff count: %s", out)
	}
	// zero is the interesting case: it is how a cleared inbox is announced
	if cleared := string(NotifyStaffMessage(0)); !strings.Contains(cleared, `"s":0`) {
		t.Errorf("staff frame dropped a zero count, so a cleared inbox would stay lit: %s", cleared)
	}
}

func TestNotifyMessageOmitsStaffCount(t *testing.T) {
	out := string(NotifyMessage(2, NotifyItem{ID: 1, Kind: "system", Body: "hello"}))
	if strings.Contains(out, `"s"`) {
		t.Errorf("arrival frame carried a staff count it cannot know: %s", out)
	}
	if !strings.Contains(out, `"n":2`) {
		t.Errorf("arrival frame missing the unread count: %s", out)
	}
	if !strings.Contains(out, `"i":`) {
		t.Errorf("arrival frame missing the item: %s", out)
	}
}

// An unread row is the common case, so Read is omitempty and absent means
// unread. The panel's list is the only place it is ever set.
func TestNotifyItemReadFlag(t *testing.T) {
	unreadRow := string(NotifyMessage(1, NotifyItem{ID: 1, Kind: "system", Body: "x"}))
	if strings.Contains(unreadRow, `"r"`) {
		t.Errorf("a new message should not carry a read flag: %s", unreadRow)
	}
	readRow := string(NotifyMessage(1, NotifyItem{ID: 1, Kind: "system", Body: "x", Read: true}))
	if !strings.Contains(readRow, `"r":true`) {
		t.Errorf("a read row lost its flag: %s", readRow)
	}
}
