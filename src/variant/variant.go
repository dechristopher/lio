package variant

import "github.com/dechristopher/lio/clock"

// Variant represents a timed octad variant
type Variant struct {
	Name     string            `json:"name"`
	HTMLName string            `json:"html_name"`
	Group    Group             `json:"group"`
	Control  clock.TimeControl `json:"time"`
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
