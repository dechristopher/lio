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
