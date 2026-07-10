package crypt

import (
	"crypto/rand"
	"encoding/base64"
	"io"
)

// Encrypt seals the given bytes with AES-GCM under the secure key and returns a
// URL-safe base64 string. A fresh random nonce is generated per call and
// prepended to the ciphertext (Seal appends to its dst), so Decrypt can recover
// it; the authentication tag GCM appends lets Decrypt detect any tampering.
func Encrypt(in []byte) ([]byte, error) {
	aead := gcm()

	// random nonce, prepended to the sealed output so it travels with it
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Seal(dst, nonce, plaintext, aad): with dst=nonce the result is
	// nonce || ciphertext || tag
	cipherText := aead.Seal(nonce, nonce, in, nil)

	// return as a URL-safe base64 encoded string
	return []byte(base64.URLEncoding.EncodeToString(cipherText)), nil
}
