package migrate_test

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/xdg"
	"github.com/leeovery/portal/internal/zmxtest"
)

// TestMain poisons every variable a Portal path resolves from, and zmx's
// directory, so a test that strays from the dirs it is given fails loudly
// instead of reaching the developer's real state, hooks.json or sessions.
func TestMain(m *testing.M) {
	errs := []error{
		os.Setenv("ZMX_DIR", zmxtest.PoisonedDir),
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
