package db

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dechristopher/lio/db/gen"
)

// MFA data plane (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4): TOTP secret storage,
// single-use recovery codes, and WebAuthn passkey credentials. Like the other
// accounts accessors these run only against a live pool — the auth package
// gates Enabled() first and MFA is unavailable in the PG-less fallback.

// --- TOTP -------------------------------------------------------------------

// GetTOTP returns the user's encrypted TOTP secret and whether it has been
// confirmed (activated). enc is nil when no secret is enrolled.
func GetTOTP(userID int64) (enc []byte, confirmed bool, err error) {
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).GetTOTP(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return row.TotpSecretEnc, row.TotpConfirmedAt.Valid, nil
}

// SetTOTPSecret stores an (encrypted) secret unconfirmed — confirm-before-
// activate. Clears any prior confirmation.
func SetTOTPSecret(userID int64, enc []byte) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).SetTOTPSecret(ctx, gen.SetTOTPSecretParams{
		ID:            userID,
		TotpSecretEnc: enc,
	})
}

// ConfirmTOTP activates the stored secret after a live code verified.
func ConfirmTOTP(userID int64) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).ConfirmTOTP(ctx, userID)
}

// ClearTOTP disables TOTP (drops the secret + confirmation).
func ClearTOTP(userID int64) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).ClearTOTP(ctx, userID)
}

// --- WebAuthn user handle ---------------------------------------------------

// GetWebAuthnUserHandle returns the user's opaque WebAuthn user id (nil until a
// first passkey registration mints one).
func GetWebAuthnUserHandle(userID int64) ([]byte, error) {
	ctx, cancel := Ctx()
	defer cancel()
	h, err := gen.New(Pool).GetWebAuthnUserHandle(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return h, err
}

// SetWebAuthnUserHandle mints the user's WebAuthn handle once (no-op if already
// set — the query is guarded IS NULL).
func SetWebAuthnUserHandle(userID int64, handle []byte) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).SetWebAuthnUserHandle(ctx, gen.SetWebAuthnUserHandleParams{
		ID:                 userID,
		WebauthnUserHandle: handle,
	})
}

// --- recovery codes ---------------------------------------------------------

// ReplaceRecoveryCodes atomically swaps a user's recovery-code set for a fresh
// batch of hashes (regenerate invalidates the old set). An empty batch just
// clears them.
func ReplaceRecoveryCodes(userID int64, hashes [][]byte) error {
	ctx, cancel := Ctx()
	defer cancel()
	tx, err := Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	if err := q.DeleteRecoveryCodes(ctx, userID); err != nil {
		return err
	}
	for _, h := range hashes {
		if err := q.InsertRecoveryCode(ctx, gen.InsertRecoveryCodeParams{
			UserID:   userID,
			CodeHash: h,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// DeleteRecoveryCodes clears a user's recovery codes (last-factor-disable
// cleanup).
func DeleteRecoveryCodes(userID int64) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).DeleteRecoveryCodes(ctx, userID)
}

// UseRecoveryCode consumes one unused code by hash, reporting whether it hit.
// false = the code was wrong or already spent (single-use).
func UseRecoveryCode(userID int64, hash []byte) (bool, error) {
	ctx, cancel := Ctx()
	defer cancel()
	_, err := gen.New(Pool).UseRecoveryCode(ctx, gen.UseRecoveryCodeParams{
		UserID:   userID,
		CodeHash: hash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CountUnusedRecoveryCodes reports how many codes are still usable.
func CountUnusedRecoveryCodes(userID int64) (int, error) {
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).CountUnusedRecoveryCodes(ctx, userID)
	return int(n), err
}

// --- WebAuthn credentials ---------------------------------------------------

// WebAuthnCredentialRecord is a stored passkey, decoupled from gen so the auth
// package (which owns the go-webauthn conversion) never imports db/gen.
type WebAuthnCredentialRecord struct {
	ID              int64
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       uint32
	Transports      string // comma-joined transport hints
	BackupEligible  bool
	BackupState     bool
	Discoverable    bool
	Nickname        string
	CreatedAt       time.Time
	LastUsedAt      *time.Time
}

// InsertWebAuthnCredential stores a newly registered passkey.
func InsertWebAuthnCredential(userID int64, rec WebAuthnCredentialRecord) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).InsertWebAuthnCredential(ctx, gen.InsertWebAuthnCredentialParams{
		UserID:          userID,
		CredentialID:    rec.CredentialID,
		PublicKey:       rec.PublicKey,
		AttestationType: rec.AttestationType,
		Aaguid:          rec.AAGUID,
		SignCount:       int64(rec.SignCount),
		Transports:      rec.Transports,
		BackupEligible:  rec.BackupEligible,
		BackupState:     rec.BackupState,
		Discoverable:    rec.Discoverable,
		Nickname:        rec.Nickname,
	})
}

// ListWebAuthnCredentials returns a user's passkeys (oldest first).
func ListWebAuthnCredentials(userID int64) ([]WebAuthnCredentialRecord, error) {
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListWebAuthnCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]WebAuthnCredentialRecord, 0, len(rows))
	for _, r := range rows {
		rec := WebAuthnCredentialRecord{
			ID:              r.ID,
			CredentialID:    r.CredentialID,
			PublicKey:       r.PublicKey,
			AttestationType: r.AttestationType,
			AAGUID:          r.Aaguid,
			SignCount:       uint32(r.SignCount),
			Transports:      r.Transports,
			BackupEligible:  r.BackupEligible,
			BackupState:     r.BackupState,
			Discoverable:    r.Discoverable,
			Nickname:        r.Nickname,
			CreatedAt:       r.CreatedAt.Time,
		}
		if r.LastUsedAt.Valid {
			t := r.LastUsedAt.Time
			rec.LastUsedAt = &t
		}
		out = append(out, rec)
	}
	return out, nil
}

// CountWebAuthnCredentials reports how many passkeys a user has registered.
func CountWebAuthnCredentials(userID int64) (int, error) {
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).CountWebAuthnCredentials(ctx, userID)
	return int(n), err
}

// UpdateWebAuthnSignCount persists the post-assertion signature counter and
// last-used stamp (clone-detection state).
func UpdateWebAuthnSignCount(userID int64, credID []byte, count uint32) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).UpdateWebAuthnSignCount(ctx, gen.UpdateWebAuthnSignCountParams{
		UserID:       userID,
		CredentialID: credID,
		SignCount:    int64(count),
	})
}

// RenameWebAuthnCredential sets a passkey's nickname (owner-scoped).
func RenameWebAuthnCredential(id, userID int64, nickname string) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).RenameWebAuthnCredential(ctx, gen.RenameWebAuthnCredentialParams{
		ID:       id,
		UserID:   userID,
		Nickname: nickname,
	})
}

// DeleteWebAuthnCredential removes a passkey (owner-scoped).
func DeleteWebAuthnCredential(id, userID int64) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).DeleteWebAuthnCredential(ctx, gen.DeleteWebAuthnCredentialParams{
		ID:     id,
		UserID: userID,
	})
}
