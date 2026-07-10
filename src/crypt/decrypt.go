package crypt

import (
	"encoding/base64"
	"errors"

	"github.com/dechristopher/lio/str"
)

// Decrypt reverses Encrypt: it base64-decodes the input, splits off the
// prepended nonce, and opens the AES-GCM ciphertext. Open verifies the
// authentication tag, so tampered or truncated input (including any cookie
// produced by the old unauthenticated CFB scheme) returns an error instead of
// bogus plaintext.
func Decrypt(in []byte) ([]byte, error) {
	cipherText, err := base64.URLEncoding.DecodeString(string(in))
	if err != nil {
		return nil, err
	}

	aead := gcm()

	nonceSize := aead.NonceSize()
	if len(cipherText) < nonceSize {
		return nil, errors.New(str.ECipherBlockTooSmall)
	}

	// split the prepended nonce from the sealed ciphertext+tag
	nonce, sealed := cipherText[:nonceSize], cipherText[nonceSize:]

	plainText, err := aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, err
	}

	return plainText, nil
}
