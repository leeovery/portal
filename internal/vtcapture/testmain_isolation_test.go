package vtcapture_test

// The poisoned ZMX_DIR makes any zmx run that misses the Backend's own
// directory fail loudly instead of reaching the developer's live sessions.
// ZMX_SESSION would turn an attach into a switch of a live session, and
// ZMX_SESSION_PREFIX would rename every session a test names.

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/zmxtest"
)

func TestMain(m *testing.M) {
	for _, err := range []error{
		os.Setenv("ZMX_DIR", zmxtest.PoisonedDir),
		os.Setenv("TMUX_TMPDIR", zmxtest.PoisonedTmpdir),
		os.Unsetenv("ZMX_SESSION"),
		os.Unsetenv("ZMX_SESSION_PREFIX"),
	} {
		if err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}
