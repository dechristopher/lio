package pools

import "github.com/dechristopher/lioctad/variant"

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
