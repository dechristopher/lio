package variant

import "github.com/dechristopher/lioctad/clock"

// Variant represents a timed octad variant
type Variant struct {
	Name  string
	Group Group
	Time  clock.TimeControl
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
