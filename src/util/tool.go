package util

import (
	"bufio"
	// import embed for side-effects to allow embedding naughty.txt
	_ "embed"
	"math/rand"
	"strings"
	"time"

	"github.com/dechristopher/lioctad/str"
)

var (
	//go:embed data/naughty.txt
	naughtyFile string
)

var (
	charset     = "ABCDEFGHJKLMNPQRSTUVWXYZ123456789"
	charsetFull = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789"
	seededRand  = rand.New(rand.NewSource(time.Now().UnixNano()))
	naughty     []string
)

func init() {
	go loadNaughtyAsync()
}

// loadNaughtyAsync runs the loadNaughty function,
// but it should be called asynchronously on init
func loadNaughtyAsync() {
	naughty = loadNaughty()
}

// loadNaughty loads the naughty list on boot
func loadNaughty() []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(naughtyFile))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	Debug(str.CTool, str.DNaughty, len(lines))
	return lines
}

// Naughty returns whether a given word or phrase is appropriate
func Naughty(in string) bool {
	check := strings.ToLower(in)
	for _, word := range naughty {
		if word == check {
			return true
		}
	}
	return false
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

// MilliTime returns the current millisecond time
func MilliTime() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// TimeSinceBoot returns the time elapsed since process boot
func TimeSinceBoot() time.Duration {
	return time.Since(BootTime)
}
