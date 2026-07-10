package main

import (
	"embed"
	"flag"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dechristopher/lio/crypt"
	"github.com/joho/godotenv"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/systems"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www"
)

var (
	//go:embed static/*
	static embed.FS
)

// init parses flags, sets constants, and prepares us for battle
func init() {
	// set boot time immediately
	config.BootTime = time.Now()

	// parse command line flags
	isHealthCheck := flag.Bool(str.FHealth, false, str.FHealthUsage)
	config.DebugFlagPtr = flag.String(str.FDebugFlags, "", str.FDebugFlagsUsage)
	flag.Parse()

	// parse out debug flags from command line options
	for _, debugFlag := range strings.Split(*config.DebugFlagPtr, ",") {
		config.DebugFlags[debugFlag] = true
	}

	// run health check if told (this exits the process; the server never starts)
	if *isHealthCheck {
		executeHealthCheck()
		return
	}

	if !env.IsProd() {
		// print development mode warning
		util.Debug(str.CMain, str.MDevMode)
	}

	// test that crypto system is operational
	_, _ = crypt.Encrypt([]byte("lio"))
}

// executeHealthCheck probes the running server's lightweight status endpoint
// over loopback and exits the process 0 (healthy) or non-zero (unhealthy). It is
// the container HEALTHCHECK: the scratch runtime image has no shell or wget, so
// the binary must check itself. It hits /lio (JSON status, no side effects, no
// object-store dependency) and never starts the server or its subsystems.
func executeHealthCheck() {
	client := http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get("http://127.0.0.1:" + config.GetPort() + "/lio")
	if err != nil {
		util.Error(str.CMain, "health check failed: %s", err.Error())
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		util.Error(str.CMain, "health check failed: status %d", resp.StatusCode)
		os.Exit(1)
	}

	os.Exit(0)
}

// main does the things
func main() {
	// load .env if any
	_ = godotenv.Load()

	// asynchronously initialize subsystems
	go systems.Run()

	// serve primary http endpoints
	www.Serve(static)
}
