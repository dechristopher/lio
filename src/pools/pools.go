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

// RatingPools is a map of all active competitive octad variants on the site
// grouped by variant group as the individual pools. The offering is curated to
// three shared time controls, each playable classic (bullet/blitz/rapid) or with
// the blind deploy pre-game — see CreateControls for the create-game pairing.
var RatingPools = map[variant.Group][]variant.Variant{
	"0" + variant.BulletGroup: {
		variant.QuarterZeroBullet,
	},
	"1" + variant.BlitzGroup: {
		variant.HalfOneBlitz,
	},
	"2" + variant.RapidGroup: {
		variant.OneTwoRapid,
	},
	"3" + variant.DeployGroup: {
		variant.QuarterZeroBulletDeploy,
		variant.HalfOneBlitzDeploy,
		variant.OneTwoRapidDeploy,
	},
}

// CreateControl is one time control offered in the custom create-game modal, in
// both its classic and blind-deploy forms. The two variants share a time control
// (and thus a display Label) and differ only by the deploy pre-game, so the UI
// presents a single time-control choice plus a Classic/Deploy mode toggle.
type CreateControl struct {
	// Label is the shared display name of the time control, e.g. "½ + 1".
	Label string
	// Group is the classic speed group (bullet/blitz/rapid) shown as a sublabel.
	Group variant.Group
	// Classic and Deploy are the two variants this control resolves to.
	Classic variant.Variant
	Deploy  variant.Variant
}

// CreateControls is the curated set of time controls offered in the custom
// create-game modal: three shared time controls (bullet ¼+0, blitz ½+1, rapid
// 1+2), each playable classic or with the blind deploy pre-game.
var CreateControls = []CreateControl{
	{
		Label:   variant.QuarterZeroBullet.Name,
		Group:   variant.BulletGroup,
		Classic: variant.QuarterZeroBullet,
		Deploy:  variant.QuarterZeroBulletDeploy,
	},
	{
		Label:   variant.HalfOneBlitz.Name,
		Group:   variant.BlitzGroup,
		Classic: variant.HalfOneBlitz,
		Deploy:  variant.HalfOneBlitzDeploy,
	},
	{
		Label:   variant.OneTwoRapid.Name,
		Group:   variant.RapidGroup,
		Classic: variant.OneTwoRapid,
		Deploy:  variant.OneTwoRapidDeploy,
	},
}
