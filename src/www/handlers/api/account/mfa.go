package account

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
)

// MFA endpoints (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4): the login-time second
// factor (TOTP / recovery code / passkey, gated by a short-lived pending token)
// and the logged-in management surface (enroll/disable each factor, all gated
// by a password re-verify). The Security section of the profile popover renders
// client-side from GET /mfa/status. Recovery codes are issued once when the
// first factor activates and only ever shown at generation time.

// wireMFA attaches the MFA routes to the /api/auth group.
func wireMFA(g fiber.Router) {
	// login-time second factor (pending-token gated, not yet logged in)
	g.Post("/login/totp", LoginTOTPHandler)
	g.Post("/login/recovery", LoginRecoveryHandler)
	g.Post("/login/webauthn/begin", LoginWebAuthnBeginHandler)
	g.Post("/login/webauthn/finish", LoginWebAuthnFinishHandler)

	// logged-in management (password re-verify gated)
	g.Get("/mfa/status", MFAStatusHandler)
	g.Post("/totp/begin", TOTPBeginHandler)
	g.Post("/totp/confirm", TOTPConfirmHandler)
	g.Post("/totp/disable", TOTPDisableHandler)
	g.Post("/recovery/regenerate", RecoveryRegenerateHandler)
	g.Post("/webauthn/register/begin", WebAuthnRegisterBeginHandler)
	g.Post("/webauthn/register/finish", WebAuthnRegisterFinishHandler)
	g.Post("/webauthn/credentials/rename", WebAuthnRenameHandler)
	g.Post("/webauthn/credentials/delete", WebAuthnDeleteHandler)
}

// --- response bodies --------------------------------------------------------

// mfaMethodsBody tells the login form which second factors an account offers.
type mfaMethodsBody struct {
	TOTP     bool `json:"totp"`
	Passkey  bool `json:"passkey"`
	Recovery bool `json:"recovery"`
}

// mfaChallengeBody replaces the normal login success body when a second factor
// is required: the client swaps to the MFA step keyed by the pending token.
type mfaChallengeBody struct {
	MFA     bool           `json:"mfa"`
	Pending string         `json:"pending"`
	Methods mfaMethodsBody `json:"methods"`
}

// recoveryCodesBody carries a freshly generated set of recovery codes (shown
// once). Codes is nil/empty when nothing new was generated.
type recoveryCodesBody struct {
	RecoveryCodes []string `json:"recoveryCodes"`
}

// totpEnrollBody is the TOTP enroll payload: the provisioning URL, a QR of it,
// and the base32 secret for manual entry.
type totpEnrollBody struct {
	Secret  string `json:"secret"`
	Otpauth string `json:"otpauth"`
	QR      string `json:"qr"`
}

// passkeyView is one registered passkey in the Security section.
type passkeyView struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	AddedAt  string `json:"addedAt"`
	LastUsed string `json:"lastUsed"`
}

// securityStatusBody is the whole Security section state.
type securityStatusBody struct {
	TOTP              bool          `json:"totp"`
	Passkeys          []passkeyView `json:"passkeys"`
	RecoveryRemaining int           `json:"recoveryRemaining"`
}

// --- login-time second factor -----------------------------------------------

// LoginTOTPHandler completes a pending login with a TOTP code.
func LoginTOTPHandler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return unavailable(c)
	}
	var req struct {
		Pending string `json:"pending"`
		Code    string `json:"code"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	p, ok := auth.ResolvePending(req.Pending)
	if !ok {
		return expiredLogin(c)
	}
	enc, confirmed, err := db.GetTOTP(p.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "login failed"})
	}
	if !confirmed || enc == nil {
		return badCode(c)
	}
	secret, err := auth.DecryptTOTPSecret(enc)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "login failed"})
	}
	if !auth.ConsumeTOTP(p.UserID, secret, strings.TrimSpace(req.Code)) {
		return badCode(c)
	}
	return finishPendingLogin(c, req.Pending, p)
}

// LoginRecoveryHandler completes a pending login with a single-use recovery
// code.
func LoginRecoveryHandler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return unavailable(c)
	}
	var req struct {
		Pending string `json:"pending"`
		Code    string `json:"code"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	p, ok := auth.ResolvePending(req.Pending)
	if !ok {
		return expiredLogin(c)
	}
	used, err := db.UseRecoveryCode(p.UserID, auth.HashRecoveryCode(req.Code))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "login failed"})
	}
	if !used {
		return badCode(c)
	}
	return finishPendingLogin(c, req.Pending, p)
}

