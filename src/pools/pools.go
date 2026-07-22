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

	// the untimed casual variants are resolvable by HTMLName (the bot-game
	// handlers and "same settings" rematch links look variants up here) but are
	// deliberately not rating pools: casual games are unrated and never pooled.
	// NewCustomRoom only reaches them through the casual toggle.
	Map[variant.UnlimitedCasual.HTMLName] = variant.UnlimitedCasual
	Map[variant.UnlimitedCasualDeploy.HTMLName] = variant.UnlimitedCasualDeploy
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
		variant.ThreeFiveRapid,
	},
	"3" + variant.DeployGroup: {
		variant.QuarterZeroBulletDeploy,
		variant.HalfOneBlitzDeploy,
		variant.OneTwoRapidDeploy,
		variant.ThreeFiveRapidDeploy,
	},
}

// CreateControl is one time control offered in the custom create-game modal. It
// carries both the non-deploy and blind-deploy variants that share this time
// control (and thus a display Label). The modal now offers only the Deploy form
// (every game is blind-deploy, surfaced as "Octad"); Classic is retained under
// the hood for possible future modes and legacy rooms.
type CreateControl struct {
	// Label is the shared display name of the time control, e.g. "½ + 1".
	Label string
	// Group is the speed group (bullet/blitz/rapid) shown as a sublabel.
	Group variant.Group
	// Classic and Deploy are the two variants this control resolves to. The modal
	// uses Deploy; Classic is retained for legacy/future use.
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
	{
		Label:   variant.ThreeFiveRapid.Name,
		Group:   variant.RapidGroup,
		Classic: variant.ThreeFiveRapid,
		Deploy:  variant.ThreeFiveRapidDeploy,
	},
}

// RatingCategoryInfo describes how a rating category (a variant HTMLName, the
// per-time-control Glicko-2 key) is displayed in the profile popover: its time
// control, speed group, and game mode. Mode is empty for the default deploy mode
// (surfaced as "Octad", so the UI shows no mode header); a future non-default
// mode carries its own label so it can be grouped under one.
type RatingCategoryInfo struct {
	TimeControl string // shared time-control label, e.g. "1 + 2"
	Speed       string // speed group: "bullet" / "blitz" / "rapid"
	Mode        string // "" for the default deploy mode; e.g. "Classic" otherwise
	Order       int    // canonical sort order (bullet < blitz < 1+2 < 3+5)
}

// ratingCategories maps every rateable variant HTMLName to its display info,
// built once from CreateControls — the only place the deploy variant → speed
// group mapping survives, since deploy variants themselves carry Group "deploy".
var ratingCategories = map[string]RatingCategoryInfo{}

func init() {
	for i, ctrl := range CreateControls {
		ratingCategories[ctrl.Deploy.HTMLName] = RatingCategoryInfo{
			TimeControl: ctrl.Label,
			Speed:       ctrl.Group.String(),
			Mode:        "", // default deploy mode — surfaced as Octad, no header
			Order:       i,
		}
		ratingCategories[ctrl.Classic.HTMLName] = RatingCategoryInfo{
			TimeControl: ctrl.Label,
			Speed:       ctrl.Group.String(),
			Mode:        "Classic",
			Order:       i,
		}
	}
}

// LookupRatingCategory resolves a rating category (a variant HTMLName) to its
// display info. ok is false for an unknown category — e.g. a legacy row that no
// longer maps to a curated variant.
func LookupRatingCategory(htmlName string) (RatingCategoryInfo, bool) {
	info, ok := ratingCategories[htmlName]
	return info, ok
}
