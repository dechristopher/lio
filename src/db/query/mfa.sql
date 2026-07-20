-- MFA data access (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4): TOTP secret on the
-- users row, single-use recovery codes, and WebAuthn passkey credentials. The
-- auth package guards Enabled()/Ready() before any of these, so they always run
-- against a live pool (like the other accounts queries).

-- --- TOTP -------------------------------------------------------------------

-- name: GetTOTP :one
-- Enrolled TOTP state for a user. Secret is the encrypted blob; confirmed_at is
-- NULL until a live code activated it (confirm-before-activate).
SELECT totp_secret_enc, totp_confirmed_at FROM users WHERE id = $1;

-- name: SetTOTPSecret :exec
-- Enroll-begin: store the (encrypted) secret unconfirmed, clearing any prior
-- confirmation so a re-enroll must be re-proven.
UPDATE users SET totp_secret_enc = $2, totp_confirmed_at = NULL WHERE id = $1;

-- name: ConfirmTOTP :exec
-- Activate the stored secret after a live code verified.
UPDATE users SET totp_confirmed_at = now() WHERE id = $1;

-- name: ClearTOTP :exec
-- Disable TOTP: drop the secret and its confirmation.
UPDATE users SET totp_secret_enc = NULL, totp_confirmed_at = NULL WHERE id = $1;

-- --- WebAuthn user handle ---------------------------------------------------

-- name: GetWebAuthnUserHandle :one
SELECT webauthn_user_handle FROM users WHERE id = $1;

-- name: SetWebAuthnUserHandle :exec
-- Mint-once on first passkey registration (guarded IS NULL so a concurrent
-- second registration can't rotate the handle out from under existing keys).
UPDATE users SET webauthn_user_handle = $2
WHERE id = $1 AND webauthn_user_handle IS NULL;

-- --- recovery codes ---------------------------------------------------------

-- name: InsertRecoveryCode :exec
INSERT INTO recovery_codes (user_id, code_hash) VALUES ($1, $2);

-- name: DeleteRecoveryCodes :exec
-- Regenerate (invalidate the whole set) and last-factor-disable cleanup.
DELETE FROM recovery_codes WHERE user_id = $1;

-- name: UseRecoveryCode :one
-- Consume a code atomically: marks the matching unused code used and returns
-- its id. No row (pgx.ErrNoRows) means the code was wrong or already spent.
UPDATE recovery_codes SET used_at = now()
WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL
RETURNING id;

-- name: CountUnusedRecoveryCodes :one
SELECT count(*) FROM recovery_codes WHERE user_id = $1 AND used_at IS NULL;

-- --- WebAuthn credentials ---------------------------------------------------

-- name: InsertWebAuthnCredential :exec
INSERT INTO webauthn_credentials (
    user_id, credential_id, public_key, attestation_type, aaguid,
    sign_count, transports, backup_eligible, backup_state, discoverable, nickname
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: ListWebAuthnCredentials :many
SELECT * FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at;

-- name: CountWebAuthnCredentials :one
SELECT count(*) FROM webauthn_credentials WHERE user_id = $1;

-- name: UpdateWebAuthnSignCount :exec
-- Post-assertion: persist the incremented signature counter + last-used stamp
-- (clone-detection state). Scoped to the owning user.
UPDATE webauthn_credentials
SET sign_count = $3, last_used_at = now()
WHERE user_id = $1 AND credential_id = $2;

-- name: RenameWebAuthnCredential :exec
UPDATE webauthn_credentials SET nickname = $3 WHERE id = $2 AND user_id = $1;

-- name: DeleteWebAuthnCredential :exec
DELETE FROM webauthn_credentials WHERE id = $2 AND user_id = $1;
