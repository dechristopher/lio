package config

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/rng"
)

type Charset int

const (
	Hex Charset = iota
	Base58

	// Version of lio
	Version = "v0.9.4"

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

	// CacheKey that is injected into static asset URLs to bust
	// the cache between deploys of the site
	// TODO: fix this with a proper cache busting system...
	CacheKey = fmt.Sprintf(".%s",
		GenerateCode(7, Base58))

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
	if Revision != "" {
		return Version + "+" + Revision
	}
	return Version
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

// GetListenPort returns the colon-formatted listen port
func GetListenPort() string {
	return fmt.Sprintf(":%s", GetPort())
}

// SiteURL returns the site URL based on environment configuration
func SiteURL() string {
	if !env.IsProd() {
		return fmt.Sprintf("http://localhost:%s/", GetPort())
	}
	return "https://lioctad.org/"
}

// CorsOrigins returns the proper CORS origin configuration
// for the current environment
func CorsOrigins() string {
	if env.IsProd() {
		return "https://lioctad.org"
	}
	return "http://localhost:4444, " +
		"http://localhost:8080, " +
		"https://dev.lioctad.org"
}
