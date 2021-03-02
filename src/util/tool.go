package util

import (
	"bufio"
	"math/rand"
	"os"
	"strings"
	"time"
)

var (
	naughtyList = "static/naughty.txt"
	charset     = "ABCDEFGHJKLMNPQRSTUVWXYZ123456789"
	charsetFull = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789"
	seededRand  = rand.New(rand.NewSource(time.Now().UnixNano()))
	naughty     = loadNaughty()
)

// loadNaughty loads the naughty list on boot
func loadNaughty() []string {
	file, err := os.Open(naughtyList)
	if err != nil {
		return nil
	}

	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

// Naughty returns whether or not a given word or phrase is appropriate
func Naughty(in string) bool {
	check := strings.ToLower(in)
	for _, word := range naughty {
		if word == check {
			return true
		}
	}
	return false
}

// GenerateCode generates a 4 character party join code
// with naughty safety baked in
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
