package variant

// HalfOneBlitzDeploy is the 30 second, one second increment blitz variant
// played with the blind deploy pre-game.
var HalfOneBlitzDeploy = Variant{
	Name:     "½ + 1",
	HTMLName: "half-one-blitz-deploy",
	Group:    DeployGroup,
	Control:  HalfOneBlitzTC,
	Deploy:   true,
}

// QuarterZeroBulletDeploy is the 15 second, zero second increment bullet variant
// played with the blind deploy pre-game.
var QuarterZeroBulletDeploy = Variant{
	Name:     "¼ + 0",
	HTMLName: "quarter-zero-bullet-deploy",
	Group:    DeployGroup,
	Control:  QuarterZeroBulletTC,
	Deploy:   true,
}

// OneTwoRapidDeploy is the one minute, two second increment rapid variant
// played with the blind deploy pre-game.
var OneTwoRapidDeploy = Variant{
	Name:     "1 + 2",
	HTMLName: "one-two-rapid-deploy",
	Group:    DeployGroup,
	Control:  OneTwoRapidTC,
	Deploy:   true,
}
