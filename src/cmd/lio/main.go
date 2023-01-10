package main

import (
	"embed"
	"flag"
	"github.com/dechristopher/lio/www"
	"math/rand"
	"strings"
	"time"

	"github.com/dechristopher/lio/crypt"
	"github.com/joho/godotenv"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/systems"
	"github.com/dechristopher/lio/util"
)

var (
	//go:embed views/*
	views embed.FS
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

	// run health check if told
	if *isHealthCheck {
		//executeHealthCheck()
		return
	}

	if !env.IsProd() {
		// print development mode warning
		util.Debug(str.CMain, str.MDevMode)
	}

	// Seed PRNG
	rand.Seed(time.Now().UTC().UnixNano())

	// test that crypto system is operational
	_, _ = crypt.Encrypt([]byte("lio"))
}

// main does the things
func main() {
	// load .env if any
	_ = godotenv.Load()

	// asynchronously initialize subsystems
	go systems.Run()

	// serve primary http endpoints
	www.Serve(views, static)
}
