package auth

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

// rebuildPHC derives an argon2id PHC under explicit params — simulates hashes
// stored by an earlier parameter era.
func rebuildPHC(mem, time uint32, threads uint8, salt []byte, pw string) string {
	key := argon2.IDKey([]byte(pw), salt, time, mem, threads, argonKeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, mem, time, threads, b64(salt), b64(key))
}

// TestPasswordRoundTrip hashes and verifies, and confirms a wrong password
// fails.
func TestPasswordRoundTrip(t *testing.T) {
	phc, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$") {
		t.Fatalf("unexpected PHC shape: %s", phc)
	}

	ok, rehash, err := VerifyPassword(phc, "correct horse battery staple")
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if rehash {
		t.Fatal("fresh hash should not need rehash")
	}

	ok, _, err = VerifyPassword(phc, "wrong password")
	if err != nil {
		t.Fatalf("verify wrong: %v", err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

// TestPasswordRehashDetection verifies a hash minted under weaker params
// (t=1 vs the current t=2) still verifies but reports needsRehash.
func TestPasswordRehashDetection(t *testing.T) {
	phc, err := HashPassword("pw-for-rehash-test")
	if err != nil {
		t.Fatal(err)
	}
	version, mem, _, threads, salt, _, err := parsePHC(phc)
	if err != nil || version != 19 {
		t.Fatalf("parse: v=%d err=%v", version, err)
	}
	old := rebuildPHC(mem, 1, threads, salt, "pw-for-rehash-test")
	ok, rehash, err := VerifyPassword(old, "pw-for-rehash-test")
	if err != nil || !ok {
		t.Fatalf("old-params verify failed: ok=%v err=%v", ok, err)
	}
	if !rehash {
		t.Fatal("old-params hash should need rehash")
	}
}

// TestPasswordMalformedPHC exercises the parser's failure modes.
func TestPasswordMalformedPHC(t *testing.T) {
	for _, phc := range []string{
		"",
		"$argon2i$v=19$m=19456,t=2,p=1$AAAA$BBBB",
		"$argon2id$v=19$m=19456,t=2,p=1$notb64!$AAAA",
		"plainly-not-a-phc",
		"$argon2id$v=99$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		if ok, _, err := VerifyPassword(phc, "x"); ok || err == nil {
			t.Errorf("malformed PHC accepted: %q ok=%v err=%v", phc, ok, err)
		}
	}
}

// TestValidatePassword checks the length bounds.
func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Error("5-char password accepted")
	}
	if err := ValidatePassword(strings.Repeat("a", 129)); err == nil {
		t.Error("129-char password accepted")
	}
	if err := ValidatePassword("just-fine-pw"); err != nil {
		t.Errorf("valid password rejected: %v", err)
	}
}
