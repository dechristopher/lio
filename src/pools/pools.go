package pools

import "github.com/dechristopher/lioctad/variant"

var Map map[string]variant.Variant

func init() {
	Map = make(map[string]variant.Variant)

	for _, rating_pool := range RatingPools {
		for _, variant := range rating_pool {
			Map[variant.HTMLName] = variant
		}
	}
}

// RatingPools is a map of all active competitive octad variants
// on the site grouped by variant group as the individual pools
var RatingPools = map[variant.Group][]variant.Variant{
	variant.BulletGroup: {
		variant.QuarterZeroBullet,
		variant.QuarterOneBullet,
	},
	variant.BlitzGroup: {
		variant.HalfZeroBlitz,
		variant.HalfOneBlitz,
	},
	variant.RapidGroup: {
		variant.OneZeroRapid,
		variant.OneTwoRapid,
	},
	//variant.HyperGroup: {
	//	variant.FiveZeroHyper,
	//},
	//variant.UltiGroup: {
	//	variant.ZeroFiveUlti,
	//},
}
