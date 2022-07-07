package main

import (
	"embed"
	"flag"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/dechristopher/lioctad/env"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/systems"
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/lioctad/www"
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
	util.BootTime = time.Now()

	// parse command line flags
	isHealthCheck := flag.Bool(str.FHealth, false, str.FHealthUsage)
	util.DebugFlagPtr = flag.String(str.FDebugFlags, "", str.FDebugFlagsUsage)
	flag.Parse()

	// parse out debug flags from command line options
	for _, debugFlag := range strings.Split(*util.DebugFlagPtr, ",") {
		util.DebugFlags[debugFlag] = true
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
