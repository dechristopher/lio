package main

import (
	"embed"
	"flag"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/dechristopher/lioctad/env"
	"github.com/dechristopher/lioctad/store"
	"github.com/dechristopher/lioctad/str"
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

	isHealthCheck := flag.Bool(str.FHealth, false, str.FHealthUsage)
	util.DebugFlagPtr = flag.String(str.FDebugFlags, "", str.FDebugFlagsUsage)
	flag.Parse()

	util.DebugFlags = strings.Split(*util.DebugFlagPtr, ",")

	// run health check if told
	if *isHealthCheck {
		//executeHealthCheck()
		return
	}

	if !env.IsProd() {
		util.Debug(str.CMain, str.MDevMode)
	}
}

// main does the things
func main() {
	_ = godotenv.Load()
	go store.Up()
	www.Serve(views, static)
}
