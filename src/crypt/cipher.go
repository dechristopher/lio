package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"os"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// gcm initializes an authenticated AES-GCM cipher with the configured secure
// key. GCM is an AEAD: unlike the raw CFB stream this replaced, it authenticates
// the ciphertext, so a tampered enclave cookie fails to Open instead of silently
// decrypting to attacker-influenced bytes (which the identity middleware would
// have trusted). A missing/invalid key is unrecoverable, so we exit hard.
func gcm() cipher.AEAD {
	key := []byte(config.CryptoKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		util.Error(str.CCrypt, str.EBadSecureKey, err.Error())
		os.Exit(1)
		return nil
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		util.Error(str.CCrypt, str.EBadSecureKey, err.Error())
		os.Exit(1)
		return nil
	}

	return aead
}
