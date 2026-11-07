package saver_test

// TestMain poisons every path the saver could resolve on its own, so a test
// that forgets its temp state dir fails loudly instead of writing into the
// developer's real state or reaching their live zmx sessions: the poisoned
// ZMX_DIR sits under /dev/null, so any read of it fails with ENOTDIR.

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/zmxtest"
)

func TestMain(m *testing.M) {
	for _, err := range []error{
		os.Setenv("PORTAL_STATE_DIR", "/nonexistent/portal-test-must-isolate-state"),
		os.Setenv("PORTAL_HOOKS_FILE", "/nonexistent/portal-test-must-isolate-hooks.json"),
		os.Setenv("PORTAL_PROJECTS_FILE", "/nonexistent/portal-test-must-isolate-projects.json"),
		os.Setenv("PORTAL_ALIASES_FILE", "/nonexistent/portal-test-must-isolate-aliases"),
		os.Setenv("XDG_CONFIG_HOME", "/nonexistent/portal-test-must-isolate-config"),
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
