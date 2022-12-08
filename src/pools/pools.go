package pools

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/dechristopher/lio/variant"
)

var Map map[string]*wsv1.Variant

func init() {
	Map = make(map[string]*wsv1.Variant)

	for _, ratingPool := range RatingPools.Pools {
		for _, control := range ratingPool.Variants {
			Map[control.HtmlName] = control
		}
	}
}

// RatingPools is a map of all active competitive octad variants
// on the site grouped by variant group as the individual pools
var RatingPools = wsv1.VariantPools{
	Pools: map[string]*wsv1.Variants{
		wsv1.VariantGroup_VARIANT_GROUP_HYPER.String(): {Variants: []*wsv1.Variant{
			variant.ThreeZeroHyper,
		}},
		wsv1.VariantGroup_VARIANT_GROUP_BULLET.String(): {Variants: []*wsv1.Variant{
			variant.FiveZeroBullet,
		}},
		wsv1.VariantGroup_VARIANT_GROUP_BLITZ.String(): {Variants: []*wsv1.Variant{
			variant.QuarterZeroBlitz,
			variant.QuarterOneBlitz,
		}},
		wsv1.VariantGroup_VARIANT_GROUP_RAPID.String(): {Variants: []*wsv1.Variant{
			variant.HalfZeroRapid,
			variant.HalfTwoRapid,
		}},
	},
}
