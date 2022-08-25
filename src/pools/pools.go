package pools

import "github.com/dechristopher/lioctad/variant"

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
	"0" + variant.BulletGroup: {
		variant.QuarterZeroBullet,
		variant.QuarterOneBullet,
	},
	"1" + variant.BlitzGroup: {
		variant.HalfZeroBlitz,
		variant.HalfOneBlitz,
	},
	"2" + variant.RapidGroup: {
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
