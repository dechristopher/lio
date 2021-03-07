package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/dechristopher/lioctad/env"
)

var (
	// Version of lio
	Version = "v0.1.2"

	// BootTime is set the instant everything comes online
	BootTime time.Time

	// DebugFlagPtr contains raw debug flags direct from STDIN
	DebugFlagPtr *string
	// DebugFlags holds all active, parsed debug flags
	DebugFlags []string
)

// ReadSecretFallback attempts to read a secret from the secrets
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

// devPrefix returns dev_ only on dev environments
func devPrefix() string {
	if env.IsDev() {
		return "dev_"
	}
	return ""
}

// IsDebugFlag returns true if a given debug flag is enabled in this instance
func IsDebugFlag(flag string) bool {
	for _, f := range DebugFlags {
		if f == flag {
			return true
		}
	}

	return false
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
	return "http://localhost:4444, https://dev.lioctad.org"
}
