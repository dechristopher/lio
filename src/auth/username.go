package auth

import (
	"errors"
	"regexp"
	"strings"
)

// Username policy: 3–20 characters, letters/digits/underscore/hyphen, must
// start with a letter or digit. Uniqueness is case-insensitive (the
// lower(username) unique index) while display case is preserved. Renames are
// not offered initially.
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{2,19}$`)

// reservedUsernames are lowercase names that would collide with UI labels,
// impersonate the site, or otherwise confuse — rejected at registration.
var reservedUsernames = map[string]struct{}{
	"anonymous": {}, "anon": {}, "bot": {}, "computer": {}, "engine": {},
	"admin": {}, "administrator": {}, "mod": {}, "moderator": {},
	"lioctad": {}, "lichess": {}, "octad": {}, "lio": {},
	"you": {}, "player": {}, "opponent": {}, "spectator": {},
	"system": {}, "root": {}, "staff": {}, "support": {}, "official": {},
}

var (
	// ErrUsernameInvalid rejects names failing the pattern.
	ErrUsernameInvalid = errors.New(
		"usernames are 3-20 letters, numbers, _ or -, starting with a letter or number")
	// ErrUsernameReserved rejects reserved names.
	ErrUsernameReserved = errors.New("that username is reserved")
)

// ValidateUsername checks a candidate username against the pattern and the
// reserved list. Availability against existing users is the database's
// lower(username) unique index, checked separately.
func ValidateUsername(name string) error {
	if !usernamePattern.MatchString(name) {
		return ErrUsernameInvalid
	}
	if _, reserved := reservedUsernames[strings.ToLower(name)]; reserved {
		return ErrUsernameReserved
	}
	return nil
}
