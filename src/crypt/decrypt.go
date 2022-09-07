package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"

	"github.com/dechristopher/lio/str"
)

// Decrypt the given input bytes using the secure token
func Decrypt(in []byte) ([]byte, error) {
	cipherText, err := base64.URLEncoding.DecodeString(string(in))
	if err != nil {
		return nil, err
	}

	block := gcm()

	if len(cipherText) < aes.BlockSize {
		err = errors.New(str.ECipherBlockTooSmall)
		return nil, err
	}

	iv := cipherText[:aes.BlockSize]
	cipherText = cipherText[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(cipherText, cipherText)

	return cipherText, nil
}
