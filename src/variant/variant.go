package variant

import "github.com/dechristopher/lio/clock"

// Variant represents a timed octad variant
type Variant struct {
	Name     string            `json:"name"`
	HTMLName string            `json:"html_name"`
	Group    Group             `json:"group"`
	Control  clock.TimeControl `json:"time"`
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
)
