package variant

import "github.com/dechristopher/lio/clock"

// Variant represents a timed octad variant
type Variant struct {
	Name     string `json:"name"`
	HTMLName string `json:"html_name"`
	Group    Group  `json:"group"`
	// Speed is the variant's speed class (bullet/blitz/rapid) when Group does
	// not already carry it. The deploy variants all share DeployGroup, which
	// collects them by pre-game rather than by pace, so the speed shorthand a
	// player reads a challenge by would otherwise be lost — see SpeedGroup.
	// Zero for the speed-grouped variants, which are their own speed class.
	Speed   Group             `json:"speed,omitempty"`
	Control clock.TimeControl `json:"time"`
	// Deploy enables the blind deploy pre-game for this variant: players
	// privately arrange their home rank before normal play begins.
	Deploy bool `json:"deploy,omitempty"`
	// Casual marks the untimed variants (see UnlimitedCasual): games with an
	// effectively infinite clock, playable against the computer or a human.
	// Casual rooms relax the idle/first-move timeouts while the players are
	// connected and instead cancel on disconnect (room.Params.Casual). Timed
	// variants are the "competitive" mode by contrast.
	Casual bool `json:"casual,omitempty"`
	// LockColors keeps each player on the same side across rematches. By default
	// subsequent games swap sides; a variant sets this to opt out.
	LockColors bool `json:"lock_colors,omitempty"`
}

// Group represents a collection of similar variants
type Group string

// String returns the group as a string
func (g Group) String() string {
	return string(g)
}

// SpeedGroup returns the variant's speed class: the explicit Speed when the
// variant's Group collects it by something other than pace (the deploy
// variants), otherwise Group itself, which already is the speed class. Views
// use this for the "½ + 1 · Blitz" shorthand so a deploy room still reads as
// bullet/blitz/rapid rather than the uninformative constant "Deploy".
func (v Variant) SpeedGroup() Group {
	if v.Speed != "" {
		return v.Speed
	}
	return v.Group
}

// Default variant groups
const (
	BulletGroup Group = "bullet"
	BlitzGroup  Group = "blitz"
	RapidGroup  Group = "rapid"
	HyperGroup  Group = "hyper"
	UltiGroup   Group = "ulti"
	// DeployGroup collects variants played with the blind deploy pre-game.
	DeployGroup Group = "deploy"
	// UnlimitedGroup collects the untimed casual variants.
	UnlimitedGroup Group = "unlimited"
)
