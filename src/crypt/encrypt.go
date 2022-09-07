package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
)

// Encrypt the given bytes using the secure token
func Encrypt(in []byte) ([]byte, error) {
	block := gcm()
	cipherText := make([]byte, aes.BlockSize+len(in))
	iv := cipherText[:aes.BlockSize]

	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(cipherText[aes.BlockSize:], in)

	//returns to base64 encoded string
	return []byte(base64.URLEncoding.EncodeToString(cipherText)), nil
}
