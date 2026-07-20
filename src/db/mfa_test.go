package db

import (
	"crypto/sha256"
	"testing"
	"time"
)

// TestMFALifecycle exercises the Phase-4 MFA accessors against a real Postgres:
// TOTP set/confirm/clear, recovery-code replace/use/count, WebAuthn handle
// set-once, and passkey insert/list/count/sign-count/rename/delete. Skips
// without DEV_LIO_PG_DSN (dev/dev.sh up), like the other db tests.
func TestMFALifecycle(t *testing.T) {
	skipNoDB(t)

	username := "mfatest" + time.Now().Format("0102150405.000")
	email := "mfa-test@example.invalid"
	uid, err := CreateUser(username, &email, "$argon2id$fake")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := Ctx()
		defer cancel()
		_, _ = Pool.Exec(ctx, "DELETE FROM users WHERE id = $1", uid) // cascades
	})

	// --- TOTP: unset → stored-unconfirmed → confirmed → cleared ---
	if enc, confirmed, err := GetTOTP(uid); err != nil || enc != nil || confirmed {
		t.Fatalf("initial TOTP: enc=%v confirmed=%v err=%v", enc, confirmed, err)
	}
	secret := []byte("enc-secret-blob")
	if err := SetTOTPSecret(uid, secret); err != nil {
		t.Fatalf("SetTOTPSecret: %v", err)
	}
	if enc, confirmed, _ := GetTOTP(uid); string(enc) != string(secret) || confirmed {
		t.Fatalf("after set: enc=%q confirmed=%v", enc, confirmed)
	}
	if err := ConfirmTOTP(uid); err != nil {
		t.Fatalf("ConfirmTOTP: %v", err)
	}
	if _, confirmed, _ := GetTOTP(uid); !confirmed {
		t.Fatal("TOTP not confirmed after ConfirmTOTP")
	}
	if err := ClearTOTP(uid); err != nil {
		t.Fatalf("ClearTOTP: %v", err)
	}
	if enc, confirmed, _ := GetTOTP(uid); enc != nil || confirmed {
		t.Fatalf("after clear: enc=%v confirmed=%v", enc, confirmed)
	}

	// --- recovery codes: replace → count → single-use → replace → delete ---
	h := func(s string) []byte { x := sha256.Sum256([]byte(s)); return x[:] }
	if err := ReplaceRecoveryCodes(uid, [][]byte{h("a"), h("b"), h("c")}); err != nil {
		t.Fatalf("ReplaceRecoveryCodes: %v", err)
	}
	if n, _ := CountUnusedRecoveryCodes(uid); n != 3 {
		t.Fatalf("count after replace: %d want 3", n)
	}
	if used, err := UseRecoveryCode(uid, h("a")); err != nil || !used {
		t.Fatalf("UseRecoveryCode(a): used=%v err=%v", used, err)
	}
	if used, _ := UseRecoveryCode(uid, h("a")); used {
		t.Fatal("recovery code reusable (not single-use)")
	}
	if used, _ := UseRecoveryCode(uid, h("nope")); used {
		t.Fatal("unknown recovery code accepted")
	}
	if n, _ := CountUnusedRecoveryCodes(uid); n != 2 {
		t.Fatalf("count after single use: %d want 2", n)
	}
	if err := ReplaceRecoveryCodes(uid, [][]byte{h("d")}); err != nil {
		t.Fatalf("ReplaceRecoveryCodes (regen): %v", err)
	}
	if n, _ := CountUnusedRecoveryCodes(uid); n != 1 {
		t.Fatalf("count after regen: %d want 1", n)
	}
	if err := DeleteRecoveryCodes(uid); err != nil {
		t.Fatalf("DeleteRecoveryCodes: %v", err)
	}
	if n, _ := CountUnusedRecoveryCodes(uid); n != 0 {
		t.Fatalf("count after delete: %d want 0", n)
	}

	// --- WebAuthn user handle: mint-once ---
	if hd, _ := GetWebAuthnUserHandle(uid); hd != nil {
		t.Fatalf("initial handle non-nil: %v", hd)
	}
	handle := []byte("handle-one-32bytes-padding-xxxxxx")
	if err := SetWebAuthnUserHandle(uid, handle); err != nil {
		t.Fatalf("SetWebAuthnUserHandle: %v", err)
	}
	// a second set is a no-op (IS NULL guard) — the original handle survives
	if err := SetWebAuthnUserHandle(uid, []byte("handle-two-should-not-win-yyyyyy")); err != nil {
		t.Fatalf("SetWebAuthnUserHandle (2nd): %v", err)
	}
	if hd, _ := GetWebAuthnUserHandle(uid); string(hd) != string(handle) {
		t.Fatalf("handle not set-once: %q", hd)
	}

	// --- WebAuthn credentials: insert → list → sign-count → rename → delete ---
	credID := []byte("cred-id-1")
	if err := InsertWebAuthnCredential(uid, WebAuthnCredentialRecord{
		CredentialID:    credID,
		PublicKey:       []byte("pubkey"),
		AttestationType: "none",
		AAGUID:          []byte{},
		SignCount:       1,
		Transports:      "internal,hybrid",
		Nickname:        "My laptop",
	}); err != nil {
		t.Fatalf("InsertWebAuthnCredential: %v", err)
	}
	if n, _ := CountWebAuthnCredentials(uid); n != 1 {
		t.Fatalf("passkey count: %d want 1", n)
	}
	creds, err := ListWebAuthnCredentials(uid)
	if err != nil || len(creds) != 1 {
		t.Fatalf("list: n=%d err=%v", len(creds), err)
	}
	if creds[0].Nickname != "My laptop" || creds[0].Transports != "internal,hybrid" || creds[0].SignCount != 1 {
		t.Fatalf("credential mismatch: %+v", creds[0])
	}
	if err := UpdateWebAuthnSignCount(uid, credID, 7); err != nil {
		t.Fatalf("UpdateWebAuthnSignCount: %v", err)
	}
	creds, _ = ListWebAuthnCredentials(uid)
	if creds[0].SignCount != 7 || creds[0].LastUsedAt == nil {
		t.Fatalf("sign-count/last-used not updated: %+v", creds[0])
	}
	if err := RenameWebAuthnCredential(creds[0].ID, uid, "Renamed"); err != nil {
		t.Fatalf("RenameWebAuthnCredential: %v", err)
	}
	creds, _ = ListWebAuthnCredentials(uid)
	if creds[0].Nickname != "Renamed" {
		t.Fatalf("rename failed: %q", creds[0].Nickname)
	}
	// owner-scoped delete: wrong owner is a no-op, right owner removes it
	if err := DeleteWebAuthnCredential(creds[0].ID, uid+99999); err != nil {
		t.Fatalf("delete (wrong owner): %v", err)
	}
	if n, _ := CountWebAuthnCredentials(uid); n != 1 {
		t.Fatal("wrong-owner delete removed the credential")
	}
	if err := DeleteWebAuthnCredential(creds[0].ID, uid); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n, _ := CountWebAuthnCredentials(uid); n != 0 {
		t.Fatalf("passkey count after delete: %d want 0", n)
	}
}
