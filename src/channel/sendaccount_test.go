package channel

import (
	"testing"
)

// drain reads everything queued on a socket's send buffer without a writer
// goroutine, so a test can assert what SendToAccount enqueued.
func drain(s *Socket) [][]byte {
	var out [][]byte
	for {
		select {
		case msg := <-s.send:
			out = append(out, msg)
		default:
			return out
		}
	}
}

// TestSendToAccountReachesEveryConnection: a notification is addressed to a
// person, and one person holds several sessions. Two devices are two uids on two
// different channels — a game room and the notification channel — and both must
// receive the frame, because each one draws its own badge.
//
// The socket maps key by uid, so this is the case a uid-keyed send gets wrong.
func TestSendToAccountReachesEveryConnection(t *testing.T) {
	const acct int64 = 42

	room := Map.GetSockMap("send-acct-room")
	idle := Map.GetSockMap("send-acct-idle")

	// same account, two devices: different uids, different channels
	laptop := NewSocket(nil, "uid-laptop", "c1", "", acct)
	phone := NewSocket(nil, "uid-phone", "c1", "", acct)
	// a different account, on the same channel as one of them
	other := NewSocket(nil, "uid-other", "c1", "", 99)
	// an anonymous session: no account, so nothing is addressable to it
	anon := NewSocket(nil, "uid-anon", "c1", "", 0)

	room.Track(laptop)
	room.Track(other)
	idle.Track(phone)
	idle.Track(anon)
	t.Cleanup(func() {
		room.UnTrack("uid-laptop", "c1")
		room.UnTrack("uid-other", "c1")
		idle.UnTrack("uid-phone", "c1")
		idle.UnTrack("uid-anon", "c1")
	})

	if sent := SendToAccount(acct, []byte(`{"t":"nt"}`)); sent != 2 {
		t.Fatalf("SendToAccount reached %d connections, want 2", sent)
	}

	for name, s := range map[string]*Socket{"laptop": laptop, "phone": phone} {
		if got := len(drain(s)); got != 1 {
			t.Errorf("%s received %d messages, want 1", name, got)
		}
	}
	for name, s := range map[string]*Socket{"other account": other, "anonymous": anon} {
		if got := len(drain(s)); got != 0 {
			t.Errorf("%s received %d messages, want 0", name, got)
		}
	}
}

// TestSendToAccountsReachesTheGroup: the shared feedback backlog belongs to
// every moderator at once, so its count fans out to a set of accounts in one
// walk. Accounts outside the set must not receive it, and the anonymous marker
// must not match — a 0 in the set would send staff state to every signed-out
// visitor on the site.
func TestSendToAccountsReachesTheGroup(t *testing.T) {
	sm := Map.GetSockMap("send-accts")

	modA := NewSocket(nil, "uid-mod-a", "c1", "", 7)
	modB := NewSocket(nil, "uid-mod-b", "c1", "", 8)
	player := NewSocket(nil, "uid-player", "c1", "", 9)
	anon := NewSocket(nil, "uid-anon-3", "c1", "", 0)

	for _, s := range []*Socket{modA, modB, player, anon} {
		sm.Track(s)
	}
	t.Cleanup(func() {
		for _, uid := range []string{"uid-mod-a", "uid-mod-b", "uid-player", "uid-anon-3"} {
			sm.UnTrack(uid, "c1")
		}
	})

	// the 0 is deliberate: it must be discarded, not matched
	if sent := SendToAccounts([]int64{7, 8, 0}, []byte(`{"t":"nt"}`)); sent != 2 {
		t.Fatalf("SendToAccounts reached %d connections, want 2", sent)
	}
	for name, s := range map[string]*Socket{"mod A": modA, "mod B": modB} {
		if got := len(drain(s)); got != 1 {
			t.Errorf("%s received %d messages, want 1", name, got)
		}
	}
	for name, s := range map[string]*Socket{"player": player, "anonymous": anon} {
		if got := len(drain(s)); got != 0 {
			t.Errorf("%s received %d messages, want 0", name, got)
		}
	}
}

// TestSendToAccountAnonymousIsNoop: account 0 means "no account". It must never
// match the anonymous sockets, which all carry 0 themselves — that would send
// one person's notification to every anonymous visitor on the site.
func TestSendToAccountAnonymousIsNoop(t *testing.T) {
	sm := Map.GetSockMap("send-acct-anon")
	anon := NewSocket(nil, "uid-anon-2", "c1", "", 0)
	sm.Track(anon)
	t.Cleanup(func() { sm.UnTrack("uid-anon-2", "c1") })

	if sent := SendToAccount(0, []byte(`{"t":"nt"}`)); sent != 0 {
		t.Fatalf("SendToAccount(0) reached %d connections, want 0", sent)
	}
	if got := len(drain(anon)); got != 0 {
		t.Errorf("anonymous socket received %d messages, want 0", got)
	}
}
