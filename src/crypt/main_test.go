package crypt

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/dechristopher/lio/config"
)

// TestMain gives the package a working AES-256 key before any test runs.
//
// config.CryptoKey normally comes from the lio_crypto_key secret (or
// DEV_CRYPTO_KEY locally), which nobody has set in a bare `go test ./...`. That
// left the key empty, and gcm() treats a bad key as unrecoverable and calls
// os.Exit(1) — so the package did not report a test failure, it killed the test
// binary, and `go test ./...` failed for everyone running it locally without
// the dev environment loaded.
//
// The key is random per run rather than a fixed literal on purpose: nothing
// here decrypts a stored ciphertext, so the tests must pass under any valid
// key, and a random one keeps a hardcoded value from quietly becoming load
// bearing. An externally configured key still wins, so a run against the dev
// environment exercises the real one.
func TestMain(m *testing.M) {
	if config.CryptoKey == "" {
		key := make([]byte, 16) // hex-encodes to the 32 bytes AES-256 wants
		if _, err := rand.Read(key); err != nil {
			panic("crypt test setup: " + err.Error())
		}
		config.CryptoKey = hex.EncodeToString(key)
	}

	os.Exit(m.Run())
}
