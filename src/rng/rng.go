// Package rng provides small, cryptographically secure random helpers backed
// by crypto/rand. They exist as drop-in replacements for the non-secure
// math/rand functions used across the server. It is a leaf package (standard
// library only) so any other package may import it without import cycles.
package rng

import (
	"crypto/rand"
	"math/big"
)

// Intn returns a uniformly distributed, cryptographically secure random int in
// the half-open interval [0, n). It returns 0 when n <= 0.
//
// It panics if the system CSPRNG fails. A failing CSPRNG is not a recoverable
// condition, and the alternative — silently returning predictable values where
// callers expect randomness (token generation, etc.) — would be worse.
func Intn(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic("rng: crypto/rand failed: " + err.Error())
	}
	return int(v.Int64())
}

// Bool returns a cryptographically secure random boolean.
func Bool() bool {
	return Intn(2) == 1
}