// LoginWebAuthnBeginHandler starts a pending login's passkey assertion.
func LoginWebAuthnBeginHandler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return unavailable(c)
	}
	pending := c.Query("pending")
	p, ok := auth.ResolvePending(pending)
	if !ok {
		return expiredLogin(c)
	}
	options, err := auth.BeginWebAuthnLogin(pending, p.UserID, p.Username)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "could not start passkey login"})
	}
	return c.JSON(options)
}

// LoginWebAuthnFinishHandler validates a pending login's passkey assertion.
func LoginWebAuthnFinishHandler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return unavailable(c)
	}
	pending := c.Query("pending")
	p, ok := auth.ResolvePending(pending)
	if !ok {
		return expiredLogin(c)
	}
	credID, signCount, err := auth.FinishWebAuthnLogin(pending, p.UserID, p.Username, c.Body())
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(errBody{Error: "passkey verification failed"})
	}
	// clone-detection state; best-effort, never blocks a valid login
	_ = db.UpdateWebAuthnSignCount(p.UserID, credID, signCount)
	return finishPendingLogin(c, pending, p)
}

// --- management: TOTP -------------------------------------------------------

// TOTPBeginHandler starts TOTP enrollment: verify the password, mint a secret,
// store it unconfirmed, and return the QR/secret for the authenticator app.
func TOTPBeginHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	if !checkPassword(*sess.UserID, req.Password) {
		return wrongPassword(c)
	}
	secret, otpauth, qr, err := auth.EnrollTOTP(sess.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not start setup"})
	}
	enc, err := auth.EncryptTOTPSecret(secret)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not start setup"})
	}
	if err := db.SetTOTPSecret(*sess.UserID, enc); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not start setup"})
	}
	return c.JSON(totpEnrollBody{Secret: secret, Otpauth: otpauth, QR: qr})
}

// TOTPConfirmHandler activates a pending TOTP secret after a live code, and
// issues recovery codes if this is the account's first second factor.
func TOTPConfirmHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	enc, _, err := db.GetTOTP(*sess.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not confirm"})
	}
	if enc == nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "start setup first"})
	}
	secret, err := auth.DecryptTOTPSecret(enc)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not confirm"})
	}
	if !auth.VerifyTOTP(secret, strings.TrimSpace(req.Code)) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errBody{Error: "that code didn't match — try the current one"})
	}
	if err := db.ConfirmTOTP(*sess.UserID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not confirm"})
	}
	return c.JSON(recoveryCodesBody{RecoveryCodes: maybeIssueRecoveryCodes(*sess.UserID)})
}

// TOTPDisableHandler turns off TOTP (password re-verify), clearing recovery
// codes if no factor remains.
func TOTPDisableHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	if !checkPassword(*sess.UserID, req.Password) {
		return wrongPassword(c)
	}
	if err := db.ClearTOTP(*sess.UserID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not disable"})
	}
	cleanupRecoveryIfNoMFA(*sess.UserID)
	return c.SendStatus(fiber.StatusNoContent)
}

// --- management: recovery codes ---------------------------------------------

// RecoveryRegenerateHandler replaces the recovery-code set (password re-verify).
func RecoveryRegenerateHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	if !checkPassword(*sess.UserID, req.Password) {
		return wrongPassword(c)
	}
	if !mfaEnabled(*sess.UserID) {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "enable a second factor first"})
	}
	plain, hashes := auth.GenerateRecoveryCodes()
	if err := db.ReplaceRecoveryCodes(*sess.UserID, hashes); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not regenerate"})
	}
	return c.JSON(recoveryCodesBody{RecoveryCodes: plain})
}

// --- management: passkeys ---------------------------------------------------

// WebAuthnRegisterBeginHandler starts passkey enrollment (password re-verify).
func WebAuthnRegisterBeginHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	if !checkPassword(*sess.UserID, req.Password) {
		return wrongPassword(c)
	}
	options, err := auth.BeginWebAuthnRegistration(regKey(sess), *sess.UserID, sess.Username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not start passkey setup"})
	}
	return c.JSON(options)
}

// WebAuthnRegisterFinishHandler stores a newly registered passkey and issues
// recovery codes if this is the account's first second factor.
func WebAuthnRegisterFinishHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	rec, err := auth.FinishWebAuthnRegistration(
		regKey(sess), *sess.UserID, sess.Username, sanitizeNickname(c.Query("nickname")), c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "passkey registration failed"})
	}
	if err := db.InsertWebAuthnCredential(*sess.UserID, rec); err != nil {
		return c.Status(fiber.StatusConflict).JSON(errBody{Error: "that passkey is already registered"})
	}
	return c.JSON(recoveryCodesBody{RecoveryCodes: maybeIssueRecoveryCodes(*sess.UserID)})
}

