package migrate_test

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/xdg"
	"github.com/leeovery/portal/internal/zmxtest"
)

// TestMain poisons every variable a Portal path resolves from, zmx's
// directory and $TMUX, so a test that strays from the dirs and the socket it
// is given reaches none of the developer's real state, hooks.json, sessions
// or tmux server.
func TestMain(m *testing.M) {
	errs := []error{
		os.Setenv("TMUX", "/nonexistent/portal-test-must-set-tmux-socket,0,0"),
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
