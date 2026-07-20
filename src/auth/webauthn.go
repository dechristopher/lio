package auth

import (
	"bytes"
	"crypto/rand"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// WebAuthn passkeys as a second factor (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4).
// The relying-party identity is derived from config.SiteURL(): RPID is the host
// (localhost in dev, lioctad.org in prod) and the single permitted origin is the
// full scheme+host. Passkeys work over http://localhost (secure-context
// exception) but not over a LAN IP — noted in the dev docs. Credential storage
// records the user handle, discoverable flag, AAGUID, and transports so
// first-factor (passwordless) passkey login later is handler-only work.
//
// Registration/login challenges live in an in-process 5-minute TTL map keyed by
// the caller (per-session for enrollment, per pending-token for login) — the
// same single-instance in-memory posture as the session cache / rate limiter.

// webAuthnHandleLen is the length of the opaque per-user WebAuthn user handle.
const webAuthnHandleLen = 32

// challengeTTL bounds how long a begun ceremony may be finished.
const challengeTTL = 5 * time.Minute

var (
	webAuthnOnce sync.Once
	webAuthnInst *webauthn.WebAuthn
	webAuthnErr  error
)

// web lazily builds (and caches) the relying-party instance from the current
// site URL. Returns an error if the config is unusable.
func web() (*webauthn.WebAuthn, error) {
	webAuthnOnce.Do(func() {
		rpID, origins := rpConfig()
		webAuthnInst, webAuthnErr = webauthn.New(&webauthn.Config{
			RPDisplayName: totpIssuer,
			RPID:          rpID,
			RPOrigins:     origins,
		})
		if webAuthnErr != nil {
			util.Error(str.CAuth, "webauthn init failed error=%s", webAuthnErr.Error())
		}
	})
	return webAuthnInst, webAuthnErr
}

// rpConfig derives the relying-party ID (host only) and permitted origin
// (scheme+host) from config.SiteURL().
func rpConfig() (rpID string, origins []string) {
	u, err := url.Parse(config.SiteURL())
	if err != nil || u.Host == "" {
		// SiteURL is server-controlled and always well-formed; this is just a
		// belt-and-suspenders fallback.
		return "localhost", []string{"http://localhost"}
	}
	return u.Hostname(), []string{u.Scheme + "://" + u.Host}
}

// --- challenge store --------------------------------------------------------

type challengeEntry struct {
	data    webauthn.SessionData
	expires time.Time
}

var challengeStore = struct {
	sync.Mutex
	m map[string]challengeEntry
}{m: make(map[string]challengeEntry)}

func putChallenge(key string, sd *webauthn.SessionData) {
	now := time.Now()
	challengeStore.Lock()
	for k, e := range challengeStore.m {
		if now.After(e.expires) {
			delete(challengeStore.m, k)
		}
	}
	challengeStore.m[key] = challengeEntry{data: *sd, expires: now.Add(challengeTTL)}
	challengeStore.Unlock()
}

// takeChallenge consumes a stored challenge (single-use).
func takeChallenge(key string) (webauthn.SessionData, bool) {
	challengeStore.Lock()
	defer challengeStore.Unlock()
	e, ok := challengeStore.m[key]
	delete(challengeStore.m, key)
	if !ok || time.Now().After(e.expires) {
		return webauthn.SessionData{}, false
	}
	return e.data, true
}

// --- webauthn.User adapter --------------------------------------------------

type waUser struct {
	id          []byte
	name        string
	displayName string
	creds       []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return u.id }
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// loadWAUser assembles the webauthn.User for an account: its opaque handle
// (minted once on demand) and its stored credentials. Called only in passkey
// flows, so minting a handle here is correct — a passwordless login later would
// look users up BY handle instead.
func loadWAUser(userID int64, username string) (*waUser, error) {
	handle, err := db.GetWebAuthnUserHandle(userID)
	if err != nil {
		return nil, err
	}
	if handle == nil {
		fresh := make([]byte, webAuthnHandleLen)
		if _, err := rand.Read(fresh); err != nil {
			return nil, err
		}
		if err := db.SetWebAuthnUserHandle(userID, fresh); err != nil {
			return nil, err
		}
		// re-read: a concurrent registration may have won the IS NULL guard
		handle, err = db.GetWebAuthnUserHandle(userID)
		if err != nil {
			return nil, err
		}
		if handle == nil {
			return nil, errors.New("auth: webauthn handle mint failed")
		}
	}

	recs, err := db.ListWebAuthnCredentials(userID)
	if err != nil {
		return nil, err
	}
	creds := make([]webauthn.Credential, 0, len(recs))
	for _, r := range recs {
		creds = append(creds, toWACredential(r))
	}
	return &waUser{id: handle, name: username, displayName: username, creds: creds}, nil
}

// --- registration -----------------------------------------------------------

// BeginWebAuthnRegistration builds creation options for a new passkey and
// stashes the challenge under key. The client feeds the returned options to
// navigator.credentials.create.
func BeginWebAuthnRegistration(key string, userID int64, username string) (*protocol.CredentialCreation, error) {
	w, err := web()
	if err != nil {
		return nil, err
	}
	u, err := loadWAUser(userID, username)
	if err != nil {
		return nil, err
	}

	exclude := make([]protocol.CredentialDescriptor, 0, len(u.creds))
	for _, c := range u.creds {
		exclude = append(exclude, protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: c.ID,
			Transport:    c.Transport,
		})
	}

	options, sd, err := w.BeginRegistration(u,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		}),
		webauthn.WithExclusions(exclude),
		webauthn.WithExtensions(protocol.AuthenticationExtensions{"credProps": true}),
	)
	if err != nil {
		return nil, err
	}
	putChallenge(key, sd)
	return options, nil
}

