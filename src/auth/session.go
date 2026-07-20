package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
)

// The unified session system (arch/ACCOUNTS_AUTH_RATINGS.md): every visitor,
// anonymous or authenticated, carries exactly one sid cookie holding an opaque
// 256-bit token whose SHA-256 is the server-side lookup key. There is no other
// identity cookie — this replaced the encrypted lio/uid pair. Logging in
// upgrades the anonymous session row in place: token rotated (fixation
// defense), account attached, uid preserved so a live game seat survives a
// mid-match login.

const (
	// SessionCookie is the single identity cookie.
	SessionCookie = "sid"

	// cookieMaxAge is the client-side cookie lifetime. Longer than either
	// server-side TTL on purpose: the server row is authoritative, and a
	// cookie referencing an expired/deleted row simply resolves to nothing
	// and gets replaced by a fresh mint.
	cookieMaxAge = 60 * 24 * time.Hour

	// anonTTL / authedTTL are the sliding server-side expiries.
	anonTTL   = 7 * 24 * time.Hour
	authedTTL = 30 * 24 * time.Hour

	// touchInterval throttles last_seen/expiry refreshes: the resolver only
	// touches a row when its last_seen is at least this stale.
	touchInterval = 5 * time.Minute

	// cacheTTL bounds how long a resolved session may be served from the
	// in-process cache — and therefore how stale a revoked session can look.
	cacheTTL = 30 * time.Second
)

// Session is a resolved identity: the per-session uid (seat/socket identity,
// same 16-char base58 shape the old cookie identity used) plus the attached
// account, if any.
type Session struct {
	ID        int64
	UID       string
	UserID    *int64
	Username  string
	tokenHash [32]byte
	lastSeen  time.Time
	expiresAt time.Time
}

// LoggedIn reports whether the session has an account attached.
func (s *Session) LoggedIn() bool {
	return s != nil && s.UserID != nil
}

// ttl returns the session's sliding server-side TTL.
func (s *Session) ttl() time.Duration {
	if s.LoggedIn() {
		return authedTTL
	}
	return anonTTL
}

// NewToken mints an opaque session token: 32 bytes of crypto/rand, base64url
// on the wire, stored server-side only as its SHA-256.
func NewToken() (token string, hash [32]byte) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		// crypto/rand failure is unrecoverable
		panic("auth: crypto/rand failed: " + err.Error())
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, sha256.Sum256(raw)
}

// hashToken recomputes the storage hash for a presented cookie token.
// Returns ok=false for tokens that aren't even well-formed base64url.
func hashToken(token string) (hash [32]byte, ok bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return hash, false
	}
	return sha256.Sum256(raw), true
}

// --- resolution cache ------------------------------------------------------

// sessionCache absorbs the per-request DB lookup; entries live cacheTTL at
// most, bounding revocation latency. Invalidated explicitly on rotate/logout.
var sessionCache = struct {
	sync.Mutex
	m map[[32]byte]cacheEntry
}{m: make(map[[32]byte]cacheEntry)}

type cacheEntry struct {
	sess    Session
	fetched time.Time
}

func cacheGet(hash [32]byte) (Session, bool) {
	sessionCache.Lock()
	defer sessionCache.Unlock()
	e, ok := sessionCache.m[hash]
	if !ok || time.Since(e.fetched) > cacheTTL {
		delete(sessionCache.m, hash)
		return Session{}, false
	}
	return e.sess, true
}

func cachePut(s Session) {
	sessionCache.Lock()
	defer sessionCache.Unlock()
	sessionCache.m[s.tokenHash] = cacheEntry{sess: s, fetched: time.Now()}
}

func cacheDrop(hash [32]byte) {
	sessionCache.Lock()
	defer sessionCache.Unlock()
	delete(sessionCache.m, hash)
}

func cacheReset() {
	sessionCache.Lock()
	defer sessionCache.Unlock()
	sessionCache.m = make(map[[32]byte]cacheEntry)
}

// --- store -----------------------------------------------------------------

