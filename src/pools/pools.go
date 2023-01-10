package pools

import (
	"github.com/dechristopher/lio/clock"
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/dechristopher/lio/variant"
)

var Map map[string]*wsv1.Variant

// ClientPools has its time controls set to use milliseconds instead of nanoseconds
var ClientPools = wsv1.VariantPools{}

func init() {
	Map = make(map[string]*wsv1.Variant)
	ClientPools.Pools = make(map[string]*wsv1.Variants)

	for variantGroup, ratingPool := range ratingPools.Pools {
		ClientPools.Pools[variantGroup] = &wsv1.Variants{Variants: []*wsv1.Variant{}}
		for _, control := range ratingPool.Variants {
			Map[control.HtmlName] = control
			// modify the game variant time control to be in milliseconds instead of nanoseconds and track it within the
			// pools exposed to the client
			ClientPools.Pools[variantGroup].Variants = append(ClientPools.Pools[variantGroup].Variants, clock.ConvertVariantTimeControl(control))
		}
	}
}

// ratingPools is a map of all active competitive octad variants
// on the site grouped by variant group as the individual pools
var ratingPools = wsv1.VariantPools{
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
