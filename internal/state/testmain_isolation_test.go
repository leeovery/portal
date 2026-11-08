package state_test

// TestMain poisons every PORTAL_* path env var before any test runs, so a test
// that forgets to isolate fails loudly instead of quietly mutating the
// developer's real configuration.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("PORTAL_STATE_DIR", "/nonexistent/portal-test-must-isolate-state")
	os.Setenv("PORTAL_HOOKS_FILE", "/nonexistent/portal-test-must-isolate-hooks.json")
	os.Setenv("PORTAL_PROJECTS_FILE", "/nonexistent/portal-test-must-isolate-projects.json")
	os.Setenv("PORTAL_ALIASES_FILE", "/nonexistent/portal-test-must-isolate-aliases")
	os.Exit(m.Run())
}
