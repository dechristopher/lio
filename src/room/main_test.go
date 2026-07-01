package room

import (
	"os"
	"testing"

	"github.com/dechristopher/lio/dispatch"
	"github.com/dechristopher/lio/engine"
)

// TestMain brings the engine dispatcher online for the whole room test binary so
// the bot deploy path (handleDeploy -> requestEngineDeploy -> dispatch ->
// engine.SelectDeployment) has a live dispatcher to talk to, and shrinks the
// deploy search depth so the 144-position selection stays fast under -race. The
// depth only affects search strength, which no room test asserts on.
func TestMain(m *testing.M) {
	dispatch.UpEngine()
	engine.DeploySearchDepth = 1
	os.Exit(m.Run())
}
