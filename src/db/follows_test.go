package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// mkFollowUser creates a throwaway account and registers its cleanup. The
// cascade on follows.follower_id / followee_id means deleting the user takes
// its edges with it, which is also what TestFollowCascade asserts.
func mkFollowUser(t *testing.T, tag string) int64 {
	t.Helper()
	email := "fol" + tag + uuid.NewString()[:6] + "@example.invalid"
	id, err := CreateUser("fol"+tag+time.Now().Format("150405.000000"), &email, "$argon2id$fake")
	if err != nil {
		t.Fatalf("create user %s: %v", tag, err)
	}
	t.Cleanup(func() {
		_, _ = Pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	return id
}

// TestFollowIdempotent covers the write path's whole contract: a first follow
// creates an edge and says so, a repeat says nothing was created, and unfollow
// mirrors both. The created/removed flags matter beyond tidiness — they are
// what stops a follow/unfollow/refollow loop from generating notifications.
func TestFollowIdempotent(t *testing.T) {
	skipNoDB(t)

	a, b := mkFollowUser(t, "a"), mkFollowUser(t, "b")

	created, err := Follow(a, b)
	if err != nil {
		t.Fatalf("follow: %v", err)
	}
	if !created {
		t.Fatal("first follow did not report a new edge")
	}
	if !IsFollowing(a, b) {
		t.Fatal("follow did not take")
	}
	// following is directed: b does not follow a as a side effect
	if IsFollowing(b, a) {
		t.Fatal("follow was mutual")
	}

	created, err = Follow(a, b)
	if err != nil {
		t.Fatalf("repeat follow: %v", err)
	}
	if created {
		t.Fatal("repeat follow reported a new edge")
	}

	removed, err := Unfollow(a, b)
	if err != nil {
		t.Fatalf("unfollow: %v", err)
	}
	if !removed {
		t.Fatal("unfollow did not report a removal")
	}
	if IsFollowing(a, b) {
		t.Fatal("unfollow did not take")
	}

	removed, err = Unfollow(a, b)
	if err != nil {
		t.Fatalf("repeat unfollow: %v", err)
	}
	if removed {
		t.Fatal("repeat unfollow reported a removal")
	}
}

// TestFollowSelf checks that an account cannot follow itself. The CHECK
// constraint is the real guard; Follow refuses first so the nonsense reads as
// nonsense rather than as a storage failure.
func TestFollowSelf(t *testing.T) {
	skipNoDB(t)

	a := mkFollowUser(t, "self")
	created, err := Follow(a, a)
	if err != nil {
		t.Fatalf("self follow returned an error: %v", err)
	}
	if created {
		t.Fatal("an account followed itself")
	}
	if IsFollowing(a, a) {
		t.Fatal("a self-follow edge exists")
	}

	// and the constraint holds even if something reaches past the accessor
	ctx, cancel := Ctx()
	defer cancel()
	if _, err := Pool.Exec(ctx,
		"INSERT INTO follows (follower_id, followee_id) VALUES ($1, $1)", a); err == nil {
		t.Fatal("follows_not_self did not refuse a direct insert")
	}
}

// TestFollowCounts checks the pair of numbers a player page prints, including
// that a banned account leaves both the count and (by the same filter) the
// list it would open. The count and the list must agree, or a profile shows
// "2 followers" over a list of one.
func TestFollowCounts(t *testing.T) {
	skipNoDB(t)

	star := mkFollowUser(t, "star")
	fan1, fan2 := mkFollowUser(t, "fan1"), mkFollowUser(t, "fan2")

	for _, fan := range []int64{fan1, fan2} {
		if _, err := Follow(fan, star); err != nil {
			t.Fatalf("follow: %v", err)
		}
	}
	if _, err := Follow(star, fan1); err != nil {
		t.Fatalf("follow back: %v", err)
	}

	counts, err := FollowCountsForUser(star)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts.Followers != 2 || counts.Following != 1 {
		t.Fatalf("counts = %+v, want 2 followers / 1 following", counts)
	}

	// ban one follower: the edge survives, but it stops being visible
	ctx, cancel := Ctx()
	defer cancel()
	if _, err := Pool.Exec(ctx,
		"UPDATE users SET banned_until = 'infinity' WHERE id = $1", fan2); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if counts, err = FollowCountsForUser(star); err != nil {
		t.Fatalf("counts after ban: %v", err)
	}
	if counts.Followers != 1 {
		t.Fatalf("banned follower still counted: %+v", counts)
	}

	// a ban is not a delete: lifting it restores the count
	if _, err := Pool.Exec(ctx,
		"UPDATE users SET banned_until = NULL WHERE id = $1", fan2); err != nil {
		t.Fatalf("unban: %v", err)
	}
	if counts, err = FollowCountsForUser(star); err != nil {
		t.Fatalf("counts after unban: %v", err)
	}
	if counts.Followers != 2 {
		t.Fatalf("unbanned follower not restored: %+v", counts)
	}
}

// TestFollowCap fills an account to MaxFollowing and checks that the next
// follow is refused with ErrFollowLimit, and that freeing a slot lets one
// through again.
//
// The fixture is built with two bulk statements rather than MaxFollowing calls
// to Follow: what is under test is the check, not the insert, and a thousand
// round trips would make this the slowest test in the package for no added
// coverage.
func TestFollowCap(t *testing.T) {
	skipNoDB(t)

	a := mkFollowUser(t, "cap")
	tag := "capfill" + uuid.NewString()[:8]

	ctx, cancel := Ctx()
	defer cancel()
	t.Cleanup(func() {
		// cascades the follows rows with them
		_, _ = Pool.Exec(context.Background(),
			"DELETE FROM users WHERE username LIKE $1", tag+"%")
	})

	if _, err := Pool.Exec(ctx,
		`INSERT INTO users (username, password_hash)
		 SELECT $1 || g, '$argon2id$fake' FROM generate_series(1, $2) g`,
		tag, MaxFollowing); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
	if _, err := Pool.Exec(ctx,
		`INSERT INTO follows (follower_id, followee_id)
		 SELECT $1, id FROM users WHERE username LIKE $2`, a, tag+"%"); err != nil {
		t.Fatalf("seed follows: %v", err)
	}

	over := mkFollowUser(t, "capover")
	_, err := Follow(a, over)
	if !errors.Is(err, ErrFollowLimit) {
		t.Fatalf("follow at the cap: err = %v, want ErrFollowLimit", err)
	}
	if IsFollowing(a, over) {
		t.Fatal("a follow past the cap was stored")
	}

	// free one slot: the cap is a bound on what is held, not a lifetime quota
	var freed int64
	if err := Pool.QueryRow(ctx,
		`DELETE FROM follows WHERE follower_id = $1
		 AND followee_id = (SELECT id FROM users WHERE username LIKE $2 LIMIT 1)
		 RETURNING followee_id`, a, tag+"%").Scan(&freed); err != nil {
		t.Fatalf("free a slot: %v", err)
	}
	if created, err := Follow(a, over); err != nil || !created {
		t.Fatalf("follow after freeing a slot: created = %v, err = %v", created, err)
	}
}

// TestFollowLists covers the two list reads: newest follow first, paging that
// does not repeat or skip a row, and a banned account leaving the list exactly
// as it leaves the count.
func TestFollowLists(t *testing.T) {
	skipNoDB(t)

	star := mkFollowUser(t, "ls")
	// followed in order, so the list must come back in the reverse of it
	fans := []int64{
		mkFollowUser(t, "lf1"), mkFollowUser(t, "lf2"), mkFollowUser(t, "lf3"),
	}
	names := make([]string, len(fans))
	for i, fan := range fans {
		if _, err := Follow(fan, star); err != nil {
			t.Fatalf("follow: %v", err)
		}
		rec, _, err := GetUserByID(fan)
		if err != nil {
			t.Fatalf("read user: %v", err)
		}
		names[i] = rec.Username
		// distinct created_at stamps; the id tiebreaker covers a tie, but the
		// ordering under test here is the timestamp
		time.Sleep(2 * time.Millisecond)
	}

	got, err := ListFollowers(star, 10, 0)
	if err != nil {
		t.Fatalf("list followers: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d followers, want 3", len(got))
	}
	// newest first: the last account to follow leads
	for i, want := range []string{names[2], names[1], names[0]} {
		if got[i].Username != want {
			t.Fatalf("row %d = %q, want %q (newest first)", i, got[i].Username, want)
		}
	}

	// paging: one row a page, no repeats and no gaps
	seen := map[string]bool{}
	for page := int32(0); page < 3; page++ {
		rows, err := ListFollowers(star, 1, page)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if len(rows) != 1 {
			t.Fatalf("page %d returned %d rows, want 1", page, len(rows))
		}
		if seen[rows[0].Username] {
			t.Fatalf("page %d repeated %q", page, rows[0].Username)
		}
		seen[rows[0].Username] = true
	}
	if len(seen) != 3 {
		t.Fatalf("paging saw %d distinct rows, want 3", len(seen))
	}

	// the mirror direction, and the banned filter on it
	if _, err := Follow(star, fans[0]); err != nil {
		t.Fatalf("follow back: %v", err)
	}
	following, err := ListFollowing(star, 10, 0)
	if err != nil {
		t.Fatalf("list following: %v", err)
	}
	if len(following) != 1 || following[0].Username != names[0] {
		t.Fatalf("following = %+v, want just %q", following, names[0])
	}

	ctx, cancel := Ctx()
	defer cancel()
	if _, err := Pool.Exec(ctx,
		"UPDATE users SET banned_until = 'infinity' WHERE id = $1", fans[0]); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if following, err = ListFollowing(star, 10, 0); err != nil {
		t.Fatalf("list following after ban: %v", err)
	}
	if len(following) != 0 {
		t.Fatalf("banned account still listed: %+v", following)
	}
	// and the count agrees with the list, which is the whole point of applying
	// the same filter to both
	counts, err := FollowCountsForUser(star)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts.Following != int64(len(following)) {
		t.Fatalf("count %d disagrees with list of %d", counts.Following, len(following))
	}
}

// TestFollowedAmong covers the feature's load-bearing read: of a given set of
// accounts, exactly the followed ones come back — and an anonymous viewer or an
// empty set costs no query at all.
func TestFollowedAmong(t *testing.T) {
	skipNoDB(t)

	me := mkFollowUser(t, "fa")
	yes1, yes2, no := mkFollowUser(t, "fay1"), mkFollowUser(t, "fay2"), mkFollowUser(t, "fan")
	for _, id := range []int64{yes1, yes2} {
		if _, err := Follow(me, id); err != nil {
			t.Fatalf("follow: %v", err)
		}
	}

	got, err := FollowedAmong(me, []int64{yes1, no, yes2})
	if err != nil {
		t.Fatalf("followed among: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(got), got)
	}
	if _, ok := got[yes1]; !ok {
		t.Fatal("a followed account was missing")
	}
	if _, ok := got[no]; ok {
		t.Fatal("an unfollowed account came back")
	}

	// an account absent from the probe set is absent from the answer, even
	// though it is followed — the query answers about the set it was given
	got, err = FollowedAmong(me, []int64{no})
	if err != nil || len(got) != 0 {
		t.Fatalf("narrow probe = %+v, err %v", got, err)
	}

	// the two no-query paths
	if got, err = FollowedAmong(0, []int64{yes1}); err != nil || len(got) != 0 {
		t.Fatalf("anonymous viewer = %+v, err %v", got, err)
	}
	if got, err = FollowedAmong(me, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty set = %+v, err %v", got, err)
	}
}

// TestFollowCascade checks that deleting an account takes its edges with it, in
// both directions. Without the cascade a deleted account would keep inflating
// everybody else's counts.
func TestFollowCascade(t *testing.T) {
	skipNoDB(t)

	keep, gone := mkFollowUser(t, "keep"), mkFollowUser(t, "gone")
	if _, err := Follow(keep, gone); err != nil {
		t.Fatalf("follow: %v", err)
	}
	if _, err := Follow(gone, keep); err != nil {
		t.Fatalf("follow back: %v", err)
	}

	ctx, cancel := Ctx()
	defer cancel()
	if _, err := Pool.Exec(ctx, "DELETE FROM users WHERE id = $1", gone); err != nil {
		t.Fatalf("delete: %v", err)
	}

	counts, err := FollowCountsForUser(keep)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts.Followers != 0 || counts.Following != 0 {
		t.Fatalf("edges survived the account: %+v", counts)
	}
}

// TestFollowNotificationSuppression covers the day-window guard that stops a
// follow/unfollow/refollow loop from being a notification generator. The guard
// is what the producer consults; every new edge is genuinely new, so without it
// each turn of the loop would announce itself.
func TestFollowNotificationSuppression(t *testing.T) {
	skipNoDB(t)

	target, actor := mkFollowUser(t, "nt"), mkFollowUser(t, "na")

	// nothing said yet, so the producer would speak
	if RecentFollowNotice(target, actor) {
		t.Fatal("suppressed with no notification on record")
	}

	row, err := CreateNotification(NewNotification{
		UserID:  target,
		ActorID: &actor,
		Kind:    KindFollow,
		Body:    "You have a new follower",
		Link:    "/@/whoever",
	})
	if err != nil {
		t.Fatalf("create notification: %v", err)
	}
	if row.ID == 0 {
		t.Fatal("notification not stored")
	}
	if row.Kind != KindFollow {
		t.Fatalf("kind = %q, want %q", row.Kind, KindFollow)
	}

	// the same pair, inside the window: silence
	if !RecentFollowNotice(target, actor) {
		t.Fatal("a second notice about the same follower was not suppressed")
	}
	// a different follower is a different piece of news
	other := mkFollowUser(t, "no")
	if RecentFollowNotice(target, other) {
		t.Fatal("a different follower was suppressed")
	}
	// and so is the same follower once the window has passed
	ctx, cancel := Ctx()
	defer cancel()
	if _, err := Pool.Exec(ctx,
		"UPDATE notifications SET created_at = now() - INTERVAL '2 days' WHERE id = $1",
		row.ID); err != nil {
		t.Fatalf("age the row: %v", err)
	}
	if RecentFollowNotice(target, actor) {
		t.Fatal("an expired notice still suppressed a new one")
	}
}
