package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"os"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// gcm Initializes a new GCM block cipher with the given secure key
func gcm() cipher.Block {
	key := []byte(config.CryptoKey)
	if c, err := aes.NewCipher(key); err != nil {
		util.Error(str.CCrypt, str.EBadSecureKey, err.Error())
		os.Exit(1)
		return nil
	} else {
		return c
	}
}
