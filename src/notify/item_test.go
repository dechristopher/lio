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
	}, nil)

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
	}, nil)
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
	}, nil)
	if !item.Read {
		t.Fatal("a read notification came through as unread")
	}
}

// The follow row's toggle is the reader's own edge back to their new follower,
// so the state has to reach the item. Without it every row paints "Follow back"
// and a mutual follow offers to do again what is already done.
func TestItemCarriesFollowState(t *testing.T) {
	row := db.Notification{
		ID:      10,
		Kind:    db.KindFollow,
		Body:    "You have a new follower",
		Link:    "/@/drewtest",
		Actor:   "drewtest",
		ActorID: 42,
		Created: time.Now(),
	}

	var asked int64
	item := Item(row, func(actorID int64) bool {
		asked = actorID
		return true
	})
	if asked != 42 {
		t.Fatalf("lookup asked about account %d, want the actor 42", asked)
	}
	if !item.Follows {
		t.Error("Follows = false for a reader who already follows the actor")
	}

	if Item(row, func(int64) bool { return false }).Follows {
		t.Error("Follows = true for a reader who does not follow the actor")
	}
}

// Every other kind is the same message for anybody, so nothing viewer-relative
// is resolved for it. This is a cost test as much as a correctness one: the
// panel would otherwise probe the follow graph for a page of rating records.
func TestItemSkipsFollowLookupForOtherKinds(t *testing.T) {
	kinds := []string{db.KindModAction, db.KindMilestone, db.KindSystem, db.KindChallenge}
	for _, kind := range kinds {
		item := Item(db.Notification{
			ID:      11,
			Kind:    kind,
			Body:    "something happened",
			Actor:   "drewtest",
			ActorID: 42,
			Created: time.Now(),
		}, func(int64) bool {
			t.Fatalf("kind %s asked the follow graph a question it has no use for", kind)
			return true
		})
		if item.Follows {
			t.Errorf("kind %s came through as a follow relationship", kind)
		}
	}
}

// An actor who deleted their account leaves the id NULL, and there is nobody
// left to follow. The row still renders — the message reads on its own — but it
// must not ask the graph about account 0, which is every anonymous session.
func TestItemFollowWithNoActor(t *testing.T) {
	item := Item(db.Notification{
		ID:      12,
		Kind:    db.KindFollow,
		Body:    "You have a new follower",
		Created: time.Now(),
	}, func(int64) bool {
		t.Fatal("asked the follow graph about a deleted actor")
		return true
	})
	if item.Follows {
		t.Error("Follows = true with no actor to follow")
	}
}

// BroadcastItem is the twin of Item, and the same rule applies to it: the
// panel's list and the frame that arrives live must describe one message
// identically, so both build the row here. A field this mapping forgets is a
// row the client cannot act on — which is exactly how the challenge expiry was
// lost once already.
func TestBroadcastItemCarriesEverythingTheRowNeeds(t *testing.T) {
	created := time.Now().Add(-time.Minute)
	expires := time.Now().Add(24 * time.Hour)
	item := BroadcastItem(db.Broadcast{
		ID:      12,
		Created: created,
		Body:    "the server restarts at 9pm",
		Link:    "/news",
		Choices: []string{"OK"},
		Expires: expires,
	})

	if !item.Broadcast {
		t.Fatal("a broadcast row did not say so, so the client would address it " +
			"as a notification and write the read to the wrong store")
	}
	if item.Kind != KindAnnounce {
		t.Errorf("Kind = %q, want %q", item.Kind, KindAnnounce)
	}
	if item.ID != 12 || item.Body == "" || item.Link != "/news" {
		t.Errorf("item lost a field: %+v", item)
	}
	if len(item.Choices) != 1 || item.Choices[0] != "OK" {
		t.Errorf("Choices = %v — a question with no options can never be cleared", item.Choices)
	}
	if item.Response != "" {
		t.Error("an unanswered broadcast came through as answered")
	}
	if item.Created != created.UnixMilli() {
		t.Errorf("Created = %d, want %d", item.Created, created.UnixMilli())
	}
	if item.Expires != expires.UnixMilli() {
		t.Errorf("Expires = %d, want %d", item.Expires, expires.UnixMilli())
	}
}

// Zero means "never expires" on the wire. It must stay 0 rather than become the
// unix epoch in milliseconds, which the client compares against now and reads as
// long past — so an open-ended announcement would render as one that had
// already ended.
func TestBroadcastItemOpenEndedHasNoExpiry(t *testing.T) {
	item := BroadcastItem(db.Broadcast{ID: 1, Created: time.Now(), Body: "x"})
	if item.Expires != 0 {
		t.Errorf("Expires = %d, want 0 for a broadcast that runs until it is retired",
			item.Expires)
	}
}

// A reader who has answered gets the answer back, not the buttons. Without it a
// second tab would offer the question again, and the reader could not tell
// which way they had already answered.
func TestBroadcastItemCarriesTheReadersAnswer(t *testing.T) {
	item := BroadcastItem(db.Broadcast{
		ID: 3, Created: time.Now(), Body: "new terms",
		Choices: []string{"Accept", "Decline"}, Response: "Accept", Read: true,
	})
	if item.Response != "Accept" {
		t.Errorf("Response = %q, want Accept", item.Response)
	}
	if !item.Read {
		t.Error("an answered broadcast came through unread")
	}
}

// The acknowledgement flag belongs to notifications too, so Item must carry it
// for a message addressed to one account.
func TestItemCarriesChoices(t *testing.T) {
	item := Item(db.Notification{
		ID: 5, Kind: db.KindSystem, Body: "please confirm", Created: time.Now(),
		Choices: []string{"OK"},
	}, nil)
	if len(item.Choices) != 1 || item.Choices[0] != "OK" {
		t.Errorf("Choices = %v — the row would render with no way to clear it", item.Choices)
	}
	if item.Broadcast {
		t.Error("a notification claimed to be a broadcast")
	}
}
