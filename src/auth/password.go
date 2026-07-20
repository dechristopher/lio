package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Password hashing: Argon2id with the OWASP-recommended minimum parameters,
// stored as PHC strings ($argon2id$v=19$m=...,t=...,p=...$salt$key). The
// parameters live inside each stored hash, so they can be raised at any time:
// VerifyPassword reports needsRehash when a stored hash's params lag the
// current ones and the login path re-hashes with the fresh plaintext it
// already holds.
const (
	argonMemoryKiB uint32 = 19 * 1024
	argonTime      uint32 = 2
	argonThreads   uint8  = 1
	argonSaltLen          = 16
	argonKeyLen    uint32 = 32
)

// Password length bounds: at least 8 (NIST 800-63B minimum, no composition
// rules), at most 128 to bound Argon2 input cost.
const (
	PasswordMinLen = 8
	PasswordMaxLen = 128
)

var (
	// ErrPasswordLength rejects out-of-bounds passwords at registration and
	// password change.
	ErrPasswordLength = fmt.Errorf(
		"password must be between %d and %d characters",
		PasswordMinLen, PasswordMaxLen)

	errMalformedPHC = errors.New("malformed password hash")

	// dummyPHC is verified against when a login names an unknown username, so
	// the unknown-user path burns the same Argon2 work as a real verification
	// and response timing does not reveal which usernames exist.
	dummyPHC string
)

func init() {
	var err error
	dummyPHC, err = HashPassword("lio-dummy-password-timing-equalizer")
	if err != nil {
		panic("auth: dummy hash init failed: " + err.Error())
	}
}

// ValidatePassword bounds-checks a candidate password. No composition rules —
// length is the only requirement.
func ValidatePassword(pw string) error {
	if len(pw) < PasswordMinLen || len(pw) > PasswordMaxLen {
		return ErrPasswordLength
	}
	return nil
}

// HashPassword derives an Argon2id hash of the password under the current
// parameters and returns it as a PHC string.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt,
		argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		b64(salt), b64(key)), nil
}

// VerifyPassword checks a password against a stored PHC string in constant
// time (with respect to the derived keys). needsRehash is true when the hash
// verified but was derived under parameters other than the current ones.
func VerifyPassword(phc, pw string) (ok, needsRehash bool, err error) {
	version, memory, time, threads, salt, key, err := parsePHC(phc)
	if err != nil {
		return false, false, err
	}
	if version != argon2.Version {
		// future-versioned hash: fail closed rather than mis-verify
		return false, false, errMalformedPHC
	}
	derived := argon2.IDKey([]byte(pw), salt,
		time, memory, threads, uint32(len(key)))
	if subtle.ConstantTimeCompare(derived, key) != 1 {
		return false, false, nil
	}
	needsRehash = memory != argonMemoryKiB ||
		time != argonTime ||
		threads != argonThreads ||
		uint32(len(key)) != argonKeyLen
	return true, needsRehash, nil
}

// VerifyDummy burns the same Argon2 work as a real verification. Called on
// unknown-username logins so their timing matches known-username failures.
func VerifyDummy(pw string) {
	_, _, _ = VerifyPassword(dummyPHC, pw)
}

// parsePHC splits a $argon2id$v=19$m=...,t=...,p=...$salt$key string.
func parsePHC(phc string) (version int, memory uint32, time uint32,
	threads uint8, salt, key []byte, err error) {
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return 0, 0, 0, 0, nil, nil, errMalformedPHC
	}
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return 0, 0, 0, 0, nil, nil, errMalformedPHC
	}
	var m, t uint32
	var p uint8
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return 0, 0, 0, 0, nil, nil, errMalformedPHC
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return 0, 0, 0, 0, nil, nil, errMalformedPHC
	}
	key, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return 0, 0, 0, 0, nil, nil, errMalformedPHC
	}
	return version, m, t, p, salt, key, nil
}
