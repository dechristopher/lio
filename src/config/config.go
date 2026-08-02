package config

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/rng"
)

type Charset int

const (
	// Version of lio
	Version = "v1.12.2"

	Hex Charset = iota
	Base58

	charsetHex    = "abcdef01234567890"
	charsetBase58 = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ123456789"
)

var (
	// Revision is the short git commit hash the running binary was built from. It
	// is empty in local/dev builds and injected at release build time via ldflags
	// (`-X github.com/dechristopher/lio/config.Revision=<hash>`) — see the
	// Dockerfile GIT_REV arg and deploy/deploy-fly.sh. VersionString folds it into
	// the displayed version.
	Revision string

	// BootTime is set the instant everything comes online
	BootTime time.Time

	// CryptoKey for use with cryptographic operations in lio
	CryptoKey = ReadSecretFallback("crypto_key")

	// DebugFlagPtr contains raw debug flags direct from STDIN
	DebugFlagPtr *string
	// DebugFlags holds all active, parsed debug flags
	DebugFlags = make(map[string]bool)
)

// ReadSecretFallback attempts to read a secret from the secret
// path, returns environment variable of the same name if error
func ReadSecretFallback(name string) string {
	secret, err := ReadSecret(name)
	if err != nil {
		return os.Getenv(strings.ToUpper(devPrefix() + name))
	}

	return secret
}

// ReadSecret will read a secret string from a file
func ReadSecret(name string) (string, error) {
	f, err := os.Open("/run/secrets/" + devPrefix() + name)
	if err != nil {
		return "", err
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Printf("failed to close file: %v", err)
		}
	}(f)

	secret, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	return string(secret), nil
}

// VersionString returns the display version: the base Version, suffixed with the
// build's git revision as semver build metadata (v0.9.0+6f80260) when Revision
// was injected at build time. Local/dev builds carry no Revision and show just
// the base version.
func VersionString() string {
	if Revision == "" {
		Revision = "local"
	}
	return Version + "+" + Revision
}

// GenerateCode generates an N character sequence with naughty safety baked in
func GenerateCode(length int, charset ...Charset) string {
	b := make([]byte, length)
	var cs Charset

	if charset == nil || len(charset) == 0 {
		cs = Base58
	} else {
		cs = charset[0]
	}

	for {
		for i := range b {
			switch cs {
			case Hex:
				b[i] = charsetHex[rng.Intn(len(charsetHex))]
			case Base58:
				b[i] = charsetBase58[rng.Intn(len(charsetBase58))]
			default:
				panic("GenerateCode: invalid charset")
			}
		}

		if !Naughty(string(b)) {
			return string(b)
		}
	}
}

// devPrefix returns dev_ only on dev environments
func devPrefix() string {
	if !env.IsProd() {
		return "dev_"
	}
	return ""
}

// IsDebugFlag returns true if a given debug flag is enabled in this instance
func IsDebugFlag(flag string) bool {
	return DebugFlags[flag] == true
}

// GetPort returns the configured primary HTTP port
func GetPort() string {
	return os.Getenv("PORT")
}

// PlausibleDomain returns the host of the self-hosted Plausible analytics
// instance (PLAUSIBLE_DOMAIN env var, e.g. "plausible.example.com"). Kept out
// of VCS deliberately; empty means analytics are disabled — the tracker script
// is not rendered and the CSP stays same-origin only.
func PlausibleDomain() string {
	return os.Getenv("PLAUSIBLE_DOMAIN")
}

// GetListenPort returns the colon-formatted listen port
func GetListenPort() string {
	return fmt.Sprintf(":%s", GetPort())
}

// GetHealthPort returns the port of the internal health listener
// (HEALTH_PORT env var, defaulting to 4445)
func GetHealthPort() string {
	if port := os.Getenv("HEALTH_PORT"); port != "" {
		return port
	}
	return "4445"
}

// GetHealthAddr returns the loopback-only listen address of the internal
// health listener. Bound to 127.0.0.1 inside the container's network
// namespace, it is unreachable from outside the container — health checks
// (`lio --health`) are purely internal.
func GetHealthAddr() string {
	return "127.0.0.1:" + GetHealthPort()
}

// SiteHost returns the canonical public host (no scheme), env-overridable so a
// future domain move is one env var + DNS. Production defaults to "octad.gg"
// (SITE_DOMAIN overrides); non-prod is the local listen host.
func SiteHost() string {
	if !env.IsProd() {
		return "localhost:" + GetPort()
	}
	if h := os.Getenv("SITE_DOMAIN"); h != "" {
		return h
	}
	return "octad.gg"
}

// SiteName returns the display brand shown in page titles, the PWA name, the OG
// wordmark, and the TOTP authenticator label. It mirrors the domain by default
// ("octad.gg", SITE_NAME overrides) but is deliberately decoupled from SiteHost
// so local dev still brands as "octad.gg" rather than "localhost:4444".
func SiteName() string {
	if n := os.Getenv("SITE_NAME"); n != "" {
		return n
	}
	return "octad.gg"
}

// SiteURL returns the site URL (scheme + host + trailing slash) based on
// environment configuration.
func SiteURL() string {
	if !env.IsProd() {
		return "http://" + SiteHost() + "/"
	}
	return "https://" + SiteHost() + "/"
}

// SiteOrigin returns the site origin (scheme + host, no trailing slash) — the
// form used for CORS/WS origin matching and absolute OpenGraph/PGN URLs.
func SiteOrigin() string {
	return strings.TrimSuffix(SiteURL(), "/")
}

// CorsOrigins returns the proper CORS origin configuration for the current
// environment. Production pins the canonical origin (SiteOrigin); everywhere
// else the wildcard admits any origin, so LAN devices, tunnels, and test
// harnesses can hit a non-prod server without curating an allowlist. The other
// two origin gates (middleware.MutationGuard, ws.okOrigin) stand down outside
// production the same way — a "*" entry would never match their exact-origin
// comparisons, so they carry explicit env bypasses instead of consuming this
// wildcard. EXTRA_ORIGINS (comma-separated) appends additional allowed origins
// in prod — used to admit the old domain during a migration cutover window.
func CorsOrigins() string {
	if !env.IsProd() {
		return "*"
	}
	origins := SiteOrigin()
	if extra := os.Getenv("EXTRA_ORIGINS"); extra != "" {
		origins += "," + extra
	}
	return origins
}
