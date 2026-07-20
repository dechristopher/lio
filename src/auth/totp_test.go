package auth

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/dechristopher/lio/config"
)

// TestTOTPVerify: a code generated from a secret verifies, a wrong one does not.
func TestTOTPVerify(t *testing.T) {
	secret, url, qr, err := EnrollTOTP("drew")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if secret == "" || url == "" {
		t.Fatal("empty enroll fields")
	}
	if len(qr) < 32 || qr[:22] != "data:image/png;base64," {
		t.Fatalf("qr not a png data uri: %.32q", qr)
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("gen code: %v", err)
	}
	if !VerifyTOTP(secret, code) {
		t.Fatal("valid code rejected")
	}
	if VerifyTOTP(secret, "000000") && code != "000000" {
		t.Fatal("wrong code accepted")
	}
}

// TestTOTPEncryptRoundTrip: the stored blob decrypts back to the secret.
func TestTOTPEncryptRoundTrip(t *testing.T) {
	config.CryptoKey = "0123456789abcdef0123456789abcdef" // 32B AES-256 key
	secret := "JBSWY3DPEHPK3PXP"
	enc, err := EncryptTOTPSecret(secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if string(enc) == secret {
		t.Fatal("secret stored in the clear")
	}
	got, err := DecryptTOTPSecret(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != secret {
		t.Fatalf("round-trip mismatch: %q != %q", got, secret)
	}
}

// TestConsumeTOTPReplay: a valid code works once, then is rejected as a replay
// within its window; a different user is unaffected.
func TestConsumeTOTPReplay(t *testing.T) {
	secret, _, _, err := EnrollTOTP("replayer")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("gen code: %v", err)
	}

	const uA, uB = int64(9001), int64(9002)
	if !ConsumeTOTP(uA, secret, code) {
		t.Fatal("first use rejected")
	}
	if ConsumeTOTP(uA, secret, code) {
		t.Fatal("replay accepted for same user")
	}
	// same code is still valid for a different user (no cross-user replay state)
	if !ConsumeTOTP(uB, secret, code) {
		t.Fatal("valid code rejected for a different user")
	}
}