// Enabled reports whether real accounts are available: sessions and users
// need Postgres. Without it (PG-less local dev) the in-memory fallback store
// keeps anonymous identity working and every account endpoint answers 503;
// prod refuses to boot without Postgres before this can matter.
func Enabled() bool {
	return db.Ready()
}

// memStore is the PG-less fallback: anonymous sessions in a process-local
// map. Lost on restart, never authenticated.
var memStore = struct {
	sync.Mutex
	m map[[32]byte]*Session
}{m: make(map[[32]byte]*Session)}

// --- lifecycle -------------------------------------------------------------

// Mint creates a fresh anonymous session, sets the sid cookie, and returns
// the session. Returns nil only on store failure (logged).
func Mint(c fiber.Ctx) *Session {
	token, hash := NewToken()
	uid := config.GenerateCode(16, config.Base58)
	now := time.Now()
	sess := Session{
		UID:       uid,
		tokenHash: hash,
		lastSeen:  now,
		expiresAt: now.Add(anonTTL),
	}

	if Enabled() {
		// User-Agent is cloned out of fasthttp's pooled buffers before it is
		// retained anywhere (the established ctx-reuse discipline).
		ua := strings.Clone(c.Get(fiber.HeaderUserAgent))
		id, err := db.CreateSession(hash[:], uid, nil, sess.expiresAt, ua)
		if err != nil {
			util.Error(str.CAuth, "session mint failed error=%s", err.Error())
			return nil
		}
		sess.ID = id
	} else {
		memStore.Lock()
		memStore.m[hash] = &sess
		memStore.Unlock()
	}

	cachePut(sess)
	setSessionCookie(c, token)
	return &sess
}

// FromRequest resolves the request's sid cookie to its session, serving from
// the cache when fresh and throttling last_seen touches. Returns nil when
// there is no cookie, the token is malformed, or the session is missing,
// expired, or unresolvable.
func FromRequest(c fiber.Ctx) *Session {
	token := c.Cookies(SessionCookie)
	if token == "" {
		return nil
	}
	hash, ok := hashToken(token)
	if !ok {
		return nil
	}

	if s, hit := cacheGet(hash); hit {
		return &s
	}

	var sess Session
	if Enabled() {
		rec, found, err := db.GetSessionByTokenHash(hash[:])
		if err != nil {
			util.Error(str.CAuth, "session resolve failed error=%s", err.Error())
			return nil
		}
		if !found || time.Now().After(rec.ExpiresAt) {
			return nil
		}
		sess = Session{
			ID:        rec.ID,
			UID:       rec.UID,
			UserID:    rec.UserID,
			Username:  rec.Username,
			tokenHash: hash,
			lastSeen:  rec.LastSeen,
			expiresAt: rec.ExpiresAt,
		}
		// sliding expiry, throttled: refresh only when meaningfully stale
		if time.Since(sess.lastSeen) > touchInterval {
			sess.lastSeen = time.Now()
			sess.expiresAt = sess.lastSeen.Add(sess.ttl())
			if err := db.TouchSession(sess.ID, sess.expiresAt); err != nil {
				util.Error(str.CAuth, "session touch failed error=%s", err.Error())
			}
		}
	} else {
		memStore.Lock()
		s, found := memStore.m[hash]
		if found && time.Now().After(s.expiresAt) {
			delete(memStore.m, hash)
			found = false
		}
		if found && time.Since(s.lastSeen) > touchInterval {
			s.lastSeen = time.Now()
			s.expiresAt = s.lastSeen.Add(anonTTL)
		}
		if found {
			sess = *s
		}
		memStore.Unlock()
		if !found {
			return nil
		}
	}

	cachePut(sess)
	return &sess
}

