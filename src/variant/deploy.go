package variant

import (
	"time"

	"github.com/dechristopher/lio/clock"
)

// DeployPreStart bounds the first-move grace after the deploy reveal: the
// revealed position is on screen for this long before white's clock starts
// draining on its own (white may move sooner to start the game manually).
const DeployPreStart = 10 * time.Second

// withDeployPreStart copies a standard time control and adds the deploy
// pre-start countdown to it.
func withDeployPreStart(tc clock.TimeControl) clock.TimeControl {
	tc.PreStart = clock.ToCTime(DeployPreStart)
	return tc
}

// HalfOneBlitzDeploy is the 30 second, one second increment blitz variant
// played with the blind deploy pre-game.
var HalfOneBlitzDeploy = Variant{
	Name:     "½ + 1",
	HTMLName: "half-one-blitz-deploy",
	Group:    DeployGroup,
	Control:  withDeployPreStart(HalfOneBlitzTC),
	Deploy:   true,
}

// QuarterZeroBulletDeploy is the 15 second, zero second increment bullet variant
// played with the blind deploy pre-game.
var QuarterZeroBulletDeploy = Variant{
	Name:     "¼ + 0",
	HTMLName: "quarter-zero-bullet-deploy",
	Group:    DeployGroup,
	Control:  withDeployPreStart(QuarterZeroBulletTC),
	Deploy:   true,
}

// OneTwoRapidDeploy is the one minute, two second increment rapid variant
// played with the blind deploy pre-game.
var OneTwoRapidDeploy = Variant{
	Name:     "1 + 2",
	HTMLName: "one-two-rapid-deploy",
	Group:    DeployGroup,
	Control:  withDeployPreStart(OneTwoRapidTC),
	Deploy:   true,
}

// ThreeFiveRapidDeploy is the three minute, five second increment rapid variant
// played with the blind deploy pre-game.
var ThreeFiveRapidDeploy = Variant{
	Name:     "3 + 5",
	HTMLName: "three-five-rapid-deploy",
	Group:    DeployGroup,
	Control:  withDeployPreStart(ThreeFiveRapidTC),
	Deploy:   true,
}
