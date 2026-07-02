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
// depth only affects search strength, which no room test asserts on. The depth
// must be lowered before UpEngine: bring-up kicks off the deploy-cache warmer,
// which reads DeploySearchDepth on another goroutine.
func TestMain(m *testing.M) {
	engine.DeploySearchDepth = 1
	dispatch.UpEngine()
	os.Exit(m.Run())
}