// Login performs the in-place session upgrade after full authentication:
// token rotated (fixation defense), account attached, authed expiry applied,
// uid preserved. sess may be nil (a login POST with no live session — e.g.
// cookies cleared mid-flow); a fresh authenticated session is minted instead.
// Requires Enabled(); callers gate on it.
func Login(c fiber.Ctx, sess *Session, userID int64, username string) error {
	token, hash := NewToken()
	now := time.Now()

	if sess == nil {
		uid := config.GenerateCode(16, config.Base58)
		ua := strings.Clone(c.Get(fiber.HeaderUserAgent))
		id, err := db.CreateSession(hash[:], uid, &userID, now.Add(authedTTL), ua)
		if err != nil {
			return err
		}
		sess = &Session{ID: id, UID: uid}
	} else {
		if err := db.RotateSessionToken(
			sess.ID, hash[:], userID, now.Add(authedTTL)); err != nil {
			return err
		}
		cacheDrop(sess.tokenHash)
	}

	sess.UserID = &userID
	sess.Username = username
	sess.tokenHash = hash
	sess.lastSeen = now
	sess.expiresAt = now.Add(authedTTL)
	cachePut(*sess)
	setSessionCookie(c, token)
	return nil
}

// CurrentSession resolves the request's session (cache-first, no mint). Nil
// when there is no live session. Handlers use it for the account-admin gate
// and to get the current session id (the one to keep on a password change /
// "log out everywhere else").
func CurrentSession(c fiber.Ctx) *Session {
	return FromRequest(c)
}

// LogoutAll revokes every session the user holds — including the current one —
// and clears the cookie. The current session's cache entry is dropped so the
// revocation is immediate for this device (the others lapse within cacheTTL).
func LogoutAll(c fiber.Ctx, userID int64) error {
	if token := c.Cookies(SessionCookie); token != "" {
		if hash, ok := hashToken(token); ok {
			cacheDrop(hash)
		}
	}
	var err error
	if Enabled() {
		err = db.DeleteSessionsForUser(userID)
	}
	clearCookie(c, SessionCookie)
	return err
}

// Logout revokes the request's session (hard delete) and clears the cookie.
// The next page load mints a fresh anonymous session.
func Logout(c fiber.Ctx) {
	if token := c.Cookies(SessionCookie); token != "" {
		if hash, ok := hashToken(token); ok {
			cacheDrop(hash)
			if Enabled() {
				if err := db.DeleteSessionByTokenHash(hash[:]); err != nil {
					util.Error(str.CAuth, "logout delete failed error=%s",
						err.Error())
				}
			} else {
				memStore.Lock()
				delete(memStore.m, hash)
				memStore.Unlock()
			}
		}
	}
	clearCookie(c, SessionCookie)
}

// setSessionCookie writes the sid cookie.
//
// SameSite is Lax, not Strict: Strict cookies are unreliably attached to
// WebSocket upgrade requests by WebKit (all iOS browsers), which is how game
// sockets historically ended up seated as spectators. Lax still withholds the
// cookie from cross-site subresource requests and POSTs.
func setSessionCookie(c fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(cookieMaxAge / time.Second),
		Secure:   !env.IsLocal(),
		HTTPOnly: true,
		SameSite: "Lax",
	})
}

// clearCookie expires a cookie in the client.
func clearCookie(c fiber.Ctx, name string) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   !env.IsLocal(),
		HTTPOnly: true,
		SameSite: "Lax",
	})
}

// UserContext builds the request-scoped user.Context for a resolved session.
// The embedded context is always non-nil (the old cookie-decoded contexts
// carried a nil one, which is why view.Render renders on Background — see the
// comment there).
func UserContext(s *Session) *user.Context {
	ctx := &user.Context{Context: context.Background(), ID: s.UID}
	if s.LoggedIn() {
		ctx.Account = &user.Account{ID: *s.UserID, Username: s.Username}
	}
	return ctx
}

// UpSweeper starts the hourly expired-session sweep (and periodic cache/
// memStore hygiene). Mirrors the room persister's lifecycle pattern.
func UpSweeper() {
	go func() {
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		for range tick.C {
			if Enabled() {
				if n, err := db.DeleteExpiredSessions(); err != nil {
					util.Error(str.CAuth, "session sweep failed error=%s",
						err.Error())
				} else if n > 0 {
					util.Debug(str.CAuth, "session sweep removed=%d", n)
				}
			}
			memStore.Lock()
			for h, s := range memStore.m {
				if time.Now().After(s.expiresAt) {
					delete(memStore.m, h)
				}
			}
			memStore.Unlock()
			cacheReset()
		}
	}()
}
