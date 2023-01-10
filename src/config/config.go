package config

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/dechristopher/lio/env"
)

type Charset int

const (
	Hex Charset = iota
	Base58

	// Version of lio
	Version = "v0.5.6"

	charsetHex    = "abcdef01234567890"
	charsetBase58 = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ123456789"
)

var (
	// BootTime is set the instant everything comes online
	BootTime time.Time

	// CacheKey that is injected into static asset URLs to bust
	// the cache between deploys of the site
	CacheKey = fmt.Sprintf(".%s",
		GenerateCode(7, Base58))

	// CryptoKey for use with cryptographic operations in lio
	CryptoKey = "testkeyforthelioctadcryptosystem" //ReadSecretFallback("crypto_key")

	// DebugFlagPtr contains raw debug flags direct from STDIN
	DebugFlagPtr *string
	// DebugFlags holds all active, parsed debug flags
	DebugFlags = make(map[string]bool)
)

// ReadSecretFallback attempts to read a secret from the secret
// path, returns environment variable of same name if error
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

	secret, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}

	return string(secret), nil
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
				b[i] = charsetHex[rand.Intn(len(charsetHex))]
			case Base58:
				b[i] = charsetBase58[rand.Intn(len(charsetBase58))]
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
		"https://dev.lioctad.org, " +
		"http://localhost:3000"
}
