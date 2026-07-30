package notify

import (
	"testing"
	"time"

	"github.com/dechristopher/lio/db"
)

// The expiry is what makes a challenge actionable in the client: the panel and
// the toast both test it against the clock, and a challenge that arrives with no
// expiry reads as one that already ran out. It then renders as an ordinary
// message — no countdown, no Accept, no Decline, no toast — which is exactly the
// bug this mapping was extracted to prevent. The two hand-written copies it
// replaced had both forgotten this field.
func TestItemCarriesExpiry(t *testing.T) {
	expires := time.Now().Add(5 * time.Minute)
	item := Item(db.Notification{
		ID:      7,
		Kind:    db.KindChallenge,
		Body:    "drewtest challenges you",
		Link:    "/Ab3xY9",
		Actor:   "drewtest",
		Created: time.Now(),
		Expires: expires,
	})

	if item.Expires != expires.UnixMilli() {
		t.Fatalf("Expires = %d, want %d — a challenge with no expiry is dead on arrival",
			item.Expires, expires.UnixMilli())
	}
	if item.ID != 7 || item.Kind != db.KindChallenge || item.Link != "/Ab3xY9" {
		t.Errorf("item lost a field: %+v", item)
	}
	if item.Actor != "drewtest" {
		t.Errorf("Actor = %q, want drewtest", item.Actor)
	}
	if item.Read {
		t.Error("a notification with no read stamp came through as read")
	}
}

// A kind that does not expire must send 0, not the unix epoch. The client
// compares the value against the current time, so an epoch timestamp would read
// as "expired in 1970" — harmless for a message with no action, but it would
// make the field meaningless as a signal.
func TestItemWithoutExpiryIsZero(t *testing.T) {
	item := Item(db.Notification{
		ID:      8,
		Kind:    db.KindSystem,
		Body:    "the site was updated",
		Created: time.Now(),
	})
	if item.Expires != 0 {
		t.Fatalf("Expires = %d, want 0 for a kind that does not expire", item.Expires)
	}
}

// Read state comes from the stored stamp, and is what stops a declined
// challenge still offering its buttons after a reload.
func TestItemReadState(t *testing.T) {
	item := Item(db.Notification{
		ID:      9,
		Kind:    db.KindModAction,
		Body:    "your report was reviewed",
		Created: time.Now(),
		Read:    time.Now(),
	})
	if !item.Read {
		t.Fatal("a read notification came through as unread")
	}
}
