package config

import (
	"bufio"
	_ "embed"
	"strings"
)

var (
	//go:embed data/naughty.txt
	naughtyFile string

	naughty []string
)

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
	if len(naughty) == 0 {
		naughty = loadNaughty()
		if len(naughty) == 0 {
			panic("naughty list missing")
		}
	}

	check := strings.ToLower(in)
	for _, word := range naughty {
		if word == check {
			return true
		}
	}
	return false
}
