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

// A broadcast frame reaches every account at once, so it can carry no personal
// count at all: any number on it would be one reader's total written over
// everybody else's. The client adds one itself for this frame and only this
// frame (see NotifyBroadcastMessage), which only works while the field stays
// absent — a 0 here would blank every badge on the site instead.
func TestNotifyBroadcastMessageCarriesNoCounts(t *testing.T) {
	out := string(NotifyBroadcastMessage(NotifyItem{
		ID: 4, Kind: "announce", Body: "the server restarts at 9pm", Broadcast: true,
	}))
	if strings.Contains(out, `"n"`) {
		t.Errorf("broadcast frame carried a personal count it cannot know: %s", out)
	}
	if strings.Contains(out, `"s"`) {
		t.Errorf("broadcast frame carried a staff count: %s", out)
	}
	if !strings.Contains(out, `"i":`) {
		t.Errorf("broadcast frame missing the item: %s", out)
	}
	if !strings.Contains(out, `"bc":true`) {
		t.Errorf("broadcast frame lost its store marker, so the client would "+
			"address the row as a notification: %s", out)
	}
}

// The two stores have separate id sequences, so "bc" is half the client's row
// key. An ordinary notification must never carry it — a row that claimed to be
// a broadcast would have its read written to the wrong place.
func TestNotifyItemBroadcastFlagIsAbsentByDefault(t *testing.T) {
	out := string(NotifyMessage(1, NotifyItem{ID: 4, Kind: "system", Body: "x"}))
	if strings.Contains(out, `"bc"`) {
		t.Errorf("a notification claimed to be a broadcast: %s", out)
	}
}

// A message that asks a question carries its options, and — once answered —
// what this reader chose. Both drive the row's shape: options mean buttons and
// "not finished by being looked at", and an answer replaces them.
func TestNotifyItemChoices(t *testing.T) {
	asking := string(NotifyMessage(1, NotifyItem{
		ID: 9, Kind: "system", Body: "new terms", Choices: []string{"Yes", "No"},
	}))
	if !strings.Contains(asking, `"c":["Yes","No"]`) {
		t.Errorf("question lost its options, so the row would offer no way to clear it: %s", asking)
	}
	if strings.Contains(asking, `"an"`) {
		t.Errorf("an unanswered question came through as answered: %s", asking)
	}

	answered := string(NotifyMessage(0, NotifyItem{
		ID: 9, Kind: "system", Body: "new terms",
		Choices: []string{"Yes", "No"}, Response: "Yes", Read: true,
	}))
	if !strings.Contains(answered, `"an":"Yes"`) {
		t.Errorf("answered row lost the answer, so a second tab would offer the "+
			"buttons again: %s", answered)
	}

	plain := string(NotifyMessage(1, NotifyItem{ID: 9, Kind: "system", Body: "x"}))
	if strings.Contains(plain, `"c"`) {
		t.Errorf("an ordinary message came through as a question: %s", plain)
	}
}
