package view

import (
	"strconv"

	"github.com/dechristopher/lio/db"
)

// Player feedback: the "something's wrong / something's great" channel, offered
// from the profile popover and read from the /system console.
//
// The moderation vocabulary deliberately does not carry over. A report is an
// accusation with a queue, a decision and a permanent record; feedback is
// somebody trying to help, and dressing it in the same tinted-severity chrome
// would frame every "the clock looks odd on my phone" as an incident.

// FeedbackInbox is the /system section's state.
type FeedbackInbox struct {
	Items []FeedbackView
	// Unread is the badge count — everything nobody has read yet, which may
	// exceed what this page shows.
	Unread int64
	// Total is everything ever submitted, so a bounded page can say what it is
	// leaving out rather than looking like the whole history.
	Total int64
	// Shown is how many rows this page is rendering, so the template can work
	// out the remainder without counting the slice in two places.
	Shown int
}

// More reports how many submissions exist beyond the ones rendered.
func (f FeedbackInbox) More() int64 {
	if f.Total <= int64(f.Shown) {
		return 0
	}
	return f.Total - int64(f.Shown)
}

// FeedbackView is one submission as rendered.
type FeedbackView struct {
	ID string
	// When it was sent, and the exact stamp for the tooltip.
	When      string
	WhenExact string
	// Kind is the submission's own word for itself; Label renders it and Class
	// tints it.
	Kind  string
	Class string
	Label string
	Body  string
	// Author is the account that sent it; Path the page they were on, when the
	// client reported a usable one.
	Author string
	Path   string
	// Unread drives the row's tint and whether it offers a Mark read control.
	Unread bool
	// Reader names whoever read it, on a read row.
	Reader string
}

// FeedbackKindClass tints a submission by what it is. Praise reads as a win and
// problems as a loss, borrowing the same semantics the clock and the audit log
// use; an idea is neither, so it takes the neutral accent rather than implying
// something is wrong.
func FeedbackKindClass(kind string) string {
	switch kind {
	case "praise":
		return "fb-praise"
	case "problem":
		return "fb-problem"
	}
	return "fb-idea"
}

// feedbackReadClass marks a row as still-unread, which is what tints it and
// lifts it out of the read history around it.
func feedbackReadClass(f FeedbackView) string {
	if f.Unread {
		return "is-unread"
	}
	return ""
}

// FeedbackKindLabel renders a kind for the inbox chip.
func FeedbackKindLabel(kind string) string {
	switch kind {
	case "problem":
		return "problem"
	case "praise":
		return "praise"
	case "idea":
		return "idea"
	}
	return kind
}

// FeedbackKindPrompt is the picker's copy for one kind: what a player would
// call the thing they are about to describe, in their words rather than the
// site's. Paired with FeedbackKindHint, which says what to write.
func FeedbackKindPrompt(kind string) string {
	switch kind {
	case "problem":
		return "Something's wrong"
	case "praise":
		return "Something's great"
	case "idea":
		return "I have an idea"
	}
	return kind
}

// FeedbackKindHint is the one-line explanation under each choice in the picker.
func FeedbackKindHint(kind string) string {
	switch kind {
	case "problem":
		return "A bug, or something that doesn't work right"
	case "praise":
		return "Something you like, love, or want to see more of"
	case "idea":
		return "Something you wish the site did"
	}
	return ""
}

// FeedbackKinds is the ordered list the picker offers. It mirrors
// db.FeedbackKinds — the set the CHECK constraint accepts — rather than keeping
// a second list that can drift from it.
func FeedbackKinds() []string {
	return db.FeedbackKinds
}

// FeedbackCountLabel summarizes the inbox for its heading.
func FeedbackCountLabel(unread, total int64) string {
	if total == 0 {
		return "nothing yet"
	}
	if unread == 0 {
		return "all read"
	}
	return strconv.FormatInt(unread, 10) + " unread"
}

// UnreadBadgeLabel is the accessible name and tooltip on the red dot. It states
// the count, because a bare dot tells a moderator there is something without
// telling them whether it is one note or forty.
func UnreadBadgeLabel(n int64) string {
	if n == 1 {
		return "1 unread feedback message"
	}
	return strconv.FormatInt(n, 10) + " unread feedback messages"
}
