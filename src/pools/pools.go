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

// speedByNameGroup resolves the (variant_name, variant_group) pair an archived
// game row stores to that variant's speed class. The archive does not keep the
// HTMLName for unrated games, so this pair is all a profile row has to go on.
//
// The two unlimited casual variants share a (name, group) pair — they differ
// only by whether the deploy pre-game runs — but they share a speed class too,
// so the collision cannot produce a wrong answer here.
var speedByNameGroup = map[string]string{}

func init() {
	for _, v := range Map {
		label := v.SpeedGroup().String()
		// Deploy is the default mode and adds nothing, but the *classic* form of
		// a control resolves to the same speed class — "½ + 1 blitz" for both —
		// so the non-default mode has to be named or two distinct variants
		// render identically in any per-variant list.
		if info, ok := ratingCategories[v.HTMLName]; ok && info.Mode != "" {
			label += " " + info.Mode
		}
		speedByNameGroup[v.Name+"\x00"+string(v.Group)] = label
	}
}

// SpeedFor returns the display qualifier for an archived game's time control —
// its speed class ("bullet"/"blitz"/"rapid"/…), plus the mode when that is not
// the default ("blitz Classic"). It falls back to the stored group for a variant
// no longer curated, so a retired pool still reads as something rather than as
// nothing.
//
// This is what keeps "deploy" off the player page: deploy is the default mode,
// so labelling every row with it says nothing, while the speed class is the
// distinction a player actually reads (see variant.SpeedGroup).
func SpeedFor(name, group string) string {
	if s, ok := speedByNameGroup[name+"\x00"+group]; ok {
		return s
	}
	return group
}
