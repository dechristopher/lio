package config

import (
	"bufio"
	_ "embed"
	"strings"
)

var (
	//go:embed data/naughty.txt
	naughtyFile string
)

var (
	naughty []string
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
