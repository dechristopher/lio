package variant

import (
	"time"

	"github.com/dechristopher/lio/clock"
)

// UnlimitedCasual is the untimed casual variant: the clock is effectively
// infinite (see UnlimitedTC), so players can think as long as they like —
// against the computer or another human. Casual rooms are cleaned up on
// disconnect instead of on the clock (see room.Params.Casual).
var UnlimitedCasual = Variant{
	Name:     "∞",
	HTMLName: "unlimited-casual",
	Group:    UnlimitedGroup,
	Control:  UnlimitedTC,
	Casual:   true,
}

// UnlimitedCasualDeploy is UnlimitedCasual with the blind deploy pre-game.
var UnlimitedCasualDeploy = Variant{
	Name:     "∞",
	HTMLName: "unlimited-casual-deploy",
	Group:    UnlimitedGroup,
	Control:  UnlimitedTC,
	Casual:   true,
	Deploy:   true,
}

// UnlimitedTC is the casual time control: a year on the clock, which is
// "infinite" for any real session. Using a plain (huge) time rather than a
// special no-clock mode keeps the clock, budget, and protocol plumbing
// untouched; clients render casual clocks as ∞ instead of the number.
var UnlimitedTC = clock.TimeControl{
	Time:      clock.ToCTime(time.Hour * 24 * 365),
	Increment: clock.ToCTime(time.Second * 0),
}