// WebAuthnRenameHandler renames a passkey (owner-scoped).
func WebAuthnRenameHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		ID       int64  `json:"id"`
		Nickname string `json:"nickname"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	if err := db.RenameWebAuthnCredential(req.ID, *sess.UserID, sanitizeNickname(req.Nickname)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "rename failed"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// WebAuthnDeleteHandler removes a passkey (owner-scoped), clearing recovery
// codes if no factor remains.
func WebAuthnDeleteHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	if err := db.DeleteWebAuthnCredential(req.ID, *sess.UserID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "delete failed"})
	}
	cleanupRecoveryIfNoMFA(*sess.UserID)
	return c.SendStatus(fiber.StatusNoContent)
}

// --- status -----------------------------------------------------------------

// MFAStatusHandler returns the Security section state the profile popover
// renders (TOTP on/off, registered passkeys, recovery codes remaining).
func MFAStatusHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	_, totp, err := db.GetTOTP(*sess.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not load security"})
	}
	creds, err := db.ListWebAuthnCredentials(*sess.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not load security"})
	}
	remaining, err := db.CountUnusedRecoveryCodes(*sess.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not load security"})
	}
	views := make([]passkeyView, 0, len(creds))
	for _, cr := range creds {
		name := cr.Nickname
		if name == "" {
			name = "Passkey"
		}
		v := passkeyView{
			ID:       cr.ID,
			Nickname: name,
			AddedAt:  relativeTime(cr.CreatedAt),
		}
		if cr.LastUsedAt != nil {
			v.LastUsed = relativeTime(*cr.LastUsedAt)
		}
		views = append(views, v)
	}
	return c.JSON(securityStatusBody{
		TOTP:              totp,
		Passkeys:          views,
		RecoveryRemaining: remaining,
	})
}

// --- shared helpers ---------------------------------------------------------

// finishPendingLogin performs the real session upgrade after a second factor
// succeeds, consuming the pending token.
func finishPendingLogin(c fiber.Ctx, pending string, p auth.Pending) error {
	if err := auth.Login(c, auth.FromRequest(c), p.UserID, p.Username, p.Title); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "login failed"})
	}
	auth.ConsumePending(pending)
	return c.Status(fiber.StatusOK).JSON(okBody{Username: p.Username})
}

// checkPassword verifies a user's current password (the re-verify gate on every
// management action).
func checkPassword(userID int64, password string) bool {
	u, found, err := db.GetUserByID(userID)
	if err != nil || !found {
		return false
	}
	ok, _, err := auth.VerifyPassword(u.PasswordHash, password)
	return err == nil && ok
}

// mfaEnabled reports whether an account has any active second factor.
func mfaEnabled(userID int64) bool {
	_, totp, err := db.GetTOTP(userID)
	if err == nil && totp {
		return true
	}
	n, err := db.CountWebAuthnCredentials(userID)
	return err == nil && n > 0
}

// cleanupRecoveryIfNoMFA drops recovery codes once the last factor is gone (they
// would otherwise be a standalone login bypass).
func cleanupRecoveryIfNoMFA(userID int64) {
	if !mfaEnabled(userID) {
		_ = db.DeleteRecoveryCodes(userID)
	}
}

// maybeIssueRecoveryCodes generates a fresh set only when the account has none
// (first-factor activation). Returns the plaintext to show once, or nil when
// the user already has codes (kept until explicitly regenerated).
func maybeIssueRecoveryCodes(userID int64) []string {
	if n, err := db.CountUnusedRecoveryCodes(userID); err != nil || n > 0 {
		return nil
	}
	plain, hashes := auth.GenerateRecoveryCodes()
	if err := db.ReplaceRecoveryCodes(userID, hashes); err != nil {
		return nil
	}
	return plain
}

// regKey scopes a passkey-registration challenge to the current session.
func regKey(sess *auth.Session) string {
	return "reg:" + strconv.FormatInt(sess.ID, 10)
}

// sanitizeNickname trims and bounds a user-supplied passkey label.
func sanitizeNickname(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 40 {
		s = string(r[:40])
	}
	return s
}

// --- small response helpers -------------------------------------------------

func expiredLogin(c fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(errBody{Error: "login timed out — start over"})
}

func badCode(c fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(errBody{Error: "that code didn't work"})
}

func wrongPassword(c fiber.Ctx) error {
	return c.Status(fiber.StatusForbidden).JSON(errBody{Error: "current password is incorrect"})
}
