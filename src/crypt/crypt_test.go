package crypt

import (
	"testing"
)

const (
	message       = "encrypt_me_please_and_thanks"
	encryptFail   = "TestCrypt_Encrypt failed error=%s"
	tamperNoError = "TestCrypt_Tamper: expected error on tampered ciphertext, got none"
	encDecFail    = "TestCrypt_EncDec failed error=%s"
	encDecBadComp = "TestCrypt_EncDec bad comparison got=%s expected=%s"
)

// TestCrypt runs the crypt package test suite
func TestCrypt(t *testing.T) {
	t.Run("Encrypt", testEncrypt)
	t.Run("Tamper", testTamperDetected)
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

// testTamperDetected verifies the AEAD guarantee that makes GCM safe for the
// identity cookie: any modification of the ciphertext (here, flipping a bit in
// the last byte) must make Decrypt fail rather than return altered plaintext.
// The retired CFB scheme would have "decrypted" this without error.
func testTamperDetected(t *testing.T) {
	cipherText, err := Encrypt([]byte(message))
	if err != nil {
		t.Fatalf(encryptFail, err.Error())
	}

	// flip a bit in the first base64 character: it corrupts the prepended
	// nonce (guaranteeing an authentication failure) or fails the decode
	// outright — either way Decrypt must not return altered plaintext
	tampered := make([]byte, len(cipherText))
	copy(tampered, cipherText)
	tampered[0] ^= 0x01

	if _, err := Decrypt(tampered); err == nil {
		t.Fatal(tamperNoError)
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
