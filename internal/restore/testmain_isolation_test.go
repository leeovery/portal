package restore_test

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/xdg"
	"github.com/leeovery/portal/internal/zmxtest"
)

// TestMain poisons every variable a Portal path resolves from, so a test that
// forgets to isolate fails loudly instead of writing into the developer's real
// state, and a restored session it makes resolves nothing of theirs. The
// poisoned ZMX_DIR sits under /dev/null, so any zmx run that misses the test's
// own directory fails with ENOTDIR rather than reaching their live sessions.
func TestMain(m *testing.M) {
	errs := []error{
		os.Setenv("ZMX_DIR", zmxtest.PoisonedDir),
		os.Setenv("TMUX_TMPDIR", zmxtest.PoisonedTmpdir),
		os.Unsetenv("ZMX_SESSION"),
		os.Unsetenv("ZMX_SESSION_PREFIX"),
	}
	for _, name := range xdg.Variables() {
		errs = append(errs, os.Setenv(name, "/nonexistent/portal-test-must-isolate/"+name))
	}
	for _, err := range errs {
		if err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}
