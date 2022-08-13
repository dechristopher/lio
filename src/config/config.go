package config

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/dechristopher/lioctad/env"
)

var (
	// Version of lio
	Version = "v0.4.0"

	// BootTime is set the instant everything comes online
	BootTime time.Time

	// CacheKey that is injected into static asset URLs to bust
	// the cache between deploys of the site
	CacheKey = fmt.Sprintf(".%s",
		GenerateCode(7, true))

	// DebugFlagPtr contains raw debug flags direct from STDIN
	DebugFlagPtr *string
	// DebugFlags holds all active, parsed debug flags
	DebugFlags = make(map[string]bool)

	charset     = "ABCDEFGHJKLMNPQRSTUVWXYZ123456789"
	charsetFull = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789"
	seededRand  = rand.New(rand.NewSource(time.Now().UnixNano()))
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
func GenerateCode(length int, useFullCharset bool) string {
	b := make([]byte, length)
	for {
		for i := range b {
			if !useFullCharset {
				b[i] = charset[seededRand.Intn(len(charset))]
			} else {
				b[i] = charsetFull[seededRand.Intn(len(charsetFull))]
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
