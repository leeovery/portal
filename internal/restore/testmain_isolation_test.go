package restore_test

// TestMain poisons every variable a Portal path resolves from, so a test that
// forgets to isolate fails loudly instead of writing into the developer's real
// state, and a restored session it makes resolves nothing of theirs. The
// poisoned ZMX_DIR sits under /dev/null, so any zmx run that misses the test's
// own directory fails with ENOTDIR rather than reaching their live sessions.

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/xdg"
	"github.com/leeovery/portal/internal/zmxtest"
)

func TestMain(m *testing.M) {
	for _, name := range xdg.Variables() {
		os.Setenv(name, "/nonexistent/portal-test-must-isolate/"+name)
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
