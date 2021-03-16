package variant

import "github.com/dechristopher/lioctad/clock"

// Variant represents a timed octad variant
type Variant struct {
	Name  string            `json:"name"`
	Group Group             `json:"group"`
	Time  clock.TimeControl `json:"time"`
}

// Group represents a collection of similar variants
type Group string

// Default variant groups
const (
	BulletGroup Group = "bullet"
	BlitzGroup  Group = "blitz"
	RapidGroup  Group = "rapid"
	HyperGroup  Group = "hyper"
	UltiGroup   Group = "ulti"
)
