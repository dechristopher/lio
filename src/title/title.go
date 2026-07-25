// Package title models an account's optional display title — the compact
// accent-colored badge rendered to the left of a username wherever a name
// shows (header, live clocks, match timeline, pre-game cards, archive, PGN
// tags, and the OG challenge card).
//
// Titles are rows in the `titles` table (db/migrations/00014_titles.sql), not
// free-form text on the user: an account references one by id, so a title's
// wording is owned in one place. This is the display-side value copied out of
// that row and carried through the session, the seat, and the render — the
// row id itself stays in the data layer, since nothing above it does anything
// but show the badge.
//
// The zero Title means "no title", the case for anonymous/bot seats and the
// vast majority of accounts, so it can be threaded and rendered
// unconditionally.
package title

// Title is an account's display title: the short badge Code ("GM") plus the
// full Name it renders as its hover tooltip ("Grandmaster"). Both come from
// the same titles row; Name is never empty for a real row, but Tooltip
// tolerates a missing one.
type Title struct {
	Code string
	Name string
}

// New builds a Title from a titles row's nullable columns (the shape every
// LEFT JOIN in the users/sessions queries produces): a NULL code — no title
// assigned — yields the zero Title.
func New(code, name *string) Title {
	if code == nil || *code == "" {
		return Title{}
	}
	t := Title{Code: *code}
	if name != nil {
		t.Name = *name
	}
	return t
}

// Set reports whether a title is assigned. Renderers skip the badge entirely
// when it is false.
func (t Title) Set() bool {
	return t.Code != ""
}

// Tooltip is the badge's hover text: the title's full name, falling back to
// the code so a row with an empty name (or a snapshot restored from a build
// that only carried the code) still gets a sensible title attribute.
func (t Title) Tooltip() string {
	if t.Name == "" {
		return t.Code
	}
	return t.Name
}
