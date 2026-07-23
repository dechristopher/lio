package config

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
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

// Username blocklisting -----------------------------------------------------
//
// NaughtyUsername reuses the same wordlist but matches it against a *username*
// candidate, where whole-string equality (the Naughty check above) is too
// weak: nobody registering "fuck1" is fooled by it. The hard part is doing
// better without becoming the classic "Scunthorpe problem" — rejecting Class,
// assassin, Cockburn, or analyst because a short banned fragment nests inside
// them.
//
// The trick is to key the *match rule* off word length, which is a free proxy
// for collision risk: the notorious false-positive fragments (ass, cock, anal,
// cunt) are all short, so short words are only matched as a *whole token*,
// while longer, more specific words (which rarely nest inside real words) are
// matched as substrings. See arch notes / CLAUDE.md for the full rationale.
//
//   - phrases (spaces) and symbol entries (a$$, s&m) are dropped — a username's
//     charset can't reproduce them, and their stripped fragments are noise;
//   - words normalizing to <3 chars (ho, xx) are dropped — too collision-prone
//     to match even as a token;
//   - words of 3-4 chars match a whole token only (split on separators/digits);
//   - words of 5+ chars match as a substring of the normalized name.
//
// Precision is favored over recall: determined evasion (padding a short word,
// deliberate misspellings) can slip through; false positives on real names are
// what we work to avoid.
const naughtyTokenMax = 4

var (
	naughtyOnce  sync.Once
	naughtyShort map[string]struct{} // 3-4 char words, whole-token match
	naughtyLong  []string            // 5+ char words, substring match
)

// naughtyAllow are known false positives: any of these substrings is blanked
// from the normalized name before the substring pass, so e.g. "therapist" no
// longer trips "rapist". Grow this from the false-positive test corpus rather
// than by loosening the matcher.
var naughtyAllow = []string{"therapist"}

// naughtyLeet folds the common leetspeak digit substitutions that fit a
// username's charset. Kept conservative on purpose (precision posture): odd
// digits are left as token separators rather than force-folded.
var naughtyLeet = strings.NewReplacer(
	"0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t",
)

// stripNaughty folds leetspeak and drops everything but a-z: the canonical form
// for whole-token / whole-name comparison ("a55" -> "ass", "f_u_c_k" -> "fuck").
func stripNaughty(s string) string {
	s = naughtyLeet.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// squashNaughty is stripNaughty plus run collapsing ("fuuuck" -> "fuck"): the
// form used for the substring pass, so repeated-letter padding can't hide a
// long word.
func squashNaughty(s string) string {
	stripped := stripNaughty(s)
	var b strings.Builder
	var prev rune
	for _, r := range stripped {
		if r != prev {
			b.WriteRune(r)
			prev = r
		}
	}
	return b.String()
}

// tokenizeNaughty splits a lowercased name into a-z runs, so digits, "_" and
// "-" all act as token boundaries ("fuck123" -> [fuck], "fuck_you" -> [fuck you]).
func tokenizeNaughty(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r < 'a' || r > 'z'
	})
}

// buildNaughtyIndex partitions the wordlist into the short (token) and long
// (substring) buckets, normalized like candidates so the comparisons line up.
func buildNaughtyIndex() {
	naughtyShort = make(map[string]struct{})
	seenLong := make(map[string]struct{})
	for _, word := range loadNaughty() {
		lw := strings.ToLower(word)
		// drop phrases and symbol entries: a username can't contain them, and
		// their stripped remnants are collision-prone noise.
		if strings.ContainsFunc(lw, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
		}) {
			continue
		}
		switch norm := stripNaughty(lw); {
		case len(norm) < 3:
			// too short to match safely, even as a token
		case len(norm) <= naughtyTokenMax:
			naughtyShort[norm] = struct{}{}
		default:
			sq := squashNaughty(lw)
			if _, dup := seenLong[sq]; !dup {
				seenLong[sq] = struct{}{}
				naughtyLong = append(naughtyLong, sq)
			}
		}
	}
	if len(naughtyShort) == 0 || len(naughtyLong) == 0 {
		panic("naughty username index empty")
	}
}

// NaughtyUsername reports whether a username candidate contains disallowed
// language, using the length-tiered matching described above. Callers should
// surface a single generic rejection — never echo which word matched.
func NaughtyUsername(name string) bool {
	naughtyOnce.Do(buildNaughtyIndex)

	// short / whole-token pass
	if _, ok := naughtyShort[stripNaughty(name)]; ok {
		return true
	}
	for _, tok := range tokenizeNaughty(name) {
		if _, ok := naughtyShort[tok]; ok {
			return true
		}
	}

	// long / substring pass, over the run-collapsed form with known false
	// positives blanked out first.
	squashed := squashNaughty(name)
	for _, safe := range naughtyAllow {
		squashed = strings.ReplaceAll(squashed, safe, "")
	}
	for _, w := range naughtyLong {
		if strings.Contains(squashed, w) {
			return true
		}
	}
	return false
}
