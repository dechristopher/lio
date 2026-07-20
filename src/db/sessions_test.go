package db

import (
	"crypto/sha256"
	"testing"
	"time"
)

// TestSessionLifecycle exercises the session rows behind the unified identity
// system against a real Postgres: mint anon → resolve → login upgrade in
// place (uid preserved) → touch → revoke. Skips without DEV_LIO_PG_DSN
// (dev/dev.sh up), like the archive tests.
func TestSessionLifecycle(t *testing.T) {
	skipNoDB(t)

	hash := sha256.Sum256([]byte("session-test-token-" +
		time.Now().Format(time.RFC3339Nano)))
	uid := "testuid_sess1"

	id, err := CreateSession(hash[:], uid, nil,
		time.Now().Add(time.Hour), "go-test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	rec, found, err := GetSessionByTokenHash(hash[:])
	if err != nil || !found {
		t.Fatalf("resolve: found=%v err=%v", found, err)
	}
	if rec.ID != id || rec.UID != uid || rec.UserID != nil || rec.Username != "" {
		t.Fatalf("anon session mismatch: %+v", rec)
	}

	// login upgrade: new hash, account attached, same row + uid. Username is
	// unique per run (unique index would fail a re-run) and the user row is
	// removed at the end, cascading away any leftover sessions.
	username := "sesstest" + time.Now().Format("0102150405")
	email := "sess-test@example.invalid"
	userID, err := CreateUser(username, &email, "$argon2id$fake")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := Ctx()
		defer cancel()
		_, _ = Pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})
	newHash := sha256.Sum256([]byte("rotated-token"))
	if err := RotateSessionToken(id, newHash[:], userID,
		time.Now().Add(2*time.Hour)); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, found, _ := GetSessionByTokenHash(hash[:]); found {
		t.Fatal("old token still resolves after rotation")
	}
	rec, found, err = GetSessionByTokenHash(newHash[:])
	if err != nil || !found {
		t.Fatalf("resolve rotated: found=%v err=%v", found, err)
	}
	if rec.UID != uid {
		t.Fatalf("uid not preserved across upgrade: %q != %q", rec.UID, uid)
	}
	if rec.UserID == nil || *rec.UserID != userID ||
		rec.Username != username {
		t.Fatalf("account not attached: %+v", rec)
	}

	if err := TouchSession(id, time.Now().Add(3*time.Hour)); err != nil {
		t.Fatalf("touch: %v", err)
	}

	if err := DeleteSessionsForUser(userID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, found, _ := GetSessionByTokenHash(newHash[:]); found {
		t.Fatal("session survives revocation")
	}
}

// TestSessionAdmin exercises the Phase-3 account-admin queries against a real
// Postgres: list a user's sessions, revoke one by id (owner-scoped), keep the
// current one on a password-change sweep, and fetch the user by id.
func TestSessionAdmin(t *testing.T) {
	skipNoDB(t)

	username := "admintest" + time.Now().Format("0102150405.000")
	email := "admin-test@example.invalid"
	uid, err := CreateUser(username, &email, "$argon2id$fake")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := Ctx()
		defer cancel()
		_, _ = Pool.Exec(ctx, "DELETE FROM users WHERE id = $1", uid)
	})

	// GetUserByID round-trips
	u, found, err := GetUserByID(uid)
	if err != nil || !found || u.Username != username {
		t.Fatalf("GetUserByID: found=%v user=%+v err=%v", found, u, err)
	}

	// three authed sessions for this user
	var ids []int64
	for i := 0; i < 3; i++ {
		h := sha256.Sum256([]byte(username + "-sess-" + time.Now().Format(time.RFC3339Nano) + string(rune('a'+i))))
		id, err := CreateSession(h[:], "uid_admin", &uid, time.Now().Add(time.Hour), "go-test UA")
		if err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
		ids = append(ids, id)
	}

	if list, err := ListSessionsForUser(uid); err != nil || len(list) != 3 {
		t.Fatalf("ListSessionsForUser: n=%d err=%v", len(list), err)
	}

	// revoke one by id, owner-scoped
	if err := DeleteSessionByID(ids[0], uid); err != nil {
		t.Fatalf("DeleteSessionByID: %v", err)
	}
	// a different user's id must not delete this user's session
	if err := DeleteSessionByID(ids[1], uid+99999); err != nil {
		t.Fatalf("DeleteSessionByID (wrong owner): %v", err)
	}
	if list, _ := ListSessionsForUser(uid); len(list) != 2 {
		t.Fatalf("after single revoke: n=%d want 2", len(list))
	}

	// password-change sweep keeps the current session (ids[2]) only
	if err := DeleteSessionsForUserExcept(uid, ids[2]); err != nil {
		t.Fatalf("DeleteSessionsForUserExcept: %v", err)
	}
	list, _ := ListSessionsForUser(uid)
	if len(list) != 1 || list[0].ID != ids[2] {
		t.Fatalf("after except-sweep: %+v (want only id %d)", list, ids[2])
	}
}
