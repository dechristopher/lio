package auth

import (
	"encoding/base64"
	"strconv"
	"sync"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/dechristopher/lio/crypt"
)

// TOTP second factor (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4): RFC 6238
// time-based one-time passwords from an authenticator app. The shared secret is
// encrypted at rest with the crypt/ AES-GCM key. Enrollment is
// confirm-before-activate: EnrollTOTP mints a secret shown as a QR + text, the
// caller stores it unconfirmed, and only a live code activates it. Login
// verification adds a ±1-step skew, a per-user replay guard, and a 5/min rate
// limit.

// totpIssuer labels the account inside authenticator apps.
const totpIssuer = "Lioctad"

// totpPeriod / totpSkew / totpDigits pin the verification parameters (defaults
// pquerna also uses, stated explicitly so verify and enroll can't drift).
const (
	totpPeriod = 30
	totpSkew   = 1
)

// EnrollTOTP mints a fresh secret for username and returns the base32 secret
// (for manual entry), the otpauth:// provisioning URL, and a PNG data-URI QR of
// that URL (rendered with skip2/go-qrcode, the same lib the invite QR uses).
// The caller encrypts + stores the secret unconfirmed.
func EnrollTOTP(username string) (secret, otpauthURL, qrDataURI string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: username,
		Period:      totpPeriod,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", "", err
	}
	otpauthURL = key.String()
	png, err := qrcode.Encode(otpauthURL, qrcode.Medium, 256)
	if err != nil {
		return "", "", "", err
	}
	qrDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	return key.Secret(), otpauthURL, qrDataURI, nil
}

// EncryptTOTPSecret seals a base32 secret for storage (crypt.Encrypt output —
// the users.totp_secret_enc blob).
func EncryptTOTPSecret(secret string) ([]byte, error) {
	return crypt.Encrypt([]byte(secret))
}

// DecryptTOTPSecret reverses EncryptTOTPSecret.
func DecryptTOTPSecret(enc []byte) (string, error) {
	b, err := crypt.Decrypt(enc)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyTOTP checks a code against a secret with ±1-step skew. No replay guard
// or rate limit — used at enroll-confirm, which is already behind an
// authenticated, password-re-verified endpoint.
func VerifyTOTP(secret, code string) bool {
	ok, err := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    totpPeriod,
		Skew:      totpSkew,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// --- login-time verification (rate limit + replay guard) --------------------

// totpLimiter caps TOTP attempts at 5/min per user (independent of the login
// password limiter, which keys on IP|username).
var totpLimiter = NewLimiter(5, time.Minute)

// AllowTOTP records a TOTP attempt for a user and reports whether it is within
// the 5/min budget.
func AllowTOTP(userID int64) bool {
	return totpLimiter.Allow(strconv.FormatInt(userID, 10))
}

// totpReplay remembers each user's last accepted code briefly, so the same code
// cannot be replayed inside its validity window (skew widens that window to a
// few steps). Cleared lazily as entries lapse.
var totpReplay = struct {
	sync.Mutex
	m map[int64]replayEntry
}{m: make(map[int64]replayEntry)}

type replayEntry struct {
	code  string
	until time.Time
}

// ConsumeTOTP is the login-path verification: it enforces the rate limit,
// checks the code with skew, and rejects a code already accepted for this user
// within its window (replay). A rejected attempt still counts against the rate
// limit. Returns true only on a fresh, valid code.
func ConsumeTOTP(userID int64, secret, code string) bool {
	if !AllowTOTP(userID) {
		return false
	}

	now := time.Now()
	totpReplay.Lock()
	if e, ok := totpReplay.m[userID]; ok && now.Before(e.until) && e.code == code {
		totpReplay.Unlock()
		return false // replay of a code still inside its window
	}
	totpReplay.Unlock()

	if !VerifyTOTP(secret, code) {
		return false
	}

	// remember it for a full skew-widened window so it can't be reused
	totpReplay.Lock()
	// opportunistic sweep
	for id, e := range totpReplay.m {
		if now.After(e.until) {
			delete(totpReplay.m, id)
		}
	}
	totpReplay.m[userID] = replayEntry{
		code:  code,
		until: now.Add((totpSkew*2 + 1) * totpPeriod * time.Second),
	}
	totpReplay.Unlock()
	return true
}
