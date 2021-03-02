package util

import (
	"fmt"
	"os"
	"time"
)

var (
	// BootTime is set the instant everything comes online
	BootTime time.Time

	DebugFlagPtr *string
	DebugFlags   []string
)

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
