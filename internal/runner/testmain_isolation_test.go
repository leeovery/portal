package runner_test

// The poisoned ZMX_DIR makes any zmx run that misses the Backend's own
// directory fail loudly instead of reaching the developer's live sessions.
// ZMX_SESSION would turn an attach into a switch of a live session, and
// ZMX_SESSION_PREFIX would rename every session a test names. The test binary
// also stands in for the attach child, re-executed under childModeEnv, for a
// whole attach under afterRunEnv, and for one that fails to begin under
// abandonEnv.

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/zmxtest"
)

func TestMain(m *testing.M) {
	if mode, ok := os.LookupEnv(childModeEnv); ok {
		os.Exit(fakeChild(mode))
	}
	if _, ok := os.LookupEnv(afterRunEnv); ok {
		os.Exit(interruptAfterRun())
	}
	if _, ok := os.LookupEnv(abandonEnv); ok {
		os.Exit(abandonCatchingTerm())
	}
	for _, err := range []error{
		os.Setenv("ZMX_DIR", zmxtest.PoisonedDir),
		os.Unsetenv("ZMX_SESSION"),
		os.Unsetenv("ZMX_SESSION_PREFIX"),
	} {
		if err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}
