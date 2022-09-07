package crypt

import (
	"testing"
)

const (
	message       = "encrypt_me_please_and_thanks"
	encryptFail   = "TestCrypt_Encrypt failed error=%s"
	decryptFail   = "TestCrypt_Decrypt failed error=%s"
	encDecFail    = "TestCrypt_EncDec failed error=%s"
	encDecBadComp = "TestCrypt_EncDec bad comparison got=%s expected=%s"
)

// TestCrypt runs the crypt package test suite
func TestCrypt(t *testing.T) {
	t.Run("Encrypt", testEncrypt)
	t.Run("Decrypt", testDecrypt)
	t.Run("EncDec", testEncryptDecrypt)
}

// testEncrypt tests non-functional requirement of not having a mis-configured
// or missing secure key and having the encryption work without error
func testEncrypt(t *testing.T) {
	_, err := Encrypt([]byte(message))

	if err != nil {
		t.Fatalf(encryptFail, err.Error())
	}
}

// testDecrypt tests non-functional requirement of not having a mis-configured
// or missing secure key and having the decryption work without error
func testDecrypt(t *testing.T) {
	_, err := Decrypt([]byte(message))

	if err != nil {
		t.Fatalf(decryptFail, err.Error())
	}
}

// testEncryptDecrypt will test functional requirement
// of encrypting/decrypting a message
func testEncryptDecrypt(t *testing.T) {
	cipherText, err := Encrypt([]byte(message))

	if err != nil {
		t.Fatalf(encDecFail, err.Error())
	}

	msg, err := Decrypt(cipherText)

	if err != nil {
		t.Fatalf(encDecFail, err.Error())
	}

	if string(msg) != message {
		t.Fatalf(encDecBadComp, msg, message)
	}
}
