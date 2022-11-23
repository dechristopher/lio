package pools

import "github.com/dechristopher/lio/variant"

var Map map[string]variant.Variant

func init() {
	Map = make(map[string]variant.Variant)

	for _, ratingPool := range RatingPools {
		for _, control := range ratingPool {
			Map[control.HTMLName] = control
		}
	}
}

// RatingPools is a map of all active competitive octad variants
// on the site grouped by variant group as the individual pools
var RatingPools = map[variant.Group][]variant.Variant{
	"0" + variant.HyperGroup: {
		variant.ThreeZeroHyper,
	},
	"1" + variant.BulletGroup: {
		variant.FiveZeroBullet,
	},
	"2" + variant.BlitzGroup: {
		variant.QuarterZeroBlitz,
		variant.QuarterOneBlitz,
	},
	"3" + variant.RapidGroup: {
		variant.HalfZeroRapid,
		variant.HalfTwoRapid,
	},
	//variant.UltiGroup: {
	//	variant.ZeroFiveUlti,
	//},
}