// FinishWebAuthnRegistration validates the attestation against the stashed
// challenge and returns the credential record to persist (nickname applied).
func FinishWebAuthnRegistration(key string, userID int64, username, nickname string, body []byte) (db.WebAuthnCredentialRecord, error) {
	var zero db.WebAuthnCredentialRecord
	w, err := web()
	if err != nil {
		return zero, err
	}
	sd, ok := takeChallenge(key)
	if !ok {
		return zero, errors.New("auth: no active registration challenge")
	}
	u, err := loadWAUser(userID, username)
	if err != nil {
		return zero, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(body))
	if err != nil {
		return zero, err
	}
	cred, err := w.CreateCredential(u, sd, parsed)
	if err != nil {
		return zero, err
	}
	return fromWACredential(cred, nickname, credPropsRK(parsed.ClientExtensionResults)), nil
}

// --- login ------------------------------------------------------------------

// BeginWebAuthnLogin builds assertion options for a user's passkeys and stashes
// the challenge under key. Errors if the user has no passkeys.
func BeginWebAuthnLogin(key string, userID int64, username string) (*protocol.CredentialAssertion, error) {
	w, err := web()
	if err != nil {
		return nil, err
	}
	u, err := loadWAUser(userID, username)
	if err != nil {
		return nil, err
	}
	if len(u.creds) == 0 {
		return nil, errors.New("auth: no passkeys registered")
	}
	options, sd, err := w.BeginLogin(u)
	if err != nil {
		return nil, err
	}
	putChallenge(key, sd)
	return options, nil
}

// FinishWebAuthnLogin validates an assertion against the stashed challenge,
// returning the used credential id and its updated signature counter (the
// caller persists the counter). A clone warning is logged but not fatal —
// counter-less authenticators legitimately report zero.
func FinishWebAuthnLogin(key string, userID int64, username string, body []byte) (credID []byte, newSignCount uint32, err error) {
	w, err := web()
	if err != nil {
		return nil, 0, err
	}
	sd, ok := takeChallenge(key)
	if !ok {
		return nil, 0, errors.New("auth: no active login challenge")
	}
	u, err := loadWAUser(userID, username)
	if err != nil {
		return nil, 0, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	cred, err := w.ValidateLogin(u, sd, parsed)
	if err != nil {
		return nil, 0, err
	}
	if cred.Authenticator.CloneWarning {
		util.Error(str.CAuth, "webauthn clone warning userID=%d", userID)
	}
	return cred.ID, cred.Authenticator.SignCount, nil
}

// --- conversions ------------------------------------------------------------

func toWACredential(r db.WebAuthnCredentialRecord) webauthn.Credential {
	return webauthn.Credential{
		ID:              r.CredentialID,
		PublicKey:       r.PublicKey,
		AttestationType: r.AttestationType,
		Transport:       parseTransports(r.Transports),
		Flags: webauthn.CredentialFlags{
			BackupEligible: r.BackupEligible,
			BackupState:    r.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    r.AAGUID,
			SignCount: r.SignCount,
		},
	}
}

func fromWACredential(c *webauthn.Credential, nickname string, discoverable bool) db.WebAuthnCredentialRecord {
	aaguid := c.Authenticator.AAGUID
	if aaguid == nil {
		aaguid = []byte{} // the column is NOT NULL
	}
	return db.WebAuthnCredentialRecord{
		CredentialID:    c.ID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		AAGUID:          aaguid,
		SignCount:       c.Authenticator.SignCount,
		Transports:      joinTransports(c.Transport),
		BackupEligible:  c.Flags.BackupEligible,
		BackupState:     c.Flags.BackupState,
		Discoverable:    discoverable,
		Nickname:        nickname,
	}
}

// parseTransports splits the stored comma-joined transport hints.
func parseTransports(csv string) []protocol.AuthenticatorTransport {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]protocol.AuthenticatorTransport, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, protocol.AuthenticatorTransport(p))
		}
	}
	return out
}

// joinTransports serializes transport hints for storage.
func joinTransports(ts []protocol.AuthenticatorTransport) string {
	if len(ts) == 0 {
		return ""
	}
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = string(t)
	}
	return strings.Join(parts, ",")
}

// credPropsRK reads the credProps.rk (resident key / discoverable) extension
// output, defaulting to false when absent.
func credPropsRK(ext protocol.AuthenticationExtensionsClientOutputs) bool {
	if ext == nil {
		return false
	}
	cp, ok := ext["credProps"].(map[string]any)
	if !ok {
		return false
	}
	rk, _ := cp["rk"].(bool)
	return rk
}
