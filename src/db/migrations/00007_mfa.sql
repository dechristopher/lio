-- +goose Up

-- Multi-factor auth (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4): TOTP + single-use
-- recovery codes + WebAuthn passkeys, all as *second factors* over the existing
-- password first factor. All additions are nullable / additive, so existing
-- accounts keep working unchanged (no MFA = password-only login).

-- users gains the TOTP + WebAuthn-identity columns.
--   totp_secret_enc     : the shared TOTP secret, encrypted at rest with the
--                         crypt/ AES-GCM key (crypt.Encrypt output; NULL = none).
--                         Written unconfirmed at enroll-begin; confirm activates.
--   totp_confirmed_at   : NULL until the user proves a live code (confirm-before-
--                         activate). Only a confirmed secret gates login / counts
--                         as an enabled factor.
--   webauthn_user_handle: the stable, opaque per-user WebAuthn user id (random,
--                         not the account id — privacy). Minted on first passkey
--                         registration, reused thereafter; NULL until then.
ALTER TABLE users
    ADD COLUMN totp_secret_enc      BYTEA,
    ADD COLUMN totp_confirmed_at    TIMESTAMPTZ,
    ADD COLUMN webauthn_user_handle BYTEA;

-- recovery_codes: single-use backup codes for when a second factor is
-- unavailable. Only the SHA-256 of each code is stored — the 80-bit random
-- codes are high-entropy enough that a fast hash is sufficient (unlike
-- passwords). Regenerating replaces the whole set (delete-then-insert);
-- disabling the last factor clears them. used_at NULL = still usable.
CREATE TABLE recovery_codes (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  BYTEA       NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, code_hash)
);
CREATE INDEX recovery_codes_user_id_idx ON recovery_codes (user_id);

-- webauthn_credentials: one row per registered passkey. Stores exactly what
-- go-webauthn needs to reconstruct a webauthn.Credential for assertion
-- (id/public key/aaguid/sign count/transports/backup flags/attestation) plus a
-- user-facing nickname and last-used stamp. credential_id is globally unique
-- (an authenticator's key id). discoverable records whether the credential is a
-- resident key (credProps.rk) — passwordless-ready metadata for later
-- first-factor passkey login (handler-only work then).
CREATE TABLE webauthn_credentials (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    credential_id    BYTEA       NOT NULL UNIQUE,
    public_key       BYTEA       NOT NULL,
    attestation_type TEXT        NOT NULL DEFAULT '',
    aaguid           BYTEA       NOT NULL DEFAULT '\x'::bytea,
    sign_count       BIGINT      NOT NULL DEFAULT 0,
    transports       TEXT        NOT NULL DEFAULT '',
    backup_eligible  BOOLEAN     NOT NULL DEFAULT false,
    backup_state     BOOLEAN     NOT NULL DEFAULT false,
    discoverable     BOOLEAN     NOT NULL DEFAULT false,
    nickname         TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at     TIMESTAMPTZ
);
CREATE INDEX webauthn_credentials_user_id_idx ON webauthn_credentials (user_id);

-- +goose Down
DROP TABLE IF EXISTS webauthn_credentials;
DROP TABLE IF EXISTS recovery_codes;
ALTER TABLE users
    DROP COLUMN IF EXISTS webauthn_user_handle,
    DROP COLUMN IF EXISTS totp_confirmed_at,
    DROP COLUMN IF EXISTS totp_secret_enc;
